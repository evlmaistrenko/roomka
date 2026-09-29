# Agent instructions

Read [CONTRIBUTING.md](CONTRIBUTING.md) before changing code. Its naming,
layout and testing conventions apply to agents exactly as they do to people.
This file only adds what matters specifically when an agent does the work.

## Scope

- The live services are `services/control` and `services/ui`.
- The control server's contracts are `packages/control-specs/`: the GraphQL
  SDL in `graphql/`, the HTTP surface in `openapi.yaml`. After touching
  either, run `npm run generate:control` yourself: don't rely on a watcher
  being up. Never hand-edit the generated `graph/generated.go`,
  `internal/schema/models_gen.go` and `rest/generated.go` under
  `services/control`, but do keep them in the change. In
  `graph/*_resolvers.go`, write only the one-line delegation bodies.
- A new non-GraphQL endpoint starts in `openapi.yaml`, not in a hand-written
  route: the generated interface is what keeps spec and server in step.
- Spec, migration or environment changes are contract changes. Say so
  explicitly in your summary.

## Commands

Run from the repository root:

```sh
npm run dev:control
npm run generate:control
gofmt -l services/control
go -C services/control vet ./...
go -C services/control test ./...
```

Before reporting a Go change as done, `gofmt -l` must print nothing, and vet and
the tests must pass. Report failures as they are; don't work around them.

## Git

- Commit messages follow Conventional Commits. commitlint rejects anything
  else.
- Don't commit, push or open pull requests unless asked.
