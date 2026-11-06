# GitHub repository settings

The automation token used by cloud agents cannot always update repository metadata.
Apply these settings in the GitHub UI under **Settings → General** (and **Topics** on the repo home page).

## About

| Field | Value |
| --- | --- |
| Description | `Unofficial read-only CLI for official BIG Games APIs (Pet Simulator 99 first). OAuth account reads, public leaderboards, and market data.` |
| Website | `https://github.com/andrew867/bg-cli#readme` |
| Topics | `cli`, `go`, `golang`, `oauth`, `pet-simulator-99`, `big-games`, `roblox` |

## Features

| Feature | Recommended |
| --- | --- |
| Issues | Enabled |
| Projects | Optional / off |
| Wiki | Off |
| Discussions | Optional |
| Releases | Enabled (used by GoReleaser) |

## Release automation

| Trigger | Workflow | Output |
| --- | --- | --- |
| Push tag `v*` | `release.yml` | GitHub Release with platform archives, source tarball, checksums |
| Push/PR to `main`, manual dispatch | `build-binaries.yml` | Workflow artifacts (snapshot archives, 14-day retention) |

No extra secrets are required beyond the default `GITHUB_TOKEN` for public or private repos where Actions can write releases.
