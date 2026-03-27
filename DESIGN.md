# Tank UI and CLI Design Guide

This document describes the UI, display, and command-line conventions used in Tank. It is a design-system document for the CLI itself: how commands should be named, how output should look, how flags should behave, and how new features should fit into the existing interaction model.

This is intentionally based on the current codebase, primarily [`ui/`](./ui), [`cmd/tank/`](./cmd/tank), and the user-facing docs in [`README.md`](./README.md). Where the current implementation is inconsistent, this document defines the preferred direction for future work.

## Purpose

Tank is a Unix-style tool. Its UI should feel:

- Direct
- Deterministic
- Compact
- Readable in a terminal without decoration-heavy output
- Friendly to both humans and shell usage

The CLI should expose the filesystem-driven model clearly, without introducing extra configuration vocabulary or abstract resource layers.

## Scope

This guide covers:

- Terminal display conventions
- Shared style primitives
- Tables, progress, and status output
- Command and subcommand structure
- Positional arguments
- Flag naming and behavior
- Error and confirmation patterns

This guide does not replace the architecture document in [`docs/DESIGN.md`](./docs/DESIGN.md). That file explains how Tank works internally; this file explains how Tank should present itself externally.

## Design Principles

- Prefer clear output over dense output.
- Prefer one obvious command over many overlapping commands.
- Prefer verbs for actions and grouped subcommands for managed resources.
- Prefer stable wording for shared concepts such as build, instance, layer, volume, cache, and base.
- Prefer plain text that remains understandable when ANSI styling is absent.
- Prefer progressive disclosure: show the summary first, then details only when they help.
- Prefer output that is useful in the common case without requiring extra flags.

## Source of Truth

Current shared UI behavior lives in:

- [`ui/styles.go`](./ui/styles.go)
- [`ui/table.go`](./ui/table.go)
- [`ui/progress.go`](./ui/progress.go)

Current CLI structure lives in:

- [`cmd/tank/main.go`](./cmd/tank/main.go)
- [`cmd/tank/*.go`](./cmd/tank)

When adding new interactive output, prefer extending the shared `ui` package instead of introducing one-off formatting in individual commands.

## Terminal Display System

### Shared visual language

Tank uses a small semantic palette rather than per-command styling decisions:

- `Primary`: titles and emphasized headings
- `Secondary`: informational highlights
- `Success`: successful completion and healthy state
- `Warning`: degraded or reclaimable state
- `Error`: failures
- `Muted`: secondary metadata, hashes, empty states, and non-primary context

The exact colors live in [`ui/styles.go`](./ui/styles.go). New output should use semantic styles, not raw colors.

### Shared symbols

Tank already uses a compact symbol set:

- `✓` success
- `✗` error
- `!` warning
- `→` info / forward progress
- `•` detail item
- `● running` / `○ stopped` for instance state

These symbols are part of the design language. New features should reuse them before inventing new glyphs.

### Message types

Tank has four recurring line-level message forms:

- Header: section title, optionally with an underline
- Info: `→ message`
- Success: `✓ message`
- Error: `✗ message`
- Step/detail: indented `• message`

Use the shared helpers in [`ui/progress.go`](./ui/progress.go):

- `PrintHeader`
- `PrintInfo`
- `PrintSuccess`
- `PrintError`
- `PrintStep`

### Tone and copy

Output copy should be:

- Short
- Literal
- Specific about the resource being changed
- Written in sentence case

Preferred examples:

- `Creating instance myproject`
- `Build cached 1a2b3c4d`
- `No instances found`
- `Waiting for SSH`

Avoid:

- Marketing language
- Jokes
- Exclamation-heavy success messages
- Vague labels like `Done` without context

### Empty states

Empty states should be explicit, muted, and terminal-friendly:

- `No instances found`
- `No layers`
- `No volumes found`
- `No volumes found for this project`

Do not emit blank output for empty results.

### Labels and sections

Status-style output uses bold labels followed by values:

- `Project:`
- `Base:`
- `Instance:`
- `State:`
- `Build:`

Preferred rules:

- Use Title Case for labels
- Keep labels stable across commands and docs
- Put the important value immediately after the label
- Use muted styling for paths and hashes, highlight styling for user-selected resources

### Hashes and identifiers

Short hashes are a repeated pattern in the codebase. The current convention is:

- Show full hashes only when precision matters
- Otherwise show the first 8 characters
- Render hashes in muted style

Use `ui.FormatHash` when possible.

### Tables

Tabular output is the preferred format for lists of peer resources. Current tables use:

- Uppercase headers
- One row per resource
- Borders with muted styling
- Normalized column order

Existing examples:

- Instance table
- Layer table
- Volume table

Rules for future tables:

- Use tables for lists; use labeled sections for single-resource inspection
- Keep headers short and stable
- Put the primary identifier in the first column
- Put status near the front
- Put low-signal metadata such as paths at the end

### Progress and waiting

Tank has three progress patterns today:

- Step-by-step logs
- Byte progress bars for downloads
- Spinner/wait loops for polling external readiness

Use:

- `PrintStep` for deterministic multi-step work where each step is meaningful
- A progress bar for long byte-oriented transfers
- A spinner for polling external state such as IP assignment or SSH readiness

Rules:

- Waiting messages should describe what is being awaited, not how the polling works
- Long waits should include elapsed time when possible
- Completion should collapse to a success or error line
- Progress output should be concise and not spam repeated lines unnecessarily

### Stdout vs stderr

Preferred convention:

- Final results and ordinary human-readable command output go to stdout
- Active waiting UI, spinners, and error presentation may use stderr

This lets stdout remain useful for piping or capture when practical.

### Machine-readable output

Inspection commands should be able to emit stable machine-readable output without reusing the human presentation layer.

Rules:

- Human-readable output remains the default
- Machine-readable output should use `--json`
- JSON output is experimental until the schema is explicitly versioned or declared stable
- JSON mode should write structured data only, with no styling or extra prose
- JSON field names should use snake_case
- JSON output should preserve the same resource vocabulary as the human CLI

Good candidates for `--json` are:

- `status`
- `list`
- `layers`
- `volume list`

## Command Design System

### Overall command shape

Tank uses a flat top-level command set for common lifecycle actions and grouped subcommands for managed resources:

- Flat verbs for common actions: `start`, `stop`, `destroy`, `build`, `ssh`, `status`, `init`
- Grouped noun for managed resource families: `volume`

This is the right model to continue using.

Rules:

- Use a top-level verb when the action is common and maps directly to the default project or instance workflow
- Use a grouped noun when the user is managing a collection of secondary resources with multiple actions
- Avoid adding synonyms as separate commands when aliases are sufficient

### Subcommands

Subcommands should follow these patterns:

- Resource groups should be singular nouns: `volume`, not `volumes`
- Actions under a resource should be short verbs: `list`, `rm`
- Aliases are acceptable when they match shell norms: `ls`, `ps`

Preferred structure:

```text
tank <verb>
tank <resource> <verb>
```

Examples:

- `tank start`
- `tank status`
- `tank volume list`
- `tank volume rm <name>`

### Positional arguments

Tank already uses a strong positional convention:

- Optional `[name]` means "instance name", defaulting to the project directory name
- Required identifiers use `<name>` or `<build-hash>`
- SSH passthrough arguments appear after `--`

Rules:

- Use at most one primary positional argument for the common path
- Default instance selection should continue to be the project directory name
- If extra arguments belong to a delegated tool, separate them with `--`
- Use Cobra argument validation for exact and required arity

### Naming

Command names should be:

- Lowercase
- Single word when possible
- Familiar Unix verbs
- Specific to Tank's domain vocabulary

Preferred domain nouns:

- `project`
- `instance`
- `build`
- `base`
- `layer`
- `volume`
- `cache`

Avoid inventing adjacent synonyms like `machine`, `vm-image`, `artifact-set`, or `workspace` unless the feature truly needs new vocabulary.

### Help text

Current commands generally follow a good pattern:

- `Use`: concise invocation shape
- `Short`: one-line summary
- `Long`: optional fuller explanation when needed

Preferred rules:

- `Short` should describe the user-visible action, not the implementation
- `Long` should explain defaults or important behavior only when the command has nuance
- `Use` should show optional names as `[name]` and required identifiers as `<name>`
- Mention defaulting behavior in `Long` when positional arguments are optional

## Flag Design System

### Naming

Flags should use long-form kebab-case names:

- `--project`
- `--no-cache`
- `--dry-run`
- `--all`
- `--force`

Rules:

- Use kebab-case, never snake_case or camelCase
- Prefer whole words over abbreviations
- Reserve single-letter shorthand for globally common flags only

Current guidance:

- `-p` for `--project` is acceptable because it is persistent and broadly useful
- New short flags should be added sparingly

### Semantics

Flags should express one of four roles:

- Scope selection: `--project`, `--all`
- Execution mode: `--dry-run`, `--apply`
- Safety override: `--force`
- Cache behavior: `--no-cache`

Rules:

- Reuse an existing flag name when the semantic meaning is the same
- Do not create near-duplicates like `--rebuild`, `--fresh`, and `--ignore-cache` for the same behavior
- Boolean flags should read naturally when present

### Shared meanings

The following names should be standardized:

- `--project`: path to the project directory
- `--no-cache`: do not reuse cached build stages
- `--dry-run`: show intended actions without changing state
- `--apply`: perform the destructive or mutating action after showing that dry-run is the default
- `--all`: include resources outside the default scope
- `--force`: bypass a safety check that would otherwise stop the operation
- `--explain <id>`: explain why a resource is in its current state

### Defaults

Defaults should be safe and unsurprising:

- Destructive operations should not happen by default when a preview mode is reasonable
- Project-scoped listing should be the default when global listing would be noisy
- Cached builds should be reused by default

### Units and descriptions

When a flag value has units:

- Keep the unit out of the flag name unless it materially affects parsing
- State the unit clearly in the help text

Examples:

- `--memory` with help text `memory in MB`
- `--cpus` with help text `number of CPUs`

## Output Patterns By Command Type

### Mutating commands

Mutating commands such as `init`, `start`, `stop`, `destroy`, and `volume rm` should:

- Announce what is happening
- Show meaningful intermediate steps when the operation takes time
- End with a clear success or error outcome

Preferred pattern:

1. Info line for the top-level action
2. Step lines for meaningful work
3. Success line on completion

### Inspection commands

Inspection commands such as `status`, `layers`, `ls`, and `volume ls` should:

- Print the current state directly
- Avoid spinner UI
- Use tables or labeled sections
- Use muted empty states instead of errors when nothing exists
- Offer `--json` when the command naturally returns structured state

### Explain-style commands

Commands like `prune --explain` should:

- Print a small header
- Show the resource identifier
- Show its state
- Show the reason or path if available

This is a good pattern for future diagnostic subcommands.

## Confirmation and Destructive Operations

Destructive actions should require one of:

- An explicit confirmation prompt
- A deliberate `--apply` mode
- A `--force` override when the danger is contextual rather than universal

Current examples:

- `tank prune` defaults to dry-run; `--apply` performs deletion
- `tank volume rm` asks for confirmation; `--force` bypasses attachment safety

Rules:

- If a command can delete user data, make the safe path the default
- Confirmation prompts should name the specific resource being deleted
- Cancellation should be explicit and quiet

## Error Handling

Tank error handling already follows a good internal pattern:

- Wrap lower-level failures with context using `%w`
- Name the failing operation directly

Examples:

- `loading project`
- `starting VM`
- `collecting volumes`

Rules:

- Errors should identify the operation that failed
- Do not hide the underlying error
- Preflight failures should present actionable hints, not only fatal messages
- Non-error empty states should not be reported as failures

## Recommended Implementation Rules

When adding a new feature:

1. Put shared presentation logic in `ui/` if more than one command could reuse it.
2. Reuse existing semantic styles and symbols.
3. Prefer `PrintInfo`, `PrintStep`, `PrintSuccess`, and `PrintError` over raw `fmt.Printf`.
4. Use tables for lists and labeled sections for one-object inspection.
5. Add the command as a top-level verb only if it matches the main lifecycle.
6. Otherwise add it under a singular resource group.
7. Reuse standard flags before introducing new ones.
8. Make destructive behavior opt-in or confirmed.

## Current Gaps To Normalize

The current codebase is close to a coherent CLI system, but a few mismatches should converge over time:

- Some commands still print raw text rather than using shared UI helpers.
- README examples and some older design notes describe flags or defaults that no longer match the code.
- `build/build.go` contains duplicated local styling that should eventually converge with the shared `ui` package.
- Listing should be documented as `list` while continuing to support `ls` and `ps` aliases where they fit shell expectations.

This document is the preferred direction for future cleanup.

## Canonical Style Summary

- Semantic styles, not ad hoc colors
- Compact symbols reused consistently
- Sentence-case human output
- Uppercase table headers
- Top-level verbs for core lifecycle
- Singular grouped nouns for managed resources
- Kebab-case long flags
- Sparse short flags
- Safe defaults for destructive operations
- Stable vocabulary across commands, docs, and output
