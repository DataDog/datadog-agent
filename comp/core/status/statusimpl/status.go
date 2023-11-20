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
	"io"
	"os"
	"path"
	"runtime"
	"strings"
	"text/template"
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
	providers []status.StatusProvider
}

// Module defines the fx options for this component.
var Module = fxutil.Component(
	fx.Provide(newStatus),
)

type headerProvider struct {
	data               map[string]interface{}
	templatesFunctions template.FuncMap
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
	tmpl, tmplErr := templatesFS.ReadFile(path.Join("templates", "header.tmpl"))
	if tmplErr != nil {
		return tmplErr
	}
	t := template.Must(template.New("header").Funcs(h.templatesFunctions).Parse(string(tmpl)))
	return t.Execute(buffer, h.data)
}

func newHeaderProvider(config config.Component) status.StatusProvider {
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
		data:               data,
		templatesFunctions: template.FuncMap{},
	}
}

func newStatus(deps dependencies) (status.Component, error) {
	providers := append([]status.StatusProvider{newHeaderProvider(deps.Config)}, deps.Providers...)

	return &statusImplementation{
		providers: providers,
	}, nil
}

func (s *statusImplementation) GetStatus(format string, verbose bool) ([]byte, error) {
	switch format {
	case "json":
		stats := make(map[string]interface{})
		for _, sc := range s.providers {
			sc.JSON(stats)
		}
		return json.Marshal(stats)
	case "text":
		var b = new(bytes.Buffer)
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

func (s *statusImplementation) GetStatusSection(section, format string, verbose bool) ([]byte, error) {
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
	default:
		return []byte{}, nil
	}
}
