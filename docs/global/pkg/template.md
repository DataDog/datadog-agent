# pkg/template

## Purpose

`pkg/template` is a vendored, lightly-patched copy of the Go standard library's `text/template` and `html/template` packages. It exists so the agent codebase can backport or apply fixes to the template engine without waiting for an upstream Go release, while keeping an identical public API surface to the stdlib packages.

The top-level package (`pkg/template`) contains only a doc comment. All usable code lives in the two sub-packages:

| Package | Stdlib equivalent | Use case |
|---------|------------------|----------|
| `pkg/template/text` | `text/template` | Plain-text output: CLI status pages, console rendering |
| `pkg/template/html` | `html/template` | HTML output with context-aware auto-escaping: GUI and web status pages |

An internal helper package `pkg/template/internal/fmtsort` provides stable map-key ordering used during template execution (mirrors `internal/fmtsort` from the Go stdlib).

## Key elements

### `pkg/template/text`

The API is identical to `text/template`. Key types and functions:

| Symbol | Description |
|--------|-------------|
| `Template` | Parsed template. Thread-safe for concurrent execution. |
| `FuncMap` | `map[string]any` mapping template function names to Go functions. |
| `New(name string) *Template` | Allocates a new named template. |
| `(*Template).Parse(text string) (*Template, error)` | Parses template source. |
| `(*Template).Execute(w io.Writer, data any) error` | Renders the template, applying `data` as the dot (`.`) value. |
| `(*Template).ExecuteTemplate(w io.Writer, name string, data any) error` | Renders a named associated template. |
| `(*Template).Funcs(funcMap FuncMap) *Template` | Registers custom template functions. |
| `Must(t *Template, err error) *Template` | Panics on error; useful for package-level `var` initialisations. |
| `ParseFiles(filenames ...string) (*Template, error)` | Parses one or more files. |
| `ParseGlob(pattern string) (*Template, error)` | Parses files matching a glob. |
| `ParseFS(fsys fs.FS, patterns ...string) (*Template, error)` | Parses files from an `fs.FS` (e.g. `embed.FS`). |

Template actions use `{{ }}` delimiters. Predefined functions include `and`, `or`, `not`, `len`, `index`, `slice`, `printf`, `print`, `println`, `call`, `eq`, `ne`, `lt`, `le`, `gt`, `ge`.

### `pkg/template/html`

Wraps `pkg/template/text` and adds context-sensitive HTML, CSS, JavaScript, and URL escaping. The public API mirrors `html/template`:

| Symbol | Description |
|--------|-------------|
| `Template` | HTML-safe template; wraps a `*text.Template` internally. |
| `HTML`, `CSS`, `JS`, `URL` | String-alias types for pre-trusted content that should not be re-escaped. |
| `HTMLEscapeString(s string) string` | Escapes a plain string for safe inclusion in HTML. |
| `FuncMap` | Identical to `text.FuncMap`. |
| `New`, `Must`, `ParseFiles`, `ParseGlob`, `ParseFS` | Same signatures as the text package. |

When data flows through an `html` template action, the engine rewrites pipelines to inject escaping functions appropriate to the current HTML context (element content, attribute value, URL query, JavaScript string, CSS value, etc.).

## Usage

The primary consumer is `comp/core/status`, which renders agent status output in both CLI (text) and GUI (HTML) forms.

```go
// comp/core/status/render_helpers.go

import (
    pkghtmltemplate "github.com/DataDog/datadog-agent/pkg/template/html"
    pkgtexttemplate "github.com/DataDog/datadog-agent/pkg/template/text"
)

// Render an HTML status page from an embedded filesystem
func RenderHTML(templateFS embed.FS, template string, buffer io.Writer, data any) error {
    tmpl, _ := templateFS.ReadFile(path.Join("status_templates", template))
    t := pkghtmltemplate.Must(pkghtmltemplate.New(template).Funcs(HTMLFmap()).Parse(string(tmpl)))
    return t.Execute(buffer, data)
}

// Render a text status page
func RenderText(templateFS embed.FS, template string, buffer io.Writer, data any) error {
    tmpl, _ := templateFS.ReadFile(path.Join("status_templates", template))
    t := pkgtexttemplate.Must(pkgtexttemplate.New(template).Funcs(TextFmap()).Parse(string(tmpl)))
    return t.Execute(buffer, data)
}
```

Other importers include:
- `comp/core/gui` — renders the browser-based GUI.
- `comp/core/secrets` — renders diagnostic output.
- `pkg/fleet/installer` — renders installer status messages.
- `comp/healthplatform` — renders health issue descriptions.

### Choosing between `text` and `html`

- Use `pkg/template/html` whenever the output is embedded in an HTTP response or displayed in a browser. Auto-escaping prevents XSS from injected data.
- Use `pkg/template/text` for CLI/terminal output and configuration file generation, where HTML escaping would corrupt the output.

### Registering custom functions

```go
funcMap := pkgtexttemplate.FuncMap{
    "myHelper": func(v string) string { return "[" + v + "]" },
}
t := pkgtexttemplate.Must(pkgtexttemplate.New("x").Funcs(funcMap).Parse(`{{myHelper .}}`))
```

### Why not import stdlib `html/template` directly?

The agent imports this package instead of the stdlib to allow targeted patches (e.g. security fixes or behavioural adjustments) without being blocked on a Go toolchain upgrade. The patch history can be tracked by diffing against the Go version recorded in `go.mod`.

---

## Related components

- **`comp/core/status`** — the primary consumer. `comp/core/status/render_helpers.go` imports both `pkg/template/html` and `pkg/template/text` to provide `RenderHTML` / `RenderText` helpers and the `HTMLFmap()` / `TextFmap()` function maps used by every status provider. Section providers call these helpers from their `HTML(verbose bool, w io.Writer)` and `Text(verbose bool, w io.Writer)` methods. See [comp/core/status docs](../comp/core/status.md).
- **`comp/core/flare`** — `comp/core/flare/builder` and `comp/core/flare/types` declare `pkg/template` in their `go.mod` replace directives, so flare provider templates rendered as part of archive creation use this vendored engine. See [comp/core/flare docs](../comp/core/flare.md).
- **`comp/core/gui`** — `comp/core/gui/guiimpl/render.go` uses `pkg/template/html` to render the browser-based GUI pages.
- **`comp/core/secrets`** — uses `pkg/template/text` to render diagnostic output for the `secret` subcommand.
- **`pkg/fleet/installer`** — renders installer status messages using `pkg/template/text`.
