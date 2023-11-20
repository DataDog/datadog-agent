// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package statusimpl

import (
	"bytes"
	"embed"
	"encoding/json"
	"fmt"
	htmlTemplate "html/template"
	"io"
	"os"
	"path"
	"runtime"
	"strings"
	textTemplate "text/template"
	"time"

	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/status"
	"github.com/DataDog/datadog-agent/pkg/collector/python"
	pkgConfig "github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/flavor"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/version"
)

type dependencies struct {
	fx.In
	Config config.Component

	Providers []status.StatusProvider `group:"status"`
}

type statusImplementation struct {
	headerProvider headerProvider
	providers      []status.StatusProvider
}

// Module defines the fx options for this component.
var Module = fxutil.Component(
	fx.Provide(newStatus),
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

func newStatus(deps dependencies) (status.Component, error) {
	// TODO: sort providers by index and name
	return &statusImplementation{
		headerProvider: newHeaderProvider(deps.Config),
		providers:      deps.Providers,
	}, nil
}

func (s *statusImplementation) GetStatus(format string, verbose bool) ([]byte, error) {
	switch format {
	case "json":
		stats := make(map[string]interface{})

		s.headerProvider.JSON(stats)

		for _, sc := range s.providers {
			sc.JSON(stats)
		}
		return json.Marshal(stats)
	case "text":
		var b = new(bytes.Buffer)

		for _, sc := range s.providers {
			sc.AppendToHeader(s.headerProvider.data)
		}

		s.headerProvider.Text(b)

		for _, sc := range s.providers {
			err := sc.Text(b)
			if err != nil {
				return b.Bytes(), err
			}
		}

		return b.Bytes(), nil
	case "html":
		var b = new(bytes.Buffer)

		for _, sc := range s.providers {
			sc.AppendToHeader(s.headerProvider.data)
		}

		s.headerProvider.HTML(b)

		for _, sc := range s.providers {
			err := sc.Text(b)
			if err != nil {
				return b.Bytes(), err
			}
		}
		return b.Bytes(), nil
	default:
		return []byte{}, nil
	}
}

func (s *statusImplementation) GetStatusByName(section, format string, verbose bool) ([]byte, error) {
	var statusSectionProvider status.StatusProvider
	for _, provider := range s.providers {
		if provider.Name() == section {
			statusSectionProvider = provider
			break
		}
	}

	switch format {
	case "json":
		stats := make(map[string]interface{})

		statusSectionProvider.JSON(stats)
		return json.Marshal(stats)
	case "text":
		var b = new(bytes.Buffer)
		err := statusSectionProvider.Text(b)
		if err != nil {
			return b.Bytes(), err
		}
		return b.Bytes(), nil
	case "html":
		var b = new(bytes.Buffer)
		err := statusSectionProvider.HTML(b)
		if err != nil {
			return b.Bytes(), err
		}
		return b.Bytes(), nil
	default:
		return []byte{}, nil
	}
}
