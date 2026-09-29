# Contributing

## Repository

| Path                     | What it is                                        |
| ------------------------ | ------------------------------------------------- |
| `services/control`       | Go GraphQL server: accounts, sessions, access     |
| `services/ui`            | React client (npm workspace)                      |
| `packages/control-specs` | Contracts of `services/control`: GraphQL, OpenAPI |
| `.runtime`               | Local state of the dev tooling (gitignored)       |

`services/` holds what runs; `packages/` holds what the services are built from
but does not run by itself. Directories directly under both are kebab-case. A
package is a plain directory unless a tool needs more: `control-specs` has no
`package.json`, because nothing installs it. A service's own dev scripts live
in its `scripts/` directory.

`.runtime/` holds what the dev tooling keeps between runs, one subdirectory per
service (`.runtime/control/`). It is never committed, and deleting it is always
safe: the tooling rebuilds it, at worst by doing work it would otherwise skip.

**Generated code is committed**, in the same commit as the source it came
from. Hand-written code is built on it, so a fresh checkout must compile
without a generation step, and a change to a source shows its effect in the
diff. `npm run generate:*` rebuilds it; during `npm run dev` a `dev:*-specs`
watcher does that on every save.

Everything runs from the repository root:

```sh
cp .env.example .env
npm install
npm run dev    # every service
npm test       # every test suite
```

### Commits

Commits follow [Conventional Commits](https://www.conventionalcommits.org/);
commitlint checks them in a Husky hook, and release-please builds the changelog
and the version from them. Pick the type by what the change means to a user of
the release: `feat` and `fix` appear in the changelog, `refactor`, `test`,
`docs`, `chore`, `ci` do not.

### Formatting

Prettier formats JS/TS/JSON/Markdown/YAML, and gofmt formats Go. lint-staged
runs both on staged files in the pre-commit hook.

### Language

Everything committed is in English: code, comments, GraphQL descriptions,
documentation, commit messages, pull requests. Discussion around the code
(issues, chat) may be in any language. The one exception is the UI's
translations (`services/ui/src/locales/`), each of which is in its own
language.

### Local notes

`.notes/` at the repository root is gitignored and belongs to whoever is
working in the checkout: deferred tasks, drafts, reminders. Nothing committed
may refer to a file in it, and nothing in it is a rule. A rule goes into this
file.

## Naming in every language

**Identifiers spell words out**: `session`, never `sess`; `address`, not
`addr`; `configuration`, not `cfg`. The exceptions are the language's own
idioms (Go's `err`, `ctx`, short method receivers; `i` for a loop index) and
near-universal shorthand (`dev`, `cert`, `id`, `url`). If you're unsure whether
an abbreviation qualifies, spell it out.

**Packages and modules are named for what they provide.** Avoid catch-all
names like `util`, `common`, `helpers`, `misc` for anything code imports: they
say where code was put, not what it does, so unrelated code keeps landing in
them. Schema files are exempt: they are sections of one schema, not units
anything imports, and a `common` section holding the shared building blocks
is fine.

## Go

Applies to every Go module under `services/`.

### Files and directories

**snake_case**, generated files included (`04_accounts_resolvers.go`,
`password_reset/`). The `_test.go` suffix is Go's own and follows the
snake_case head (`password_reset_test.go`).

A Go package clause cannot contain an underscore, so the directory
`password_reset/` declares `package passwordreset` and is imported with an
explicit alias:

```go
passwordreset "control/internal/password_reset"
```

Fixed tool names (`go.mod`, `gqlgen.yml`) and extensions (`.graphqls`) are
exempt.

### Identifiers

Go idiom: PascalCase for exported names, camelCase for the rest, initialisms
kept whole (`userID`, `APIPort`). A package name must not shadow a stdlib
package the same code imports.

### Imports

Three groups separated by a blank line: stdlib, third-party, then the module's
own packages.

### Comments

Every package has a package comment saying what it is responsible for. Comments
explain _why_: a constraint, a rejected alternative, a trap. Don't narrate the
code.

### Tests

- A package's tests sit next to it (`session_test.go` beside `session.go`).
- Prefer the real dependency to a mock when it's cheap to run, as sqlite is.
- Create temporary directories for files a test keeps open (a sqlite database)
  with `os.MkdirTemp` and best-effort cleanup, not `t.TempDir`. On Windows
  sqlite's WAL/SHM files can stay locked for a moment after close, and
  `t.TempDir` fails the test on the cleanup error.

### Before you commit

```sh
gofmt -l services/<module>                 # must print nothing
go -C services/<module> vet ./...
go -C services/<module> test ./...
```

## JavaScript and TypeScript

Applies to every npm package under `services/`.

### Files and directories

**kebab-case**: `settings-dialog.tsx`, `use-theme.ts`, `import-settings/`.
Dotted suffixes are not part of the name, and the head before them stays
kebab-case: `settings-dialog.test.tsx`, `webcodecs-extra.d.ts`,
`vite.config.ts`.

### Identifiers

| What                                           | Case                                       |
| ---------------------------------------------- | ------------------------------------------ |
| Variables, functions, parameters               | camelCase                                  |
| Properties, including fields of our interfaces | camelCase                                  |
| Types, interfaces, classes, React components   | PascalCase                                 |
| Module-level constants with a fixed value      | UPPER_SNAKE_CASE (`const MAX_RETRIES = 3`) |

"Constant" means a value fixed at authoring time, declared at module level. A
local `const` binding is just a variable that isn't reassigned, and is
camelCase.

The one exception is an external API whose names are spelled differently (for
example `snake_case` JSON). Only the property names that mirror it keep the
API's spelling, and only at the boundary: the local variables that receive
those values, and our own interfaces, stay camelCase.

## services/ui

A React client of `services/control`: urql over HTTP for queries and
mutations, one graphql-ws socket for subscriptions, reached same-origin at
`/graphql` (the Vite proxy in development).

```sh
npm run generate:ui         # operations -> types, always
npm run dev:ui-documents    # regenerate on every save (part of npm run dev)
```

Operations are written in place with `graphql()` from `@/graphql/generated`,
which graphql-codegen types against the control server's SDL. The output in
`src/graphql/generated/` is committed and never edited.

**Every user-facing string is a translation key.** The English messages in
`src/locales/en.ts` define the keys, and every other locale has to provide all
of them, which the type check enforces. A signed-in user's language is their
`language` preference on the server; before sign-in the page uses the last
language this browser showed, then the browser's own.

## services/control

A GraphQL API (gqlgen, schema-first) over sqlite (bun on the pure-Go
`modernc.org/sqlite` driver), plus the few HTTP endpoints that are not GraphQL
(oapi-codegen, spec-first). The binary must stay cgo-free: the image builds it
with `CGO_ENABLED=0`.

### Running

```sh
npm run generate:control    # both specs -> Go, always
npm run dev:control         # regenerate if needed, then air: rebuild and
                            # restart on every .go change
npm run dev:control-specs   # regenerate on every spec save that changes it
npm run test:control        # go test ./...
```

`npm run dev` starts all three `dev:*` together, so a spec save regenerates the
Go code and air then restarts the server on it, once. air, gqlgen and
oapi-codegen come from the `tool` block in `go.mod`: nothing to install.

`scripts/generate.js` decides when to regenerate, per spec: only when a hash
of it differs from the one recorded after its last successful run
(`.runtime/control/graphql.sha256`, `openapi.sha256`), and then only the Go
package generated from it. air ignores the fully generated files, which gqlgen
deletes and rewrites over several seconds, and rebuilds on those hash files
instead, so it never builds a half-written package (`.air.toml`).

The server has no development mode and behaves the same everywhere. Next to
`/graphql` it always serves Apollo Sandbox at `/graphql/playground`, the
OpenAPI spec at `/openapi.json`, and Swagger UI over it at `/openapi` (its
assets are embedded in the binary). Which of these the outside world reaches
is the reverse proxy's call. GraphQL introspection is on by default: the SDL is
public in this repository, so hiding it from the live server would protect
nothing. `ROOMKA_DISABLE_GRAPHQL_INTROSPECTION=true` turns it off, for a
deployment whose schema is not public. That switch has to be the server's: a
proxy could filter request bodies, but queries also run over the subscription
WebSocket, whose frames it never sees.

The dev database lives at `.runtime/control/roomka.db`; delete it to start
over.

Configuration is environment only. Every `ROOMKA_*` variable is required with no
default, except the ones `internal/config` marks optional. A new variable goes
into `internal/config`, `.env.example`, and the README.

`graph/generated.go`, `internal/schema/models_gen.go` and `rest/generated.go`
are entirely generated: never edit them. In `graph/*_resolvers.go` gqlgen
writes the stubs and we write the bodies, which it carries over when it
regenerates. `rest/` implements the interface oapi-codegen generates, so an
operation added to the spec does not compile until it is implemented.

### Layout

The contracts, in `packages/control-specs/`:

```
graphql/
  01_schema.graphqls     the contract starts here: header, empty root types
  02_common.graphqls     scalars, @constraint, PageInfo, Query.version
  03_errors.graphqls     ErrorCode, InvalidInputReason
  04_accounts.graphqls   @auth, roles, users, sessions; extends the roots
openapi.yaml             the HTTP surface: REST endpoints in full, /graphql as
                         a transport only (its operations are the SDL's)
```

The server, in `services/control/`:

```
main.go                  wiring only: config, database, roles, server
gqlgen.yml               GraphQL codegen: where the SDL is, where code goes
oapi_codegen.yml         OpenAPI codegen, likewise; skips the graphql tag
.air.toml                what air watches (see Running)
scripts/generate.js      regenerate what changed, once or on every save
rest/
  generated.go           generated types, handlers, embedded spec — never edit
  server.go              implements the generated interface
  docs.go                /openapi.json and the embedded Swagger UI
graph/
  *_resolvers.go         one per schema file with fields: generated stubs,
                         filled with one-line delegations
  generated.go           generated executable schema — never edit
  resolver.go            Resolver struct and directive wiring
  access.go              @auth
  validation.go          @constraint
internal/
  accounts/              the domain: sign-in, users, sessions, preferences
  config/                environment
  database/              open, migrate, sqlite helpers
    migrations/          one Go file per migration
  events/                in-process pub/sub for subscriptions
  identity/              principal in context, cookie, password hashing
  model/                 database rows (bun models)
  network/               which address a request came from
  password_reset/        signed reset secrets
  ratelimit/             token buckets, caps, and the Policy with every limit
  rbac/                  role→permission map read from @grants
  schema/                generated GraphQL types (models_gen.go) +
                         hand-written errors, Void
  server/                HTTP routes, middleware, error presenter
  session/               session storage and resolution
api_test/                black-box tests through real HTTP
```

### Naming

**Migrations** are named `<yyyymmddhhmmss>_<what_it_does>.go`; bun takes the
order from the name.

**Schema files** are `packages/control-specs/graphql/<nn>_<subject>.graphqls`,
numbered in reading
order: the header first, then shared building blocks, then one file per
domain. gqlgen ignores the order, so the numbers are only for people. A new
domain takes the next free number and adds its root fields with
`extend type Query` / `Mutation` / `Subscription`, its own permissions with
`extend enum Permission`, and its own error codes with `extend enum ErrorCode`,
without editing the files before it.

**Case by layer:**

| Layer                  | Case                                      |
| ---------------------- | ----------------------------------------- |
| Go                     | Go idiom (see [Go](#go))                  |
| GraphQL fields, args   | camelCase; enum values `UPPER_SNAKE_CASE` |
| SQL tables and columns | snake_case (`user_roles`, `expires_at`)   |
| Environment variables  | `ROOMKA_UPPER_SNAKE_CASE`                 |

Go↔SQL mapping lives in exactly one place: explicit `bun:"column_name"` tags in
`internal/model`. Write every tag out, even when bun's default would match, so
that renaming a Go field can't silently rename a column. Avoid column names that
are SQL keywords instead of quoting them.

**Domain vocabulary:** `Viewer` is the authenticated caller (`Query.me`). A
person who watches a stream is a `Reader`, never a viewer.

### Where code goes

- **`graph/` delegates and decides nothing.** A resolver is one call into a
  service. Keep domain rules out of `graph/`: once a rule lives in a resolver,
  the next resolver can skip it.
- **Access policy is declared in the schema.** A field says what it needs with
  `@auth` on the root field. Only rules a directive can't express (ranks) are
  enforced in `internal/accounts`, next to the data they concern.
- **Input shape is declared in the schema.** Lengths, patterns and ranges go in
  `@constraint`. Code checks only what the schema can't state (for example
  "too guessable" for passwords), and reports it in the same error shape.
- **Services speak the schema's types.** `internal/accounts` returns
  `schema.User`, not a domain type of its own; `internal/model` is only the
  row. Conversions between the two live in the service's `mapping.go`.
- **Errors carry a code from the schema's `ErrorCode` enum.** Use
  `schema.Coded` / `schema.InvalidInput`, so clients branch on the code, never
  the message. An error that says whether something exists (a user, a session)
  is itself a leak: answer "missing" or "invalid" the same way for unknown,
  foreign and malformed ids.
- **Timestamps are UTC, written by the server through bun.** Never use a SQL
  `DEFAULT` for time: two writers in two formats compare wrongly.
- **Secrets are never stored in the clear.** A session cookie is a random
  secret, and only its sha256 is stored. A password reset secret is random
  too, and only a sha256 of it together with the password hash in force is
  stored, so setting a password by any path retires it. No key signs either:
  there is nothing that could forge one. Passwords are argon2id at OWASP's minimum
  parameters: the cost is for the day the database leaks, when guessing is
  offline and no rate limit applies.
- **Every limit is in `ratelimit.Policy`.** The numbers sit side by side in
  `DefaultPolicy`; code takes them from there, never as literals. A limit that
  needs to know what a call is about (which account a password is for) is
  enforced by the service that knows; the rest sit in front of the whole
  server in `internal/server`. Anything that can be guessed at — a password, a
  reset — is limited both per address and per target, and the per-target
  limit applies whether or not the target exists, so running into it says
  nothing about existence.
- **The client address comes from `network.ClientAddress`.** It believes
  `X-Forwarded-For` only from loopback, where the proxy runs, and then only
  the entry the proxy appended. Never read the header anywhere else: a limit
  keyed on a header the client writes is no limit.
- **Realtime goes through `internal/events`.** Topic names come from the
  functions in that package. A mutation publishes to the affected topics. A
  subscription emits the current state on subscribe and again on every change,
  and ends without an error when the access behind it changes: it never
  re-checks permissions on its own.

### Migrations

The server applies every pending migration when it starts, before it serves
anything; a migration that fails stops it. There is no separate migration step
in a deployment.

- **A migration only adds.** Tables, columns, indexes. Removing or renaming
  takes two releases: one where the code stops using the old thing, and a later
  one whose migration drops it. That is what makes a rollback safe: rolling
  back is starting the previous image, and the previous code has to run on the
  newer schema, because it knows nothing of the newer migrations, their down
  functions included. Down functions are for development; nothing in a
  deployment runs them.
- **A shipped migration never changes.** Its effect on a database has to be the
  same on every database it ever runs on. A migration that builds from the
  types in `internal/model` creates whatever the models say today, so it may do
  that only until it ships: the base migration builds from the models now,
  because nothing but development databases exists, and is written out on its
  own terms at the first release. From then on, every change to the schema is a
  new migration.
- **Backups are not the server's job.** They are taken regularly, outside the
  app, by whoever runs it: the database is a single sqlite file in the data
  volume.

### Comments

A doc comment on a service method that answers a GraphQL field starts with the
field's name (`// revokeSession ends one of the caller's other sessions.`).

In the schema, try a better name, a type or nullability before writing a
description. Describe only what structure can't express.

### Tests

- **`api_test/` is the default place for a behaviour test.** It starts the real
  server in-process (`httptest`) on a fresh sqlite and talks to it over HTTP
  and WebSocket with a cookie jar, exactly like a client. Most of what can
  break (`@auth`, `@constraint`, cookie middleware, error codes) is outside the
  resolver methods, so tests that call resolvers directly would miss it. The
  package contains only `_test.go` files.
- **Package tests** are for logic that is easier to pin down below the API:
  token handling, migrations, hashing.
