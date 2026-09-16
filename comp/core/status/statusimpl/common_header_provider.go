// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package statusimpl

import (
	"fmt"
	"io"
	"maps"
	"os"
	"path"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/status"

	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/fips"
	htmlTemplate "github.com/DataDog/datadog-agent/pkg/template/html"
	textTemplate "github.com/DataDog/datadog-agent/pkg/template/text"
	"github.com/DataDog/datadog-agent/pkg/util/flavor"
	"github.com/DataDog/datadog-agent/pkg/version"
)

var nowFunc = time.Now
var startTimeProvider = pkgconfigsetup.StartTime

type headerProvider struct {
	constdata              map[string]interface{}
	name                   string
	textTemplatesFunctions textTemplate.FuncMap
	htmlTemplatesFunctions htmlTemplate.FuncMap
	config                 config.Component
	params                 status.Params
}

func (h *headerProvider) Index() int {
	return 0
}

func (h *headerProvider) Name() string {
	return h.name
}

func (h *headerProvider) JSON(_ bool, stats map[string]interface{}) error {
	maps.Copy(stats, h.data())

	return nil
}

func (h *headerProvider) Text(_ bool, buffer io.Writer) error {
	tmpl, tmplErr := templatesFS.ReadFile(path.Join("templates", "text.tmpl"))
	if tmplErr != nil {
		return tmplErr
	}
	t := textTemplate.Must(textTemplate.New("header").Funcs(h.textTemplatesFunctions).Parse(string(tmpl)))
	return t.Execute(buffer, h.data())
}

func (h *headerProvider) HTML(_ bool, buffer io.Writer) error {
	tmpl, tmplErr := templatesFS.ReadFile(path.Join("templates", "html.tmpl"))
	if tmplErr != nil {
		return tmplErr
	}
	t := htmlTemplate.Must(htmlTemplate.New("header").Funcs(h.htmlTemplatesFunctions).Parse(string(tmpl)))
	return t.Execute(buffer, h.data())
}

func (h *headerProvider) data() map[string]interface{} {
	data := maps.Clone(h.constdata)
	data["time_nano"] = nowFunc().UnixNano()
	data["config"] = populateConfig(h.config)
	data["fips_status"] = populateFIPSStatus(h.config)
	pythonVersion := h.params.PythonVersionGetFunc()
	data["python_version"] = strings.Split(pythonVersion, " ")[0]
	return data
}

func newCommonHeaderProvider(params status.Params, config config.Component) status.HeaderProvider {

	data := map[string]interface{}{}
	data["version"] = version.AgentVersion
	data["flavor"] = flavor.GetFlavor()
	data["conf_file"] = config.ConfigFileUsed()
	data["extra_conf_file"] = config.ExtraConfigFilesUsed()
	data["pid"] = os.Getpid()
	data["go_version"] = runtime.Version()
	data["agent_start_nano"] = startTimeProvider.UnixNano()
	data["build_arch"] = runtime.GOARCH

	return &headerProvider{
		constdata:              data,
		name:                   fmt.Sprintf("%s (v%s)", flavor.GetHumanReadableFlavor(), data["version"]),
		textTemplatesFunctions: status.TextFmap(),
		htmlTemplatesFunctions: status.HTMLFmap(),
		config:                 config,
		params:                 params,
	}
}

func populateConfig(config config.Component) map[string]string {
	conf := make(map[string]string)
	conf["log_file"] = config.GetString("log_file")
	conf["log_level"] = config.GetString("log_level")
	conf["confd_path"] = config.GetString("confd_path")
	conf["additional_checksd"] = config.GetString("additional_checksd")

	isFipsAgent, _ := fips.Enabled()
	conf["fips_proxy_enabled"] = strconv.FormatBool(config.GetBool("fips.enabled") && !isFipsAgent)
	conf["fips_local_address"] = config.GetString("fips.local_address")
	conf["fips_port_range_start"] = config.GetString("fips.port_range_start")

	return conf
}

func populateFIPSStatus(config config.Component) string {
	fipsStatus := fips.Status()
	if fipsStatus == "not available" && config.GetString("fips.enabled") == "true" {
		return "proxy"
	}
	return fipsStatus
}
