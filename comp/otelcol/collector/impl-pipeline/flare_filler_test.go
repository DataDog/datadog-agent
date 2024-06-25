// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test && otlp
// +build test,otlp

// Package collector implements the collector component
package collector

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync"
	"testing"
	"text/template"

	"github.com/stretchr/testify/require"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/flare/helpers"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

func createFakeOTelExtensionHTTPServer() (string, func()) {
	waitServerDone := &sync.WaitGroup{}
	waitServerDone.Add(1)

	testServerURL := ""
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/one" {
			io.WriteString(w, "data-source-1")
			return
		} else if r.URL.Path == "/two" {
			io.WriteString(w, "data-source-2")
			return
		} else if r.URL.Path == "/three" {
			pageTmpl := `<body>Another source is <a href="%s/secret">here</a></body>`
			io.WriteString(w, fmt.Sprintf(pageTmpl, testServerURL))
			return
		} else if r.URL.Path == "/four" {
			io.WriteString(w, "data-source-4")
			return
		}
		http.NotFound(w, r)
	}))
	testServerURL = ts.URL
	return testServerURL, func() { ts.Close() }
}

var customerConfig = map[string]string{
	"key1": "value1",
	"key2": "value2",
	"key3": "value3",
}

var overrideConfig = map[string]string{
	"key4": "value4",
	"key5": "value5",
	"key6": "value6",
}

var cmdline = "./otel-agent a b c"

var environment = map[string]string{
	"DD_KEY7": "value7",
	"DD_KEY8": "value8",
	"DD_KEY9": "value9",
}

func TestOTelExtFlareBuilder(t *testing.T) {
	localServerURL, shutdown := createFakeOTelExtensionHTTPServer()
	defer shutdown()

	// Override the response that the flare builder gets from the otel extension
	overrideConfigResponseTemplate := `{
	"version": "0.0.1",
	"command": {{.cmdline}},
	"provided_configuration": {{.customerconfig}},
	"environment_variable_configuration": "",
	"runtime_override_configuration": {{.overrideconfig}},
	"full_configuration": "",
	"sources": {
		"prometheus": {
			"url": "{{.url}}/one",
			"crawl": false
		},
		"health_check": {
			"url": "{{.url}}/two",
			"crawl": false
		},
		"zpages": {
			"url": "{{.url}}/three",
			"crawl": true
		},
		"pprof": {
			"url": "{{.url}}/four",
			"crawl": false
		}
	},
	"environment": {{.environment}}
}
`
	tmpl, err := template.New("").Parse(overrideConfigResponseTemplate)
	require.NoError(t, err)
	b := &bytes.Buffer{}
	err = tmpl.Execute(b, map[string]string{
		"url":            localServerURL,
		"customerconfig": strconv.Quote(toJSON(customerConfig)),
		"overrideconfig": strconv.Quote(toJSON(overrideConfig)),
		"cmdline":        strconv.Quote(cmdline),
		"environment":    toJSON(environment),
	})
	require.NoError(t, err)
	overrideConfigResponse = b.String()
	defer func() { overrideConfigResponse = "" }()

	cfg := fxutil.Test[config.Component](t,
		fx.Options(
			config.MockModule(),
		),
	)
	cfg.Set("otel.enabled", true, pkgconfigmodel.SourceAgentRuntime)
	cfg.Set("otel.extension_url", 7777, pkgconfigmodel.SourceAgentRuntime)

	reqs := Requires{
		Lc:     compdef.NewTestLifecycle(),
		Config: cfg,
	}
	provs, _ := NewComponent(reqs)
	col := provs.Comp.(*collectorImpl)

	// Fill the flare
	f := helpers.NewFlareBuilderMock(t, false)
	col.fillFlare(f.Fb)

	f.AssertFileExists("otel", "otel-response.json")

	// Template for the crawable page
	pageTmpl := `<body>Another source is <a href="%s/secret">here</a></body>`

	f.AssertFileContent("data-source-1", "otel/otel-flare/prometheus.dat")
	f.AssertFileContent("data-source-2", "otel/otel-flare/health_check.dat")
	f.AssertFileContent(fmt.Sprintf(pageTmpl, localServerURL), "otel/otel-flare/zpages.dat")
	f.AssertFileContent("data-source-4", "otel/otel-flare/pprof.dat")

	f.AssertFileContent(strconv.Quote(toJSON(customerConfig)), "otel/otel-flare/customer.cfg")
	f.AssertFileContent(strconv.Quote(toJSON(overrideConfig)), "otel/otel-flare/runtime_override.cfg")
	f.AssertFileContent(toJSON(environment), "otel/otel-flare/environment.json")
	f.AssertFileContent(cmdline, "otel/otel-flare/command.txt")
}
