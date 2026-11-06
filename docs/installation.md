# Installation

## Binary releases

Download the archive for your platform from the GitHub Releases page. Verify the published checksums before installing.

| Platform | Artifact |
| --- | --- |
| Windows amd64 | `bg-cli-windows-amd64.exe` |
| Linux amd64 | `bg-cli-linux-amd64` |
| macOS amd64 | `bg-cli-darwin-amd64` |
| macOS arm64 | `bg-cli-darwin-arm64` |

Place the binary on your `PATH` and confirm:

```bash
bg-cli version
bg-cli --help
```

## Install with Go

```bash
go install github.com/andrew867/bg-cli/cmd/bg-cli@latest
```

The `bg-cli` binary is written to `$(go env GOPATH)/bin`. Ensure that directory is on your `PATH`.

## Build from source

```bash
git clone https://github.com/andrew867/bg-cli.git
cd bg-cli
go build -o bg-cli ./cmd/bg-cli
./bg-cli version
```

Required toolchain: Go 1.23 or newer.

## Configuration locations

| Item | Default location |
| --- | --- |
| Non-secret profile metadata | `$BG_CLI_CONFIG_DIR` or OS user config dir `/bg-cli/config.json` |
| Access tokens | OS credential store service `bg-cli` |
| Public response cache | `$BG_CLI_CACHE_DIR` or OS user cache dir `/bg-cli/public` |

Override directories with environment variables when running in constrained environments. Tokens are never written to the config or cache directories.

## Credential store prerequisites

| OS | Backend |
| --- | --- |
| Windows | Credential Manager |
| macOS | Keychain |
| Linux | FreeDesktop Secret Service (for example gnome-keyring) |

If no secure store is available, `auth login` fails with exit code 7. The client does not fall back to plaintext token files.
