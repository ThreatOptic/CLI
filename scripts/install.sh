#!/usr/bin/env bash
#
# Install the ThreatOptic CLI on macOS or Linux.
#
#   curl -fsSL https://github.com/ThreatOptic/CLI/releases/latest/download/install.sh | bash
#
# Environment:
#   THREATOPTIC_VERSION       Tag to install (default: the latest release)
#   THREATOPTIC_INSTALL_DIR   Where to put the binary (default: /usr/local/bin, or
#                             ~/.local/bin when /usr/local/bin is not writable)

set -euo pipefail

REPO="ThreatOptic/CLI"
BINARY="threatoptic"

# Global so the EXIT trap can still see it after main returns.
workdir=""

cleanup() {
	if [ -n "$workdir" ]; then
		rm -rf "$workdir"
	fi
}
trap cleanup EXIT

fail() {
	echo "install.sh: $1" >&2
	exit 1
}

need() {
	command -v "$1" >/dev/null 2>&1 || fail "$1 is required but was not found on PATH"
}

detect_os() {
	case "$(uname -s)" in
	Darwin) echo darwin ;;
	Linux) echo linux ;;
	*) fail "unsupported operating system: $(uname -s). Windows users: see install.ps1" ;;
	esac
}

detect_arch() {
	case "$(uname -m)" in
	x86_64 | amd64) echo amd64 ;;
	arm64 | aarch64) echo arm64 ;;
	*) fail "unsupported architecture: $(uname -m)" ;;
	esac
}

# Follows the /releases/latest redirect rather than calling the API, which is
# rate limited far more aggressively for unauthenticated callers.
latest_tag() {
	local url
	url=$(curl -fsSLI -o /dev/null -w '%{url_effective}' "https://github.com/$REPO/releases/latest") ||
		fail "could not reach GitHub to resolve the latest release"
	local tag="${url##*/}"
	[ -n "$tag" ] && [ "$tag" != "latest" ] || fail "could not determine the latest release tag"
	echo "$tag"
}

sha256_of() {
	if command -v sha256sum >/dev/null 2>&1; then
		sha256sum "$1" | cut -d' ' -f1
	else
		shasum -a 256 "$1" | cut -d' ' -f1
	fi
}

choose_install_dir() {
	if [ -n "${THREATOPTIC_INSTALL_DIR:-}" ]; then
		echo "$THREATOPTIC_INSTALL_DIR"
	elif [ -w /usr/local/bin ]; then
		echo /usr/local/bin
	else
		echo "$HOME/.local/bin"
	fi
}

main() {
	need curl
	need tar
	command -v sha256sum >/dev/null 2>&1 || need shasum

	local os arch tag archive tmp install_dir
	os=$(detect_os)
	arch=$(detect_arch)
	tag="${THREATOPTIC_VERSION:-$(latest_tag)}"
	# Must match archives.name_template in .goreleaser.yaml.
	archive="${BINARY}_${os}_${arch}.tar.gz"

	workdir=$(mktemp -d)
	tmp="$workdir"

	echo "Downloading $BINARY $tag for $os/$arch..."
	curl -fsSL -o "$tmp/$archive" "https://github.com/$REPO/releases/download/$tag/$archive" ||
		fail "no release asset $archive in $tag"
	curl -fsSL -o "$tmp/checksums.txt" "https://github.com/$REPO/releases/download/$tag/checksums.txt" ||
		fail "could not download checksums.txt for $tag"

	local expected actual
	expected=$(grep " $archive\$" "$tmp/checksums.txt" | cut -d' ' -f1)
	[ -n "$expected" ] || fail "$archive is not listed in checksums.txt"
	actual=$(sha256_of "$tmp/$archive")
	[ "$expected" = "$actual" ] || fail "checksum mismatch for $archive (expected $expected, got $actual)"

	tar -xzf "$tmp/$archive" -C "$tmp" "$BINARY"

	install_dir=$(choose_install_dir)
	mkdir -p "$install_dir"
	install -m 0755 "$tmp/$BINARY" "$install_dir/$BINARY" 2>/dev/null ||
		fail "could not write to $install_dir. Set THREATOPTIC_INSTALL_DIR to a writable directory."

	echo "Installed $BINARY $("$install_dir/$BINARY" version) to $install_dir/$BINARY"

	case ":$PATH:" in
	*":$install_dir:"*) ;;
	*) echo "Add it to your PATH:  export PATH=\"$install_dir:\$PATH\"" ;;
	esac

	echo "Next:  $BINARY auth login"
}

main "$@"
