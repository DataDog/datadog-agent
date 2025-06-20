// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package guiimpl

import (
	"bytes"
	"encoding/json"
	"expvar"
	"io"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/autodiscoveryimpl"
	"github.com/DataDog/datadog-agent/comp/core/status"
	"github.com/DataDog/datadog-agent/pkg/collector"
	"github.com/DataDog/datadog-agent/pkg/collector/check/stats"
	template "github.com/DataDog/datadog-agent/pkg/template/html"
)

var fmap = status.HTMLFmap()

// Data is a struct used for filling templates
type Data struct {
	Name       string
	LoaderErrs map[string]map[string]string
	ConfigErrs map[string]string
	Stats      map[string]interface{}
	CheckStats []*stats.Stats
}

func renderRunningChecks() (string, error) {
	var b = new(bytes.Buffer)

	runnerStatsJSON := []byte(expvar.Get("runner").String())
	runnerStats := make(map[string]interface{})
	if err := json.Unmarshal(runnerStatsJSON, &runnerStats); err != nil {
		return "", err
	}
	loaderErrs := collector.GetLoaderErrors()
	configErrs := autodiscoveryimpl.GetConfigErrors()

	data := Data{LoaderErrs: loaderErrs, ConfigErrs: configErrs, Stats: runnerStats}
	e := fillTemplate(b, data, "runningChecks")
	if e != nil {
		return "", e
	}
	return b.String(), nil
}

func fillTemplate(w io.Writer, data Data, request string) error {
	t := template.New(request + ".tmpl")
	t.Funcs(fmap)
	tmpl, err := templatesFS.ReadFile("views/templates/" + request + ".tmpl")
	if err != nil {
		return err
	}
	t, e := t.Parse(string(tmpl))
	if e != nil {
		return e
	}
	e = t.Execute(w, data)
	return e
}
