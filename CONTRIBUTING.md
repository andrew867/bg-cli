# Contributing

## Development setup

1. Install Go 1.23 or newer
2. Clone the repository
3. Run `go test ./...`

## Coding standards

- Format with `gofmt`
- Keep packages focused; prefer `internal/` for implementation details
- Do not commit secrets, live API responses containing personal data, or credential files
- Deterministic tests must use local servers and synthetic fixtures under `testdata/fixtures`
- Production code must not hard-code credentials or attach account tokens to public routes

## Pull requests

- Use conventional commits (`feat`, `fix`, `docs`, `test`, `chore`, `refactor`)
- Keep changes focused and documented
- Update `CHANGELOG.md` under `[Unreleased]` when behavior changes
- Ensure `go test ./...`, `go vet ./...`, and formatting checks pass

## Security

Follow `SECURITY.md` for vulnerability reports. Never paste live client secrets or access tokens into issues or pull requests.
