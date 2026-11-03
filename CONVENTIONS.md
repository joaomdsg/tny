# Conventions

Each convention gives its reasoning, then the rule. If they disagree, the
reasoning wins. If the reasoning no longer applies, edit the rule here.

## Standard Library Only

Reasoning: tny is meant to stay small enough to read in one sitting. Each
dependency adds code no one here has read, and the standard library
already covers HTTP, JSON, processes, and testing.

Rule: No third-party modules, so `go.mod` has no `require` block. This
also covers tests: use `testing` and `net/http/httptest`, not testify or
mock generators. Adding a dependency is a deliberate decision to discuss
first, not something to do in passing.

## Deep Modules

Reasoning: The `agent` package hides the wire format, cache breakpoints,
tool loop, history rollback, and pricing behind `New`, `Ask`, `Reset`, and
`Session`. Callers can't misuse what they
can't reach, and the internals can change freely.

Rule:

- Export as little as possible. Add new capability behind the existing
  methods or as an `Option` before adding a method.
- `main` holds only the REPL, env parsing, and terminal output. Anything
  that knows about the Messages API belongs in `agent`.
- Don't export wire types. JSON structs for the API stay unexported, and
  content blocks pass through as `json.RawMessage` so fields tny doesn't
  know about reach the API unchanged.

## Functional Options

Reasoning: Variadic options keep `New(apiKey)` clean for the common case
and avoid constructors full of booleans or positional config.

Rule: Optional config uses `type Option func(*Agent)` with a `With...`
constructor per setting. Defaults are named constants in `agent.go`.

## Test Names

Reasoning: Consistent naming makes tests discoverable and makes clear what
each test verifies.

Rule: Use `Test` + PascalCase subject + underscore + camelCase behavior
(present tense verb). The underscore separates *what* from *does what*.

- Good: `TestAsk_rollsBackAFailedTurnSoTheNextOneStartsClean`
- Good: `TestTruncate_neverSplitsAMultibyteCharacter`
- Bad: `TestAsk` (vague: what about it?)
- Bad: `Test_ask_rolls_back` (wrong casing)

The name should read as a claim about behavior, not a description of what
the test does internally.

## Test-First

Reasoning: Writing the test first forces you to define the contract before
the implementation, and gives every behavior a test.

Rule: No implementation before a failing test. The sequence is always:
write test → confirm it fails for the right reason → implement → confirm
it passes. A bug fix starts with a test that reproduces the bug.

## Test Scope: Outside-In Through the Public API

Reasoning: Tests coupled to internals break on refactors, not on
regressions. The public API is the contract.

Rule: Tests enter through exported symbols, in `package agent_test`. Only
use `package agent` (internal) when `Ask` can't reach the behavior, as a
last resort. Today: `bash_test.go` (exit codes, truncation) and `usage_test.go`
(pricing math).

## Fakes: Real > Fake > Mock

Reasoning: The closer a test is to production behavior, the more confidence
it gives. Mocks that verify call counts or arguments test wiring, not
behavior, and break when the implementation changes.

Order of preference:

1. **Real**: the actual implementation. Tests run real bash.
2. **Fake**: a small working stand-in at the process boundary, such as
   the `httptest` server in `agent_test.go` that serves scripted
   Messages API replies and records the requests.
3. **Mock**: a double that asserts on calls. Don't use these.

Rule: The only boundary to fake is the Messages API, reached through
`WithEndpoint`. Never fake code you own.

## Test Behavior, Not Implementation

Reasoning: Tests that assert on internal state or call order specify how
something works rather than what it does. They get in the way of
refactoring.

Rule:

- Assert on observable outputs: the reply, the returned `Usage` and error,
  what was sent to the API (request bodies the fake recorded), and what
  was written to the tool log.
- Don't assert on unexported fields or execution order.
- For large or generated output, use `strings.Contains`: its shape can
  change without breaking the contract.

## Assertions

Reasoning: Without testify, one failure-message shape keeps failures
scannable.

Rule:

- Use `t.Fatalf` for preconditions that guard the rest of the test (an
  unexpected error, a missing request). Use `t.Errorf` for the behavioral
  claims you want to see all of, even when one fails.
- Format value comparisons as `got = X, want Y`:
  `t.Errorf("reply = %q, want %q", reply, "hello")`.
- Compare floats with a tolerance (`math.Abs(got-want) > 1e-9`), never
  `==`.

## Table-Driven Tests

Reasoning: Repeating the same test structure with different inputs is
clearer as a table, which separates the cases from the mechanics.

Rule: Use table-driven subtests for parameterized scenarios. Each case
needs a `name` field. Run subtests with `t.Run`.

```go
tests := []struct {
    name  string
    input string
    want  string
}{
    {"empty input", "", ""},
    {"single word", "hello", "hello"},
}
for _, tt := range tests {
    t.Run(tt.name, func(t *testing.T) {
        t.Parallel()
        if got := fn(tt.input); got != tt.want {
            t.Errorf("fn(%q) = %q, want %q", tt.input, got, tt.want)
        }
    })
}
```

## Parallel Tests

Reasoning: Running tests in parallel surfaces data races and speeds up
the suite.

Rule: Call `t.Parallel()` at the top of every test and subtest that does
not share mutable state. Each test gets its own fake server and its own
`Agent`. When in doubt, don't parallelize, because a correct slow test
beats a flaky fast one.

Always run tests with `-race`: `go test -race ./...`.

## Test Helpers

Reasoning: Helpers that don't call `t.Helper()` report misleading failure
line numbers. Helpers in production files pollute the API.

Rule:

- All test helpers live in `_test.go` files.
- Helpers that take a `*testing.T` and can fail call `t.Helper()` as
  their first statement.
- Prefer small builders (`text("hi")`, `bash(id, cmd)`, `newAgent(url)`)
  for scripting the fake over repeating JSON literals in each test.

## Comments

Comments explain **why**, never **what**. If a comment restates what the
code already says, delete it.

### Exported symbols

Document every exported type, function, method, and constant with a godoc
comment. The comment must add information beyond the name. State the
contract, the default, or the caveat.

```go
// Good: states the default and the trade-off
// WithCacheTTL sets the prompt-cache lifetime, "5m" (default) or "1h". 1h
// survives long pauses, but its cache writes cost 2x input instead of 1.25x.

// Bad: restates the name
// WithModel sets the model.
```

### Unexported symbols and inner logic

Leave out comments whose purpose is clear from the name and context. Add
a comment only when a reader would otherwise have to reconstruct
non-obvious reasoning:

- A constraint imposed by an external system or protocol (the API rejects
  a tool call without a result).
- A subtle invariant that must hold across edits (only the last block
  carries a cache breakpoint).
- A deliberate choice that looks wrong but isn't.

```go
// Good: explains a magic number from an external tool
// timeout(1) exits 124 when it had to kill the command.
timeoutExitCode = 124

// Bad: obvious from context
// append the message to history
a.history = append(a.history, msg)
```

### Tests

The test name is the comment, so don't put prose above a test function.
Comment inside a test only when the setup has a non-obvious precondition,
or to show where an expected number comes from:

```go
// in 1 + 1.25*1 (5m write) + 2*1 (1h write) + 0.1*1 (read) = 4.35, out 10
if got := u.Cost(); math.Abs(got-14.35) > 1e-9 {
```

## Errors

Reasoning: One rule each for returning, creating, and panicking.

### Panic vs. Return

Rule: Panic for programming mistakes, errors that can't happen if the code
is correct (`mustJSON` on values built in-package). Return errors for
anything that depends on the outside world: the network, the API, user
input.

### Creating Errors

Rule: Use `fmt.Errorf` with a short lowercase prefix that names the step
that failed (`encode request: ...`, `unreadable response: ...`). Wrap the
cause with `%w`.

### No Custom Error Types

Rule: Use plain `error` values. Don't introduce sentinel errors or custom
types unless a concrete caller switches on them. An error taxonomy built
before anyone needs it is speculative abstraction.

### Failures Leave State Valid

Rule: A method that fails must leave the conversation as it was. `Ask`
truncates history back to where it started; billed usage still counts
toward `Session`. Never store a
partial or undecodable response, because one bad message makes every
later request invalid.

## Package and File Organization

Reasoning: Files grouped by responsibility beat one file per type or one
file for everything.

### Packages

Rule: A package owns one concept. Split a package when it has two
unrelated responsibilities, not when it gets large. Today that is `main`
(the terminal app) and `agent` (the conversation).

### Files

Rule: Group by responsibility, not by type, and name files after the
responsibility: `api.go` (wire format and requests), `bash.go` (the tool),
`usage.go` (tokens and cost), `agent.go` (the public surface), `main.go`
(REPL and env vars), `term.go` (color, spinner, tool log).

Split a file when it exceeds ~300 lines or contains two concerns that
change for different reasons. Don't split preemptively.

## Markdown

Reasoning: One style keeps diffs about content, not formatting.

### Line Length

Rule: Keep lines to 80 characters or fewer. Break long lines at logical
points. Avoid tables. Use lists, prose, or code blocks instead.

### Lists

Rule: Surround lists with blank lines.

### Headings

Rule: Use headings (`##`) instead of emphasis (`**text**`) to introduce
sections. Reserve emphasis for inline content within paragraphs.
