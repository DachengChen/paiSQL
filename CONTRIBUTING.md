# Contributing to paiSQL

Thank you for your interest in contributing to paiSQL! This document outlines the process for contributing to the project.

## Branching Strategy

We use the following branching model:

- `main`: **Stable Branch**. This branch contains production-ready code. Commits here are tagged releases.
- `development`: **Active Development**. All feature branches and bug fixes should be merged into `development` first. Once stable, `development` is merged into `main`.

## Getting Started

1.  **Fork the repository** on GitHub.
2.  **Clone** your fork locally.
3.  **Create a branch** for your feature or fix from `development`:
    ```bash
    git checkout development
    git pull origin development
    git checkout -b feature/my-awesome-feature
    ```

## Local Development

### Prerequisites
- Go 1.24 or higher (we use [goenv](https://github.com/go-nv/goenv) for version management)

### Running the App
```bash
go run .
```

### Linting
Run lint checks locally before submitting a PR (mirrors CI exactly):
```bash
scripts/lint-local.sh
```

### Running Tests
Ensure tests pass before submitting a PR:
```bash
go test -v ./...
```

### Building Binary
To build a production binary:
```bash
go build -o bin/paisql .
```

## Submitting Changes

1.  Push your branch to your fork.
2.  Open a **Pull Request** targeting the `development` branch.
3.  Ensure CI checks (linting, tests) pass.

## Release Process

Releases are automated using GitHub Actions and GoReleaser.

1.  Update version in `tui/tui.go` or wherever version is defined.
2.  Tag the commit on `main`:
    ```bash
    git tag v0.2.0
    git push origin v0.2.0
    ```
3.  The **Release** workflow will automatically build binaries and create a GitHub Release.
