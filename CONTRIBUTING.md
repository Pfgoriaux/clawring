# Contributing to Clawring

Thanks for your interest in contributing to Clawring!

## Development Setup

1. **Go 1.22+** is required. Install from [go.dev](https://go.dev/dl/).

2. Fork and clone the repository:

   ```bash
   git clone https://github.com/<your-username>/clawring.git
   cd clawring
   ```

3. Build:

   ```bash
   go build -o clawring .
   ```

4. Run tests:

   ```bash
   go test ./...
   go test -race ./...
   ```

## Making Changes

1. Create a branch from `main`:

   ```bash
   git checkout -b my-feature
   ```

2. Make your changes.

3. Ensure code passes formatting and vetting:

   ```bash
   gofmt -w .
   go vet ./...
   ```

4. Run the full test suite:

   ```bash
   go test -race ./...
   ```

5. Commit with a clear message (see below).

6. Push and open a pull request against `main`.

## Pull Request Process

- Keep PRs focused. One logical change per PR.
- Include tests for new functionality or bug fixes.
- Update documentation if your change affects user-facing behavior.
- All CI checks must pass before merge.

## Commit Messages

Use short, descriptive commit messages:

```
Add rate limiting per agent
Fix token rotation race condition
Update vendor registry docs
```

Start with an imperative verb (Add, Fix, Update, Remove, Refactor). Keep the first line under 72 characters.

## Code Style

- Run `gofmt` on all Go files.
- Run `go vet ./...` before committing.
- Keep functions short and focused.
- Avoid adding external dependencies unless absolutely necessary.

## Reporting Bugs

Open a GitHub issue with:

- What you expected to happen
- What actually happened
- Steps to reproduce
- Go version and OS

## Security Issues

Do **not** open a public issue for security vulnerabilities. See [SECURITY.md](SECURITY.md) for responsible disclosure instructions.
