# Add a component status provider

Register a status provider to include your component's information in the Agent status output.

## Add the provider

To add a status provider to your component, you need to declare it in the return value of its `NewComponent()` function.

Choose the provider interface and rendering helpers from the [status provider reference](../../reference/components/status.md).

The `embed.FS` variable points to the location of the different status templates. These templates must be inside the component files. The folder must be named `status_templates`. The name of the templates do not have any rules, but to keep the same consistency across the code, we suggest using `"<component>.tmpl"` for the text template and `"<component>HTML.tmpl"` for the HTML template.

Below is an example of adding a status provider to your component.

/// tab | :octicons-file-code-16: comp/compression/impl/compressor.go
```go
package impl

import (
    "fmt"

    compression "github.com/DataDog/datadog-agent/comp/compression/def"
    "github.com/DataDog/datadog-agent/comp/status"
)

type Requires struct {
}

type Provides struct {
    Comp compression.Component
    Status status.InformationProvider
}

type compressor struct {
}

// NewComponent returns an implementation for the compression component
func NewComponent(reqs Requires) Provides {
    comp := &compressor{}

    return Provides{
        Comp: comp,
        Status: status.NewInformationProvider(&comp)
    }
}

//
// Since we are using the compressor as status provider we need to implement the status interface on our component
//

//go:embed status_templates
var templatesFS embed.FS

// Name renders the name
func (c *compressor) Name() string {
    return "Compression"
}

// Index renders the index
func (c *compressor) Section() int {
    return "Compression"
}

// JSON populates the status map
func (c *compressor) JSON(_ bool, stats map[string]interface{}) error {
    c.populateStatus(stats)

    return nil
}

// Text renders the text output
func (c *compressor) Text(_ bool, buffer io.Writer) error {
    return status.RenderText(templatesFS, "compressor.tmpl", buffer, c.getStatusInfo())
}

// HTML renders the html output
func (c *compressor) HTML(_ bool, buffer io.Writer) error {
    return status.RenderHTML(templatesFS, "compressorHTML.tmpl", buffer, c.getStatusInfo())
}

func (c *compressor) populateStatus(stats map[string]interface{}) {
    // Here we populate whatever informatiohn we want to display for our component
    stats["compressor"] = ...
}

func (c *compressor) getStatusInfo() map[string]interface{} {
    stats := make(map[string]interface{})

    c.populateStatus(stats)

    return stats
}
```
///

## Testing

A critical part of your component development is ensuring that the status output is displayed as expected is. We highly encourage you to add tests to your components, giving you the confidence that your status output is accurate and reliable. For our example above, testing the status output is as easy as testing the result of calling `JSON`, `Text` and `HTML`.

/// tab | :octicons-file-code-16: comp/compression/impl/component_test.go
```go
package impl

import (
    "bytes"
    "testing"
)

func TestText(t *testing.T) {
    requires := Requires{}

    provides := NewComponent(requires)
    component := provides.Comp
    buffer := new(bytes.Buffer)

    result, err := component.Text(false, buffer)
    assert.Nil(t, err)

    assert.Equal(t, ..., string(result))
}

func TestJSON(t *testing.T) {
    requires := Requires{}

    provides := NewComponent(requires)
    component := provides.Comp
    info := map[string]interface{}

    result, err := component.JSON(false, info)
    assert.Nil(t, err)

    assert.Equal(t, ..., result["compressor"])
}
```
///

To complete testing, we encourage adding the new status section output as part of the e2e tests. The CLI status e2e tests are in `test/new-e2e/tests/agent-subcommands/status` folder.
