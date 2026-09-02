# contributing

## prerequisites

- Go 1.27.1;
- GNU Make;
- `golangci-lint` for linting;
- `govulncheck` for dependency auditing.

## workflow

format and validate changes before opening a pull request:

```sh
make format
make check
make lint
```

keep changes focused, add tests for observable behavior, and update the relevant specification or documentation when behavior changes. do not add a provider SDK without a documented need and review of its maintenance and security impact.
