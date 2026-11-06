# Installation

## Prebuilt binaries (recommended)

Tagged releases publish self-contained archives built with [GoReleaser](https://goreleaser.com):

| Platform | Archive |
| --- | --- |
| Linux amd64 | `bg-cli_Linux_x86_64.tar.gz` |
| Linux arm64 | `bg-cli_Linux_arm64.tar.gz` |
| macOS Intel | `bg-cli_Darwin_x86_64.tar.gz` |
| macOS Apple Silicon | `bg-cli_Darwin_arm64.tar.gz` |
| Windows amd64 | `bg-cli_Windows_x86_64.zip` |
| Source tree | `bg-cli-<version>-source.tar.gz` |
| Checksums | `bg-cli_<version>_checksums.txt` |

Binaries are statically linked (`CGO_ENABLED=0`). Extract the archive and place `bg-cli` (or `bg-cli.exe`) on your `PATH`.

```bash
# Example: Linux amd64
tar -xzf bg-cli_Linux_x86_64.tar.gz
sudo mv bg-cli /usr/local/bin/
bg-cli version

# Verify checksums
sha256sum -c bg-cli_0.1.2_checksums.txt
```

On Windows, extract the zip and add the folder containing `bg-cli.exe` to your user `PATH`, or run it from the extraction directory.

CI also uploads snapshot archives as workflow artifacts on pushes and pull requests targeting `main` (14-day retention). Prefer tagged GitHub Releases for production installs.

## Install with Go

```bash
go install github.com/andrew867/bg-cli/cmd/bg-cli@latest
```

Or pin a release tag:

```bash
go install github.com/andrew867/bg-cli/cmd/bg-cli@v0.1.2
```

The binary is written to `$(go env GOPATH)/bin`. Ensure that directory is on your `PATH`.

## Build from source

```bash
git clone https://github.com/andrew867/bg-cli.git
cd bg-cli
git checkout v0.1.2   # optional pin
go build -o bg-cli ./cmd/bg-cli
./bg-cli version
```

Required toolchain: Go 1.23 or newer.

Local multi-platform archives (developers):

```bash
goreleaser release --snapshot --clean --skip=publish
ls dist/
```

## Publishing a release

Maintainers create a semver tag. GitHub Actions runs GoReleaser and attaches archives, the source tarball, and checksums to the GitHub Release:

```bash
git tag v0.1.2
git push origin v0.1.2
```

## Configuration locations

| Item | Default location |
| --- | --- |
| Non-secret profile metadata | `$BG_CLI_CONFIG_DIR` or OS user config dir `/bg-cli/config.json` |
| Access tokens | OS credential store service `bg-cli` |
| Public response cache | `$BG_CLI_CACHE_DIR` or OS user cache dir `/bg-cli/public` |

## Credential store prerequisites

| OS | Backend |
| --- | --- |
| Windows | Credential Manager |
| macOS | Keychain |
| Linux | FreeDesktop Secret Service (for example gnome-keyring) |

If no secure store is available, `auth login` fails with exit code 7. The client does not fall back to plaintext token files.
