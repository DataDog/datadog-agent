// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build otlp

// Package pipelineimpl implements the collector component
package pipelineimpl

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	flaretypes "github.com/DataDog/datadog-agent/comp/core/flare/types"
	ipchttp "github.com/DataDog/datadog-agent/comp/core/ipc/httphelpers"
	extensiontypes "github.com/DataDog/datadog-agent/comp/otelcol/ddflareextension/types"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

func (c *collectorImpl) fillFlare(fb flaretypes.FlareBuilder) error {
	if !c.config.GetBool("otelcollector.enabled") {
		fb.AddFile("otel/otel-agent.log", []byte("'otelcollector.enabled' is disabled in the configuration"))
		return nil
	}

	// request config from Otel-Agent
	responseBytes, err := c.requestOtelConfigInfo(c.config.GetString("otelcollector.extension_url"))
	if err != nil {
		msg := fmt.Sprintf("did not get otel-agent configuration: %v", err)
		log.Error(msg)
		fb.AddFile("otel/otel-agent.log", []byte(msg))
		return nil
	}

	// add raw response to flare, and unmarshal it
	fb.AddFile("otel/otel-response.json", responseBytes)
	var responseInfo extensiontypes.Response
	if err := json.Unmarshal(responseBytes, &responseInfo); err != nil {
		msg := fmt.Sprintf("could not read sources from otel-agent response: %s, error: %v", responseBytes, err)
		log.Error(msg)
		fb.AddFile("otel/otel-agent.log", []byte(msg))
		return nil
	}
	// BuildInfoResponse
	fb.AddFile("otel/otel-flare/command.txt", []byte(responseInfo.AgentCommand))
	fb.AddFile("otel/otel-flare/ext.txt", []byte(responseInfo.ExtensionVersion))
	fb.AddFile("otel/otel-flare/environment.json", []byte(toJSON(responseInfo.Environment)))
	// ConfigResponse
	fb.AddFile("otel/otel-flare/customer.cfg", []byte(toJSON(responseInfo.CustomerConfig)))
	fb.AddFile("otel/otel-flare/runtime.cfg", []byte(toJSON(responseInfo.RuntimeConfig)))
	fb.AddFile("otel/otel-flare/runtime_override.cfg", []byte(toJSON(responseInfo.RuntimeOverrideConfig)))
	fb.AddFile("otel/otel-flare/env.cfg", []byte(toJSON(responseInfo.EnvConfig)))

	// retrieve each source of configuration
	for name, src := range responseInfo.Sources {
		sourceURLs := src.URLs
		for _, sourceURL := range sourceURLs {
			if !strings.HasPrefix(sourceURL, "http://") && !strings.HasPrefix(sourceURL, "https://") {
				sourceURL = fmt.Sprintf("http://%s", sourceURL)
			}

			urll, err := url.Parse(sourceURL)
			if err != nil {
				fb.AddFile(fmt.Sprintf("otel/otel-flare/%s.err", name), []byte(err.Error()))
				continue
			}

			path := strings.ReplaceAll(urll.Path, "/", "_")
			name := name + path

			response, err := http.Get(sourceURL)
			if err != nil {
				fb.AddFile(fmt.Sprintf("otel/otel-flare/%s.err", name), []byte(err.Error()))
				continue
			}
			defer response.Body.Close()

			data, err := io.ReadAll(response.Body)
			if err != nil {
				fb.AddFile(fmt.Sprintf("otel/otel-flare/%s.err", name), []byte(err.Error()))
				continue
			}

			isOctetStream := false
			if contentTypeSlice, ok := response.Header["Content-Type"]; ok {
				for _, contentType := range contentTypeSlice {
					if contentType == "application/octet-stream" {
						isOctetStream = true
					}
				}
			}

			if isOctetStream {
				fb.AddFileWithoutScrubbing(fmt.Sprintf("otel/otel-flare/%s.dat", name), data)
			} else {
				fb.AddFile(fmt.Sprintf("otel/otel-flare/%s.dat", name), data)
			}

		}
	}
	return nil
}

func toJSON(it interface{}) string {
	data, err := json.Marshal(it)
	if err != nil {
		return err.Error()
	}
	return string(data)
}

var (
	// Can be overridden for tests
	overrideConfigResponse = ""
	// Default timeout for reaching the OTel extension
	defaultExtensionTimeout = 20
)

func (c *collectorImpl) requestOtelConfigInfo(endpointURL string) ([]byte, error) {
	// Value to return for tests
	if overrideConfigResponse != "" {
		return []byte(overrideConfigResponse), nil
	}

	data, err := c.client.Get(endpointURL, ipchttp.WithContext(c.ctx), ipchttp.WithTimeout(c.clientTimeout))
	if err != nil {
		return nil, err
	}

	return data, nil
}
