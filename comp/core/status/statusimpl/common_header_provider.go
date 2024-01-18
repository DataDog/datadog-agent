// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package statusimpl

import (
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
	"github.com/DataDog/datadog-agent/comp/core/status"

	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/version"
)

var nowFunc = time.Now
var startTimeProvider = pkgconfigsetup.StartTime

type headerProvider struct {
	data                   map[string]interface{}
	name                   string
	textTemplatesFunctions textTemplate.FuncMap
	htmlTemplatesFunctions htmlTemplate.FuncMap
}

func (h *headerProvider) Index() int {
	return 0
}

func (h *headerProvider) Name() string {
	return h.name
}

func (h *headerProvider) JSON(stats map[string]interface{}) error {
	for k, v := range h.data {
		stats[k] = v
	}

	return nil
}

func (h *headerProvider) Text(buffer io.Writer) error {
	tmpl, tmplErr := templatesFS.ReadFile(path.Join("templates", "text.tmpl"))
	if tmplErr != nil {
		return tmplErr
	}
	t := textTemplate.Must(textTemplate.New("header").Funcs(h.textTemplatesFunctions).Parse(string(tmpl)))
	return t.Execute(buffer, h.data)
}

func (h *headerProvider) HTML(buffer io.Writer) error {
	tmpl, tmplErr := templatesFS.ReadFile(path.Join("templates", "html.tmpl"))
	if tmplErr != nil {
		return tmplErr
	}
	t := htmlTemplate.Must(htmlTemplate.New("header").Funcs(h.htmlTemplatesFunctions).Parse(string(tmpl)))
	return t.Execute(buffer, h.data)
}

func newCommonHeaderProvider(params status.Params, config config.Component) status.HeaderProvider {

	data := map[string]interface{}{}
	data["version"] = version.AgentVersion
	data["flavor"] = params.Flavor
	data["conf_file"] = config.ConfigFileUsed()
	data["pid"] = os.Getpid()
	data["go_version"] = runtime.Version()
	data["agent_start_nano"] = startTimeProvider.UnixNano()
	pythonVersion := params.PythonVersion
	data["python_version"] = strings.Split(pythonVersion, " ")[0]
	data["build_arch"] = runtime.GOARCH
	data["time_nano"] = nowFunc().UnixNano()
	data["config"] = populateConfig(config)

	return &headerProvider{
		data:                   data,
		name:                   fmt.Sprintf("%s (v%s)", params.Flavor, data["version"]),
		textTemplatesFunctions: status.TextFmap(),
		htmlTemplatesFunctions: status.HTMLFmap(),
	}
}

func populateConfig(config config.Component) map[string]string {
	conf := make(map[string]string)
	conf["log_file"] = config.GetString("log_file")
	conf["log_level"] = config.GetString("log_level")
	conf["confd_path"] = config.GetString("confd_path")
	conf["additional_checksd"] = config.GetString("additional_checksd")

	conf["fips_enabled"] = config.GetString("fips.enabled")
	conf["fips_local_address"] = config.GetString("fips.local_address")
	conf["fips_port_range_start"] = config.GetString("fips.port_range_start")

	return conf
}
