---
name: create-status-provider
description: "Adds a new status provider section to the Datadog Agent status command output in JSON, text, and HTML formats. Use when the user asks to extend the agent status output, add a status section, create a status provider, or customize what the agent status command displays."
allowed-tools: Bash, Read, Write, Edit, Glob, Grep, AskUserQuestion
argument-hint: "[provider-name]"
---

Add a new status provider to the Datadog Agent. Status providers contribute sections to the `agent status` output in JSON, plain text, and HTML formats.

## Instructions

### Step 1: Gather information from the user

Use `AskUserQuestion` to collect the following. If `$ARGUMENTS` provides the provider name, skip that question.

1. **Provider name**: Display name for this status section (e.g. `"DogStatsD"`, `"Forwarder"`).
2. **Section name**: Grouping key. Providers with the same section are grouped. `"collector"` always appears first.
3. **What data to display**: What information should this section show?
4. **Placement**: Inside an existing component's `Provides` struct (most common) or standalone module?
5. **Conditional?**: Should the provider only appear when a feature is enabled?

### Step 2: Read reference examples

Before writing any code, read the appropriate reference files to follow existing patterns:

| What | Reference file |
|---|---|
| Provider interface | `comp/core/status/component.go` |
| Provider implementation + templates | `comp/dogstatsd/status/statusimpl/status.go` and its `status_templates/` directory |
| Registration in component Provides | `comp/trace/status/statusimpl/status.go` |
| Template helpers (humanize, etc.) | `comp/core/status/render_helpers.go` |

### Step 3: Implement the provider

Create the provider implementation following the reference. Minimal provider skeleton:

```go
//go:embed status_templates
var templatesFS embed.FS

type myProvider struct{}

func (p myProvider) Name() string      { return "My Feature" }
func (p myProvider) Section() string   { return "my_section" }
func (p myProvider) JSON(_ bool, stats map[string]interface{}) error {
    stats["myFeatureStats"] = collectStats()
    return nil
}
func (p myProvider) Text(_ bool, buffer io.Writer) error {
    return status.RenderText(templatesFS, "myfeature.tmpl", buffer, collectStats())
}
func (p myProvider) HTML(_ bool, buffer io.Writer) error {
    return status.RenderHTML(templatesFS, "myfeatureHTML.tmpl", buffer, collectStats())
}
```

Also create `status_templates/<name>.tmpl` and `status_templates/<name>HTML.tmpl` following the templates in the reference's `status_templates/` directory.

To make a provider conditional, return `nil` from the constructor when the feature is disabled.

### Step 4: Register the provider

Register using `status.NewInformationProvider(yourProvider{...})` — either in an existing component's `Provides` struct (most common) or as a standalone `Module()`. The reference files show both patterns.

### Step 5: Verify

1. Build: `dda inv agent.build --build-exclude=systemd`
2. Lint: `dda inv linter.go`
3. Report the results to the user.

## Important Notes

- Template directory must be named exactly `status_templates` and embedded via `//go:embed status_templates`.
- Use a top-level key (e.g. `"myFeatureStats"`) to namespace data in the shared stats map.
- For `HeaderProvider` (rarely needed), use `NewHeaderInformationProvider()` instead.

## Usage

- `/create-status-provider` — Interactive: prompts for all details
- `/create-status-provider "My Feature"` — Pre-fills the provider name
