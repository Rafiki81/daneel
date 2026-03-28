# Contributing to Daneel

Thank you for your interest in contributing! Daneel is an open-source Go library for building AI agents and we welcome all kinds of contributions.

## Getting Started

1. **Fork** the repository on GitHub.
2. **Clone** your fork locally:
   ```sh
   git clone https://github.com/<your-username>/daneel.git
   cd daneel
   ```
3. Create a feature or fix branch:
   ```sh
   git checkout -b feat/my-feature
   ```

## Development Requirements

- **Go 1.24+**
- No external dependencies — the entire library uses only the Go standard library. Keep it that way.

## Running Tests

```sh
go test ./...
```

All tests must pass before submitting a PR. For coverage:

```sh
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out -o coverage.html
```

## Code Style

- Run `go vet ./...` and `go fmt ./...` before committing.
- Follow standard Go idioms and naming conventions.
- Exported types and functions must have godoc comments.
- Keep packages small and focused. Each package should do one thing well.

## Submitting a Pull Request

1. Ensure `go build ./...` and `go test ./...` both pass.
2. Write or update tests for any changed behaviour.
3. Update relevant documentation in godoc comments or `README.md` if applicable.
4. Open a pull request against the `main` branch with a clear description of what was changed and why.

## Reporting Issues

Open an issue on GitHub with:
- A minimal reproducible example.
- The Go version you are using (`go version`).
- The operating system.
- What you expected to happen vs what actually happened.

## License

By contributing you agree that your contributions will be licensed under the [MIT License](LICENSE).
