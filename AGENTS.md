# Repository Guidelines

## Project Structure

- `main.go` starts the CLI application.
- `cmd/` contains command implementations.
- `cli/` contains CLI entry points and validation helpers.
- `core/` contains shared Apstra domain logic, CSV helpers, and logging setup.
- `templates` contains j2 templates that are used with the -o Templates.

## Development Notes

- This is a Go module: `rickycraft/lapstra`.
- Run `gofmt` on edited Go files.
- Prefer focused changes that match existing package boundaries.
- Keep changes simple and small; prefer the least code that solves the request.
- Do not create testfile.
- If an object is not nil and you get it back from a function there is no need to call .ID() and check that is not nil

## Verification

- DO NOT USE `go test ./...` when changes affect Go behavior.
- Use `go build ./...` when checking build-only changes.

