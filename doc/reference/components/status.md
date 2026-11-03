# Status provider reference

Components can register a status provider. When the status command is executed, we will populate the information displayed using all the status providers.

## Status Providers

There are two types of status providers:

- **Header Providers:** these providers are displayed at the top of the status output. This section is reserved for the most important information about the agent, such as agent version, hostname, host info, or metadata.
- **Regular Providers:** these providers are rendered after all the header providers.

Each provider has the freedom to configure how they want to display their information for the three types of status output: JSON, Text, and HTML. This flexibility allows you to tailor the output to best suit your component's needs.

The JSON and Text outputs are displayed within the status CLI, while the HTML output is used for the Agent GUI.

To guarantee consistent output, we order the status providers internally. The ordering mechanism is different depending on the status provider. We order the header providers based on an index using the ascending direction. The regular providers are ordered alphabetically based on their names.

### Header Providers Interface

```go
type HeaderProvider interface {
    // Index is used to choose the order in which the header information is displayed.
    Index() int
    // Name is rendered as a header in text output.
    Name() string
    JSON(verbose bool, stats map[string]interface{}) error
    Text(verbose bool, buffer io.Writer) error
    HTML(verbose bool, buffer io.Writer) error
}
```

### Regular Providers Interface

```go
// Provider supplies a regular status section.
type Provider interface {
    // Name is used to sort the status providers alphabetically.
    Name() string
    // Section is used to group the status providers.
    // Section is rendered as a header in text output.
    Section() string
    JSON(verbose bool, stats map[string]interface{}) error
    Text(verbose bool, buffer io.Writer) error
    HTML(verbose bool, buffer io.Writer) error
}
```

## Rendering helpers

The status component provides helper functions to create status providers: `NewInformationProvider` and `NewHeaderInformationProvider`.

The `RenderText` and `RenderHTML` helpers render text and HTML output. Both accept the following arguments:

```go
(templateFS embed.FS, template string, buffer io.Writer, data any)
```

See [adding a status provider](../../how-to/components/status.md) for registration, templates, and testing.
