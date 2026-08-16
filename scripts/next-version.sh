#!/usr/bin/env bash
#
# Print the next CLI release tag.
#
# Usage:
#   scripts/next-version.sh              # bump from commits since the latest v* tag
#   scripts/next-version.sh patch|minor|major
#
# Exit 0 and print vMAJOR.MINOR.PATCH.
# Exit 2 if HEAD is already an exact semantic version tag (nothing to publish).
# Exit 1 on usage or git errors.

set -euo pipefail

forced_bump="${1:-}"
case "$forced_bump" in
'' | patch | minor | major) ;;
*)
	echo "usage: $0 [patch|minor|major]" >&2
	exit 1
	;;
esac

latest_semver_tag() {
	git tag --list 'v[0-9]*.[0-9]*.[0-9]*' --sort=-v:refname | head -n1
}

bump_from_commits() {
	local range="$1"
	local messages
	messages=$(git log "$range" --pretty=%B)

	if printf '%s\n' "$messages" | grep -qE 'BREAKING CHANGE|^[a-z]+(\([^)]*\))?!:|[[:space:]]\[major\]|^\[major\]'; then
		echo major
		return
	fi
	if printf '%s\n' "$messages" | grep -qiE '\[major\]'; then
		echo major
		return
	fi
	if printf '%s\n' "$messages" | grep -qiE '\[minor\]'; then
		echo minor
		return
	fi
	if printf '%s\n' "$messages" | grep -qE '^feat(\(|:|!)'; then
		echo minor
		return
	fi
	echo patch
}

head_tag=""
if head_tag=$(git describe --tags --exact-match HEAD 2>/dev/null); then
	if [[ "$head_tag" =~ ^v[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.]+)?$ ]]; then
		echo "HEAD is already tagged ${head_tag}; skipping" >&2
		exit 2
	fi
fi

latest=$(latest_semver_tag)
if [ -z "$latest" ]; then
	echo v0.1.0
	exit 0
fi

if [ -n "$forced_bump" ]; then
	bump="$forced_bump"
else
	bump=$(bump_from_commits "${latest}..HEAD")
fi

ver=${latest#v}
ver=${ver%%-*}
IFS=. read -r major minor patch <<<"$ver"

case "$bump" in
major) echo "v$((major + 1)).0.0" ;;
minor) echo "v${major}.$((minor + 1)).0" ;;
patch) echo "v${major}.${minor}.$((patch + 1))" ;;
esac
