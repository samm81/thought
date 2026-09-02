# thought

thought is a terminal-first, local-first publishing tool for writing Markdown notes and explicitly cross-posting them to Bluesky and X.

## AGENTS.md

- This file and the `.agents/` directory should follow the progressive disclosure policy
- The top level AGENTS.md should ideally be < 50 lines, and never > 100
- The root AGENTS.md should contain only:
    - One-line project description
    - Package manager (if not npm)
    - Non-obvious commands only (skip `npm test`, `npm run build` if standard)
    - Links to `.agents/rules/` files with brief descriptions
    - Verification section (always required)
- Remaining instructions should go in `.agents/rules/` files by category (e.g., TypeScript conventions, testing patterns, API design, Git workflow).
- The following content should not go in AGENTS.md nor `.agents/` files:
    - **API documentation** — link to external docs instead
    - **Code examples** — agents can infer from reading source files
    - **Interface/type definitions** — these exist in the code
    - **Generic advice** — "write clean code", "follow best practices"
    - **Obvious instructions** — "use TypeScript for .ts files"
    - **Redundant info** — things already included in your operating instructions
    - **Too vague** — things that aren't actionable

## package manager

- go modules, defined in `go.mod`.

## commands

- `go run ./cmd/thought --help` — inspect the command surface.

## rules

Before any Go coding, review, debugging, troubleshooting, or setup task, load the `samber/cc-skills-golang@golang-how-to` skill first — it routes to whichever other Go skills the task needs.

- [go development](.agents/rules/go.md) — project-specific Go structure, dependency, and archive rules.

## verification

after making changes:

- `make check` — run formatting checks, `go vet`, and race-enabled tests.
- `make lint` — run the configured `golangci-lint` checks.
- `make audit` — run the `govulncheck` dependency scan.
