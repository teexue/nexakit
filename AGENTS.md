# nexakit coding standards

Go runtime library, module `github.com/teexue/nexakit`. This document constrains this module only.

## Build and test

```bash
go test ./...
go vet ./...
```

`golangci-lint run` runs in CI (gocyclo ≤ 15, nesting > 4 fails). It does not replace the two commands above. Do not loosen thresholds, delete tests, add `nolint`, or skip hooks to pass the gate. No `go generate`, `go install`, or Docker.

- **Go**: manage dependencies with `go mod`. Do not hand-edit versions in `go.mod`.

## Architecture (highest priority)

- **Single Agent Loop entry**: every caller must use the same `loop.Run`. Do not copy a second loop in this module.
- **One Tool abstraction**: expose every capability through the `Tool` interface. Do not hardcode business logic inside the loop.
- **Differences come from Agent config**: prompts, tool allowlists, permissions, and model settings come from the `agent.Agent` the caller fills in. Do not write `switch agent` branches. This module does not parse files.
- **Event stream**: the only outward type is `event.Event`. Types: `text_delta` / `reasoning_delta` / `tool_start` / `tool_result` / `tool_approval_required` / `compaction` / `sub_agent_start` / `sub_agent_end` / `error` / `done`.
- **Events are a contract**: a new `event.Type` must update this module's `event` package in the same change, including `PrintEvents` and `AllTypes`. Mark the commit as breaking.
- **Dependency direction**: this module does not import callers. Callers use the public API.
- **Identity is supplied by the caller**: the caller passes the MCP handshake name and version. An empty value may fall back only to `nexakit` / `dev`. This module does not wrap audit logs.
- **Layering serves readability**: layering and interfaces are for *present* clarity and decoupling, not speculation. Add a layer or interface when it makes the current structure clearer or breaks a real coupling; do not add layers for imagined future needs (see Forbidden). When a package grows several distinct concerns (e.g. protocol types vs transports), split it into subpackages — the directory name is the package name.
- **Dependency direction is one-way**: a package depends only on packages "below" it. Before adding an import, check it does not create a cycle; if it would, the shared type belongs in the lower package (for example `tool` defines the registry interface `registry` implements).

## Layout

The directory name is the package name. Do not add a `core/` or `tools/` prefix.

```
loop  event  provider  tool  session  agent  compaction
embedding  mcp  registry
hook  permission  subagent

provider/openai  provider/anthropic  provider/ollama  provider/catalog
mcp/protocol  mcp/stdio  mcp/sse  tool/builtin
```

This module has no database, observability stack, or file parsing.

| Package | Responsibility |
|---------|----------------|
| `loop` | The only Agent Loop. Every path must go through `Run`. |
| `event` | Shared event types |
| `provider` | LLM contract: `Provider` interface, message/chunk types, model specs, vendors, HTTP/SSE helpers, mock. Implementations live in `provider/openai`, `provider/anthropic`, `provider/ollama`; `provider/catalog` resolves profiles to providers. |
| `tool` | Tool interface (`Name` / `Description` / `InputSchema` / `Execute`), the registry interface, canonical tool names, and per-call context helpers. `Result.Output` is `json.RawMessage`. Concrete tools live in `tool/builtin`. |
| `tool/builtin` | The built-in tools (file ops, run_command, web_fetch, …) plus their shared internals (path safety, file lock, shell detection). `registry.RegisterBuiltin` wires them. |
| `agent` | Runtime agent config and validation. Does not read files. |
| `session` | Thread-safe session: `AddMessages` / `GetMessages` / `Clear` |
| `compaction` | Context compaction |
| `embedding` | Embedding HTTP client. No index store. |
| `mcp` | MCP client: protocol types, `Manager`, and the `ExternalTool` bridge. Transports live in `mcp/stdio` and `mcp/sse`. Handshake identity is supplied by the caller. |
| `registry` | Tools by name plus LLM definitions for the LLM; `RegisterBuiltin` wires the built-in tools |
| `hook` | Lifecycle hooks |
| `permission` | Tool-execution permission |
| `subagent` | Sub-agent scheduling. Loading an agent by name uses optional `LoadAgent`. If it is unset, loading by name is unavailable. |

- Package names are short, lowercase, and have no underscores (`loop`, not `agent_loop`). The directory name is the package name.
- One package per directory. No `util`, `common`, `helper`, or `misc` packages or files.
- Shared types live in a package with a clear meaning (for example `event`, `agent`).
- **No duplicate definitions**: a concept is defined once. In particular, an interface lives in the lowest package that needs it (dependency direction points that way) and every implementation satisfies it — do not redeclare the same interface in another package. `tool.Registry` is the single tool-registry interface; `tool.Tool` the single capability interface; `provider.ToolDefinition` the single tool-definition type.
- **One export surface per package**: a package exposes the minimal set other packages need. Do not re-export another package's types or leak internal helpers through the public API.

## Conventions

- **Tool execution**: `tool_execution.mode` selects parallel (streaming, the default) or serial. `tool_execution.max_parallel` caps concurrency (default 4).
- **Test mocks**: use `provider/mock.MockProvider` (each step can set `Text`, `Reasoning`, and `ToolCalls`) and `mock.EchoThenReply()`. Do not call a real LLM. Test doubles live in sibling `mock` packages (`provider/mock`, `embedding/mock`), never in the production package itself.
- **Tool names**: snake_case and globally unique (for example `read_file`). Register with `(*registry.Registry).Register`. Do not register from `init()`.
- **HTTP client**: providers and MCP tools use `provider.DefaultHTTPClient()` (no total timeout; a 60s response-header timeout). Do not use `http.DefaultClient`.
- **Thinking**: OpenAI-compatible `ThinkingConfig` controls Kimi-style reasoning. `ReasoningDelta` is written to the event stream.
- **No file I/O**: do not parse YAML or SQLite, and do not hardcode a caller's config directory. Runtime structs do not carry file-format tags.
- **Optional injection**: when `loop.Config.LoadAgent` and `EnrichContext` are nil, the loop does not load an agent by name and does not rewrite the request context.

## Agent config fields

`agent.Agent` expresses runtime config only. This module does not define a file format.

## Maintainability

| Metric | Limit |
|--------|-------|
| Lines per file | ≤ 500 (tests ≤ 600) |
| Lines per function | ≤ 80 |
| Parameters | ≤ 5 (use a Config struct beyond that) |
| Nesting depth | ≤ 4 (return early or extract a function) |
| Cyclomatic complexity | ≤ 15 |

These numbers are not adjustable. If a limit is exceeded, split the code. Nesting and cyclomatic complexity are enforced by golangci (`gocyclo` and nesting depth).

**Counting**:

- **File**: physical lines, including blanks and comments. Production `*.go` (not `_test.go`); tests are `*_test.go`.
- **Function**: from the signature line through the matching final `}`, including both. Every `func` (methods and tests included) is ≤ 80.
- **Parameters**: each parameter counts as 1, including `ctx`. The method receiver does not count. `...T` counts as 1. Multi-line signatures must be parsed. `a, b string` is 2.
- **Exported docs**: the line above each exported `func`, `type`, `var`, and `const` must be a doc comment that starts with that name. Each exported identifier in a `const (` block has its own comment. One comment must not cover several.
- **Size exemption**: `vendor/`. Do not hide business code in an exempt directory to dodge the line limit.

## Go style

- **Names**: exported PascalCase, unexported camelCase, files snake_case, tool names snake_case.
- **Imports**: three groups — standard library, external dependencies, internal packages.
- **File layout**: follow `imports → const → var → type → func`. Package-level loose constants (defaults, thresholds, magic values) live in a top-of-file const block and never after their first user; a typed enum's const block directly follows its type declaration (the one exception); a constant used by a single function stays inside that function. Before adding a constant, grep the package for the string value — do not introduce a second name for the same literal.
- **Errors**: return `(T, error)` and wrap with `fmt.Errorf("context: %w", err)`. Return early. Do not panic on expected errors.
- **Context**: first parameter of I/O and LLM calls. Do not store it in a struct. Honor cancellation.
- **Interfaces**: small interfaces, injected via `NewXxx(deps...)`. Avoid global registration in `init()`.
- **Comments**: exported symbols need a doc comment (a short comment that starts with the name). Write why, not a restatement of the code. No section banners (`// ───`).
- **Logs**: structured `log/slog`, with fields `session_id`, `agent`, `tool`, and `turn`.
- **Splitting `loop.Run`**: extract unexported functions in the same package only. Do not change the single entry point or the external behavior.
- If a new function already needs more than 5 parameters, it must take a Config. Do not write it first and refactor later.

## Comments

- Write why: invariants, external contracts, non-obvious algorithms, and deliberate departures from the usual approach.
- Do not restate the code, add section banners, or leave stale history comments.
- Exported symbols must have a doc comment.
- Do not use `nolint` instead of a refactor. If the rule cannot be satisfied in code, the marker must sit next to the violation and state why.

## Tests

- Prefer table-driven tests. Use `testify/assert` and `testify/require`. Cover the happy path and at least one error path.
- Split files by scenario. A test file is ≤ 600 lines.
- Test files mirror the source file they test: `web_fetch.go` is tested by `web_fetch_test.go`. Do not create catch-all test files named after a fix or incident (`fixes_test.go`, `bugfix_test.go`); if a tool has no test file yet, create `<tool>_test.go`.
- Do not call a real LLM. Use `provider.MockProvider`.

## Git

- **Commit**: Conventional Commits — `<type>(<scope>): <description>`
- **Description language**: English
- **Type**: `feat` / `fix` / `docs` / `refactor` / `test` / `chore` / `perf` / `style`
- **Branches**: `feat/<name>`, `fix/<name>`, hyphen-separated, all lowercase
- **PR**: title follows Conventional Commits, links an issue, and focuses on one subtask. CI must run `go test ./...` and `golangci-lint`.

## Forbidden

| Do not | Why |
|--------|-----|
| Import a caller module | Reverses the dependency direction |
| Add an HTTP/gRPC server framework | This module is not a transport layer |
| Create a `utils`, `helpers`, or `misc` package or file | The package has no clear meaning |
| Add a `core/` or `tools/` directory prefix again | This module no longer uses those layers |
| Call an LLM provider outside the loop | Skips permission and audit |
| Execute a tool without a permission check | Security hole |
| Branch on a specific agent with if/else | Agent behavior is config-driven |
| Panic on an expected error | Return an error |
| Store context in a struct | Breaks Go practice |
| Commit `.env`, an API key, or credentials | Security risk |
| Hand-edit versions in `go.mod` | Use `go get` / `go mod tidy` |
| Abstract ahead of need | Do not define an interface or layer for a *hypothetical* future requirement |
| Delete tests or change a threshold to pass a check | Split the code instead |

## Change discipline

- One change focuses on one subtask.
- Create a git commit only when the user explicitly asks.
- A new tool: implement it in `tool` (or its own package), register it via `(*registry.Registry).Register` (built-ins via `registry.RegisterBuiltin`), plus table-driven tests for success and failure.
- A new `event.Type`: update the `event` declaration, `PrintEvents`, and `AllTypes` together. Mark the commit as breaking.
- A new provider: implement `provider.Provider` and use `DefaultHTTPClient`.
- A new package: the directory name is the package name, and its job can be stated in one sentence.
