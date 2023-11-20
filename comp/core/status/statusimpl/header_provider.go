package statusimpl

import (
	"embed"
	"fmt"
	htmlTemplate "html/template"
	"io"
	"os"
	"path"
	"runtime"
	"strings"
	textTemplate "text/template"
	"time"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/pkg/collector/python"
	pkgConfig "github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/flavor"
	"github.com/DataDog/datadog-agent/pkg/version"
)

type headerProvider struct {
	data                   map[string]interface{}
	textTemplatesFunctions textTemplate.FuncMap
	htmlTemplatesFunctions htmlTemplate.FuncMap
}

func (h headerProvider) Index() int   { return 0 }
func (h headerProvider) Name() string { return "Header" }
func (h headerProvider) JSON(stats map[string]interface{}) {
	for k, v := range h.data {
		stats[k] = v
	}
}

//go:embed templates
var templatesFS embed.FS

func (h headerProvider) Text(buffer io.Writer) error {
	tmpl, tmplErr := templatesFS.ReadFile(path.Join("templates", "text.tmpl"))
	if tmplErr != nil {
		return tmplErr
	}
	t := textTemplate.Must(textTemplate.New("header").Funcs(h.textTemplatesFunctions).Parse(string(tmpl)))
	return t.Execute(buffer, h.data)
}

func (h headerProvider) HTML(buffer io.Writer) error {
	tmpl, tmplErr := templatesFS.ReadFile(path.Join("templates", "html.tmpl"))
	if tmplErr != nil {
		return tmplErr
	}
	t := htmlTemplate.Must(htmlTemplate.New("header").Funcs(h.htmlTemplatesFunctions).Parse(string(tmpl)))
	return t.Execute(buffer, h.data)
}

func (h headerProvider) AppendToHeader(map[string]interface{}) {}

func newHeaderProvider(config config.Component) headerProvider {
	configObject := config.Object()
	data := map[string]interface{}{}
	//  TODO: using globals
	data["version"] = version.AgentVersion
	//  TODO: using globals
	data["flavor"] = flavor.GetFlavor()
	data["conf_file"] = configObject.ConfigFileUsed()
	data["pid"] = os.Getpid()
	data["go_version"] = runtime.Version()
	//  TODO: using globals
	data["agent_start_nano"] = pkgConfig.StartTime.UnixNano()
	//  TODO: using globals
	pythonVersion := python.GetPythonVersion()
	data["python_version"] = strings.Split(pythonVersion, " ")[0]
	data["build_arch"] = runtime.GOARCH
	now := time.Now()
	data["time_nano"] = now.UnixNano()
	// TODO: We need to be able to configure per agent binary
	title := fmt.Sprintf("Agent (v%s)", data["version"])
	data["title"] = title

	return headerProvider{
		data:                   data,
		textTemplatesFunctions: textTemplate.FuncMap{},
		htmlTemplatesFunctions: htmlTemplate.FuncMap{},
	}
}
