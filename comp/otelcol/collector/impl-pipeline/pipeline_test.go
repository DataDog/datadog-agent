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
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
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
	listener, err := net.Listen("tcp", ":0")
	if err != nil {
		panic(err)
	}
	usingPort := listener.Addr().(*net.TCPAddr).Port
	serverURL := fmt.Sprintf("http://localhost:%d", usingPort)

	waitServerDone := &sync.WaitGroup{}
	waitServerDone.Add(1)

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/one" {
			io.WriteString(w, "data-source-1")
			return
		} else if r.URL.Path == "/two" {
			io.WriteString(w, "data-source-2")
			return
		} else if r.URL.Path == "/three" {
			pageTmpl := `<body>Another source is <a href="http://localhost:%d/secret">here</a></body>`
			io.WriteString(w, fmt.Sprintf(pageTmpl, usingPort))
			return
		} else if r.URL.Path == "/four" {
			io.WriteString(w, "data-source-4")
			return
		}
		http.NotFound(w, r)
	})
	srv := &http.Server{Addr: fmt.Sprintf("localhost:%d", usingPort), Handler: mux}

	go func() {
		defer waitServerDone.Done()
		srv.ListenAndServe()
	}()

	shutdowner := func() {
		srv.Shutdown(context.TODO())
	}
	return serverURL, shutdowner
}

var startupConfig = map[string]string{
	"key1": "value1",
	"key2": "value2",
	"key3": "value3",
}

var runtimeConfig = map[string]string{
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
	localServerURL, shutdowner := createFakeOTelExtensionHTTPServer()
	defer shutdowner()

	// Override the response that the flare builder gets from the otel extension
	overrideConfigResponseTemplate := `{
	"startup_configuration": {{.startupconfig}},
	"runtime_configuration": {{.runtimeconfig}},
	"cmdline": {{.cmdline}},
	"sources": [
		{
			"name": "prometheus",
			"url": "{{.url}}/one",
			"crawl": true
		},
		{
			"name": "zpages",
			"url": "{{.url}}/two",
			"crawl": false
		},
		{
			"name": "healthcheck",
			"url": "{{.url}}/three",
			"crawl": true
		},
		{
			"name": "pprof",
			"url": "{{.url}}/four",
			"crawl": true
		}
	],
	"environment": {{.environment}}
}
`
	tmpl, err := template.New("").Parse(overrideConfigResponseTemplate)
	require.NoError(t, err)
	b := &bytes.Buffer{}
	err = tmpl.Execute(b, map[string]string{
		"url":           localServerURL,
		"startupconfig": toJSON(startupConfig),
		"runtimeconfig": toJSON(runtimeConfig),
		"cmdline":       strconv.Quote(cmdline),
		"environment":   toJSON(environment),
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

	// Template for the crawable page
	pageTmpl := `<body>Another source is <a href="%s/secret">here</a></body>`

	f.AssertFileExists("otel", "otel-response.json")
	f.AssertFileContent("data-source-1", "otel/otel-flare/prometheus.dat")
	f.AssertFileContent("data-source-2", "otel/otel-flare/zpages.dat")
	f.AssertFileContent(fmt.Sprintf(pageTmpl, localServerURL), "otel/otel-flare/healthcheck.dat")
	f.AssertFileContent("data-source-4", "otel/otel-flare/pprof.dat")

	f.AssertFileContent(toJSON(startupConfig), "otel/otel-flare/startup.cfg")
	f.AssertFileContent(toJSON(runtimeConfig), "otel/otel-flare/runtime.cfg")
	f.AssertFileContent(toJSON(environment), "otel/otel-flare/environment.cfg")
	f.AssertFileContent(cmdline, "otel/otel-flare/cmdline.txt")
}
