# ThreatOptic CLI

Command-line client for the ThreatOptic security monitoring platform.

Written in Go so that every invocation starts in single-digit milliseconds — the binary has no runtime to load, so per-command latency is network time and nothing else.

## Install

macOS and Linux — downloads the right archive for your platform, verifies its SHA-256 against the release `checksums.txt`, and installs to `/usr/local/bin` (or `~/.local/bin` if that is not writable):

```bash
curl -fsSL https://github.com/ThreatOptic/CLI/releases/latest/download/install.sh | bash
```

Windows (PowerShell) — installs to `%LOCALAPPDATA%\Programs\threatoptic` and adds it to your user `PATH`:

```powershell
irm https://github.com/ThreatOptic/CLI/releases/latest/download/install.ps1 | iex
```

Homebrew:

```bash
brew install --cask threatoptic/tap/threatoptic
```

Or download an archive by hand from [the releases page](https://github.com/ThreatOptic/CLI/releases). Prebuilt binaries cover macOS (amd64, arm64), Linux (amd64, arm64), and Windows (amd64).

The install scripts take `THREATOPTIC_VERSION` to pin a tag and `THREATOPTIC_INSTALL_DIR` (`-InstallDir` on Windows) to choose a location.

## Build from source

Requires Go 1.25 or newer.

```bash
go install github.com/ThreatOptic/CLI/cmd/threatoptic@latest
```

From a checkout:

```bash
make build            # stamps the version from git describe
go build -o threatoptic ./cmd/threatoptic
```

Cross-compile for another platform:

```bash
GOOS=linux GOARCH=amd64 go build -o threatoptic-linux-amd64 ./cmd/threatoptic
```

## Getting started

Create an API key in the ThreatOptic dashboard under **Account → API keys**. Keys are shown once, start with `top_`, and inherit your account's permissions. The CLI cannot create or revoke keys because the `/api-keys` endpoints require a browser session (JWT), not an API key.

```bash
$ threatoptic auth login
ThreatOptic API key:
Logged in as dev@example.com
Credentials saved to /Users/you/.config/threatoptic/config.yaml

$ threatoptic whoami
dev@example.com  (user 0192f3a1-..., roles: member)

$ threatoptic check stripe.com
kind: domain
subject: stripe.com
present: true
mcp: false
generated: 3
...

$ threatoptic whoami --json
{"email":"dev@example.com","id":"0192f3a1-...","roles":["member"]}
```

`auth login` verifies the key against `GET /auth/me` before writing anything, so a bad paste fails immediately rather than on some later command.

## Commands

| Command | Description |
| --- | --- |
| `threatoptic version` | Print the binary version. Needs no configuration or network. |
| `threatoptic auth login` | Verify an API key and store it. |
| `threatoptic auth status` | Show the endpoint, the masked key and where it came from, and the account. |
| `threatoptic auth logout` | Remove the stored key. |
| `threatoptic whoami` | Print the account id, email, and roles for the current key. |
| `threatoptic check <query>` | Look up live lookalikes for a domain, package, or MCP name. |

Global flags: `--api-url`, `--api-key`, `--json`.

## Configuration

Settings resolve in this order, first match wins:

| Priority | API URL | API key |
| --- | --- | --- |
| 1. Flag | `--api-url` | `--api-key` |
| 2. Environment | `THREATOPTIC_API_URL` | `THREATOPTIC_API_KEY` |
| 3. Config file | `api_url` | `api_key` |
| 4. Default | `http://localhost:8000` | none |

The config file lives at `$XDG_CONFIG_HOME/threatoptic/config.yaml`, falling back to `~/.config/threatoptic/config.yaml`:

```yaml
api_url: https://api.example.com
api_key: top_...
```

Point at a different environment without touching the stored config:

```bash
threatoptic whoami --api-url https://api.example.com
```

Run in CI without writing anything to disk:

```bash
export THREATOPTIC_API_KEY="$CI_THREATOPTIC_KEY"
threatoptic whoami --json
```

`auth login` sources its key from `--api-key`, then `THREATOPTIC_API_KEY`, and otherwise the prompt. A key already on disk never suppresses the prompt, because someone running `auth login` is replacing it.

## Credential security

- The config file is written `0600` inside a `0700` directory, using a temporary file and rename so an interrupted write cannot truncate an existing credential.
- The API key is never logged and never printed in full. `auth status` shows only a masked prefix, in both text and JSON output.
- The interactive prompt does not echo the key to the terminal. Prefer it over `--api-key`, which is visible to other users via `ps`; the flag exists for CI, where the value comes from a secret store.
- Errors report only the API's `detail` field, never a raw response body.
- Your key carries your full account permissions. Revoke it in the dashboard if a machine is lost, and run `threatoptic auth logout` to remove the local copy.

## Output conventions

- Results go to stdout, errors to stderr, and failures exit non-zero.
- `--json` emits machine-readable output as the only content on stdout, so `threatoptic whoami --json | jq -r .email` is safe.

## Development

```bash
make test            # unit tests, no live API required
make lint            # go vet and a gofmt check
```

Layout:

```
cmd/threatoptic/     # main, version stamping
internal/cmd/        # cobra command tree
internal/config/     # load/save, precedence, masking
internal/api/        # HTTP client, API error mapping
```

Commands depend on a small `client` interface rather than the concrete type, so command tests run against a fake while the client's own tests exercise real HTTP via `httptest`.

## Releasing

Pushing a semver tag (`v1.2.3`) triggers the release workflow, which runs tests and then [goreleaser](https://goreleaser.com) to build all five platform archives, write `checksums.txt`, create the GitHub release, and update the Homebrew cask in [ThreatOptic/homebrew-tap](https://github.com/ThreatOptic/homebrew-tap).

To validate a config change without publishing anything:

```bash
make check           # validate .goreleaser.yaml
make snapshot        # build every platform archive into dist/
```

Archive names (`threatoptic_<version>_<os>_<arch>.tar.gz`) are a contract: `scripts/install.sh` and `scripts/install.ps1` reconstruct them. Change `archives.name_template` in `.goreleaser.yaml` and update both install scripts to match.

Maintainers who publish from a workstation instead of CI need `GITHUB_TOKEN` with `contents: write` on this repository and on `ThreatOptic/homebrew-tap`, then:

```bash
git tag -a v1.2.3 -m "Release v1.2.3"
git push origin v1.2.3
# or, if the tag already exists locally:
make release
```
