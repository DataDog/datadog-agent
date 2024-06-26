// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build otlp

// Package collector implements the collector component
package collector

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	flaretypes "github.com/DataDog/datadog-agent/comp/core/flare/types"
	extension "github.com/DataDog/datadog-agent/comp/otelcol/extension/def"
	"github.com/gocolly/colly/v2"
)

func (c *collectorImpl) fillFlare(fb flaretypes.FlareBuilder) error {
	if !c.config.GetBool("otelcollector.enabled") {
		fb.AddFile("otel/otel-agent.log", []byte("'otelcollector.enabled' is disabled in the configuration"))
		return nil
	}

	// request config from Otel-Agent
	responseBytes, err := c.requestOtelConfigInfo(c.config.GetString("otelcollector.extension_url"))
	if err != nil {
		fb.AddFile("otel/otel-agent.log", []byte(fmt.Sprintf("did not get otel-agent configuration: %v", err)))
		return nil
	}

	// add raw response to flare, and unmarshal it
	fb.AddFile("otel/otel-response.json", responseBytes)
	var responseInfo extension.Response
	if err := json.Unmarshal(responseBytes, &responseInfo); err != nil {
		fb.AddFile("otel/otel-agent.log", []byte(fmt.Sprintf("could not read sources from otel-agent response: %s, error: %v", responseBytes, err)))
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
		sourceURL := src.URL
		if !strings.HasPrefix(sourceURL, "http://") && !strings.HasPrefix(sourceURL, "https://") {
			sourceURL = fmt.Sprintf("http://%s", sourceURL)
		}
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
		fb.AddFile(fmt.Sprintf("otel/otel-flare/%s.dat", name), data)

		if !src.Crawl {
			continue
		}

		// crawl the url by following any hyperlinks
		col := colly.NewCollector()
		col.OnHTML("a", func(e *colly.HTMLElement) {
			// visit all links
			link := e.Attr("href")
			if err := e.Request.Visit(e.Request.AbsoluteURL(link)); err != nil {
				filename := strings.ReplaceAll(url.PathEscape(link), ":", "_")
				fb.AddFile(fmt.Sprintf("otel/otel-flare/crawl-%s.err", filename), []byte(err.Error()))
			}
		})
		col.OnResponse(func(r *colly.Response) {
			// the root sources (from the extension.Response) were already fetched earlier
			// don't re-fetch them
			responseURL := r.Request.URL.String()
			if responseURL == src.URL {
				return
			}
			// use the url as the basis for the filename saved in the flare
			filename := strings.ReplaceAll(url.PathEscape(responseURL), ":", "_")
			fb.AddFile(fmt.Sprintf("otel/otel-flare/crawl-%s", filename), r.Body)
		})
		if err := col.Visit(sourceURL); err != nil {
			fb.AddFile("otel/otel-flare/crawl.err", []byte(err.Error()))
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

// Can be overridden for tests
var overrideConfigResponse = ""

func (c *collectorImpl) requestOtelConfigInfo(endpointURL string) ([]byte, error) {
	// Value to return for tests
	if overrideConfigResponse != "" {
		return []byte(overrideConfigResponse), nil
	}

	tr := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}
	client := &http.Client{Transport: tr}

	ctx := context.TODO()
	req, err := http.NewRequestWithContext(ctx, "GET", endpointURL, nil)
	if err != nil {
		return nil, err
	}

	res, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	body, err := io.ReadAll(res.Body)
	res.Body.Close()
	if err != nil {
		return nil, err
	}
	if res.StatusCode >= 400 {
		return nil, fmt.Errorf("%s", body)
	}
	return body, nil
}
