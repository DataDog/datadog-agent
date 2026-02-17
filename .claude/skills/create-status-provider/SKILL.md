---
name: create-status-provider
description: Add a new section to the agent status output (agent status command)
allowed-tools: Bash, Read, Write, Edit, Glob, Grep, AskUserQuestion
argument-hint: "[provider-name]"
---

Add a new status provider to the Datadog Agent. Status providers contribute sections to the `agent status` output in JSON, plain text, and HTML formats.

## Provider Types

There are two provider interfaces defined in `comp/core/status/component.go`:

- **Provider** — Regular status section. Grouped by `Section()`, sorted alphabetically by `Name()` within each section.
- **HeaderProvider** — Appears at the top of the status output, ordered by `Index()`. Used for critical agent-wide info (hostname, version, etc.). Rarely needed for new features.

## Instructions

### Step 1: Gather information from the user

Use `AskUserQuestion` to collect the following. If `$ARGUMENTS` provides the provider name, skip that question.

1. **Provider name**: Display name for this status section (e.g. `"DogStatsD"`, `"Forwarder"`, `"APM Agent"`).

2. **Section name**: Grouping key for the section. Providers with the same section are grouped together. Often matches the provider name. Note: `"collector"` is a special section that always appears first.

3. **What data to display**: What information should this status section show? (metrics, state, counts, configuration values, etc.)

4. **Placement**: Where should the provider live?
   - **Inside an existing component** — Add to the component's `Provides` struct (most common)
   - **Standalone module** — Create a new status sub-package (for larger/independent status sections)

5. **Conditional?**: Should the provider only appear when a feature is enabled?

### Step 2: Implement the status provider

#### File structure

For a standalone status module:
```
comp/<bundle>/<component>/status/
├── statusimpl/
│   ├── status.go
│   └── status_templates/
│       ├── <name>.tmpl        # Text template
│       └── <name>HTML.tmpl    # HTML template
```

For adding to an existing component, create the templates directory alongside the implementation and add the provider to the existing `Provides` struct.

#### Provider implementation

```go
// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package statusimpl

import (
	"embed"
	"io"

	"github.com/DataDog/datadog-agent/comp/core/status"
)

//go:embed status_templates
var templatesFS embed.FS

type statusProvider struct {
	// Add fields for data sources (config, component references, etc.)
}

func (s statusProvider) Name() string {
	return "<ProviderName>"
}

func (s statusProvider) Section() string {
	return "<SectionName>"
}

func (s statusProvider) JSON(_ bool, stats map[string]interface{}) error {
	s.populateStatus(stats)
	return nil
}

func (s statusProvider) Text(_ bool, buffer io.Writer) error {
	return status.RenderText(templatesFS, "<name>.tmpl", buffer, s.getStatusInfo())
}

func (s statusProvider) HTML(_ bool, buffer io.Writer) error {
	return status.RenderHTML(templatesFS, "<name>HTML.tmpl", buffer, s.getStatusInfo())
}

func (s statusProvider) getStatusInfo() map[string]interface{} {
	stats := make(map[string]interface{})
	s.populateStatus(stats)
	return stats
}

func (s statusProvider) populateStatus(stats map[string]interface{}) {
	// Gather data and populate the stats map
	// Example:
	// stats["myFeatureStats"] = map[string]interface{}{
	//     "enabled": true,
	//     "count":   42,
	// }
}
```

**Key points:**
- `JSON()` populates an existing `stats` map (shared across all providers)
- `Text()` and `HTML()` render into a buffer using templates
- `getStatusInfo()` creates a fresh map for template rendering
- `populateStatus()` is shared between JSON and template rendering
- The `verbose` parameter can be used to include extra detail

#### Conditional provider

To conditionally include a provider (e.g., only when a feature is enabled):

```go
func newProvider(config config.Component) status.Provider {
	if !config.GetBool("my_feature.enabled") {
		return nil // Returning nil excludes the provider from status output
	}
	return statusProvider{config: config}
}
```

### Step 3: Create text and HTML templates

Templates are stored in a `status_templates/` directory and embedded via `//go:embed`.

#### Text template (`status_templates/<name>.tmpl`)

```
{{- if .myFeatureStats -}}
==========
My Feature
==========
  Enabled: {{.myFeatureStats.enabled}}
  Items processed: {{humanize .myFeatureStats.count}}
  {{- if .myFeatureStats.lastError}}
  Last Error: {{.myFeatureStats.lastError}}
  {{- end}}

{{- end -}}
```

#### HTML template (`status_templates/<name>HTML.tmpl`)

```
{{- if .myFeatureStats -}}
<div class="stat">
  <span class="stat_title">My Feature</span>
  <span class="stat_data">
    Enabled: {{.myFeatureStats.enabled}}<br>
    Items processed: {{humanize .myFeatureStats.count}}<br>
    {{- if .myFeatureStats.lastError}}
    Last Error: {{.myFeatureStats.lastError}}<br>
    {{- end}}
  </span>
</div>
{{- end -}}
```

#### Available template helper functions

Defined in `comp/core/status/render_helpers.go`:

| Function | Description |
|---|---|
| `humanize` | Format large numbers with commas |
| `humanizeDuration` | Format durations readably |
| `formatUnixTime` | Format Unix timestamp |
| `formatTitle` | Split camelCase into words |
| `status` | Format check status with colors |
| `redText`, `yellowText`, `greenText` | Colored text (text templates) |
| `formatJSON` | Pretty-print a value as JSON |
| `percent` | Format as percentage |
| `printDashes` | Print separator dashes |
| `add` | Add two integers |
| `doNotEscape` | Mark HTML as safe (HTML templates only) |

### Step 4: Register the provider

#### Option A: Add to an existing component's Provides struct

This is the most common pattern. Add `status.InformationProvider` to the component's `Provides`:

```go
import "github.com/DataDog/datadog-agent/comp/core/status"

type Provides struct {
	fx.Out

	Comp           mycomponent.Component
	StatusProvider status.InformationProvider
}

func NewComponent(deps Requires) (Provides, error) {
	instance := &myImpl{/* ... */}
	return Provides{
		Comp:           instance,
		StatusProvider: status.NewInformationProvider(statusProvider{/* deps */}),
	}, nil
}
```

The `status.InformationProvider` wrapper uses fx group injection (`group:"status"`) so it's automatically collected by the status component.

#### Option B: Standalone status module

Create a dedicated module function:

```go
package statusimpl

import (
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/core/status"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

func Module() fxutil.Module {
	return fxutil.Component(
		fx.Provide(newProvider),
	)
}

type dependencies struct {
	fx.In
	Config config.Component
}

type provides struct {
	fx.Out
	StatusProvider status.InformationProvider
}

func newProvider(deps dependencies) provides {
	return provides{
		StatusProvider: status.NewInformationProvider(statusProvider{config: deps.Config}),
	}
}
```

Then add `statusimpl.Module()` to the relevant bundle's `Bundle()` function.

### Step 5: Verify

1. Build:
   ```bash
   dda inv agent.build --build-exclude=systemd
   ```

2. Run the linter:
   ```bash
   dda inv linter.go
   ```

3. Report the results to the user. If the build or linting fails, fix the issues.

## Important Notes

- The `status.RenderText` and `status.RenderHTML` functions read templates from the embedded filesystem at path `status_templates/<filename>`. The directory name must be exactly `status_templates`.
- Template data is always `map[string]interface{}`. Use a top-level key (e.g. `"myFeatureStats"`) to namespace your data within the shared stats map.
- The `verbose` bool parameter is passed to all three methods. Use it to conditionally include more detail.
- Sections are sorted alphabetically, except `"collector"` which always appears first.
- Providers within the same section are sorted alphabetically by `Name()`.
- For `HeaderProvider`, use `NewHeaderInformationProvider()` instead and the `Index()` method controls ordering (lower = higher on page).

## Usage

- `/create-status-provider` — Interactive: prompts for all details
- `/create-status-provider "My Feature"` — Pre-fills the provider name
