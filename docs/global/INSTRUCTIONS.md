# Documentation

Your goal is to write documentations for packages in this repo.
These documentations will be markdown files named following this pattern: `<package-name>.md`.

About how to write the documentation, keep in mind it should be understandable for a contributor who's not very familiar with the package. Each documentation must contain these sections **in this order**:

1. **TL;DR** — A single blockquote line (`> **TL;DR:** ...`) at the very top summarising what the package does in one sentence. Written for quick scanning: "what is this package for in 10 words?"

2. **Purpose** — 2–4 sentences describing what the package does and why it exists.

3. **Key elements** — The most important Go concepts a contributor must know before working on it. Use H3 subsections to group them:
   - `### Key types` — structs and type aliases
   - `### Key interfaces` — interfaces (omit if none)
   - `### Key functions` — exported functions and constructors (omit if none)
   - `### Configuration and build flags` — config keys, build tags, Cgo, platform notes (omit if none)
   Each entry should have a name and a clear description. **When enriching existing docs: you may rewrite for clarity and improve wording, but never remove meaningful technical details, code examples, or tables. Adding is always good. Shortening is only acceptable for genuinely redundant or bloated text — never for substantive technical content.**

4. **Usage** — How it's used in the codebase. Include a cross-references table for related packages.

# Packages

The packages to check reside in `docs/global/COMP.md` and `docs/global/PKG.md`. Cover all of them.

Put the documentations in `docs/global/comp/<package-name>.md` or `docs/global/pkg/<package-name>.md` depending on the file you got the package name.
If a package is composed of multiple packages, mimic the architecture for the doc files like `pkg/<package-collection>/<package-name>.md`.

# Skippable packages

Some packages don't add useful documentation value and should be skipped. Do not write a doc file for them. A human will review the skipped list afterwards.

Skip a package if it falls into one of these categories:
- **Test infrastructure**: `testutil/`, `testdata/`, `testprogs/`, `dyninsttest/`, packages whose name ends in `test` or `tests`
- **Generated mocks**: `mocks/`, packages whose name ends in `mock` or `mocks`
- **Pure generated code**: packages that only contain protobuf/msgpack generated files with no hand-written logic (e.g. `pbgo/`, `msgpgo/`)
- **Internal implementation details**: `internal/` packages that are only ever imported by their direct parent and expose no concepts worth explaining independently
- **Trivial stubs**: packages with a single file and fewer than ~50 lines that only provide a no-op or constant (e.g. `impl-noop/`)
- **Fixtures and test data**: `fixtures/`, `testdata/`, `resources/` used only in tests

When you skip a package, append its path to `docs/global/SKIPPED.md` with a one-line reason.

# Strategy

**Order matters.** Write foundational packages first so that later documentation can reference and build on them.

Use `docs/global/PKG-USAGE.md` and `docs/global/COMP-USAGE.md` to determine the order. Both files list packages sorted by number of importers (most-used first). Document packages in that order — high importer count means many other docs will want to reference it, so it should exist early.

For every package:
1. Go through the code and write a first draft.
2. Check which other local packages import it (or which packages it imports) to understand usage context.
3. Read the docs of those packages (if already written) to enrich the Usage section and add cross-references. When referencing another package, use its local path as the name, e.g. `pkg/util/log` or `comp/core/config`. No need to hyperlink — just mention the name inline in the text.

# Compaction

If you hit max context capacity at some point, read this doc again to make sure you keep the instructions you need to follow.

# Instruction

Go through all listed packages in the order described in Strategy and write a doc for each one. Skip packages matching the Skippable criteria and log them in `docs/global/SKIPPED.md`.
