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

	"github.com/gocolly/colly/v2"

	"github.com/DataDog/datadog-agent/comp/core/config"
	flaretypes "github.com/DataDog/datadog-agent/comp/core/flare/types"
	corelog "github.com/DataDog/datadog-agent/comp/core/log"
	"github.com/DataDog/datadog-agent/comp/core/status"
	"github.com/DataDog/datadog-agent/comp/core/tagger"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	"github.com/DataDog/datadog-agent/comp/metadata/inventoryagent"
	collector "github.com/DataDog/datadog-agent/comp/otelcol/collector/def"
	extension "github.com/DataDog/datadog-agent/comp/otelcol/extension/def"
	"github.com/DataDog/datadog-agent/comp/otelcol/logsagentpipeline"
	"github.com/DataDog/datadog-agent/comp/otelcol/otlp"
	"github.com/DataDog/datadog-agent/comp/otelcol/otlp/datatype"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/serializer"
	"github.com/DataDog/datadog-agent/pkg/util/optional"
)

const (
	otlpEnabled = "feature_otlp_enabled"
)

// Requires specifies a list of dependencies required for the collector
// to be instantiated.
type Requires struct {
	// Lc specifies the lifecycle settings, used for appending startup
	// and shutdown hooks.
	Lc compdef.Lifecycle

	// Config specifies the Datadog Agent's configuration component.
	Config config.Component

	// Log specifies the logging component.
	Log corelog.Component

	// Serializer specifies the metrics serializer that is used to export metrics
	// to Datadog.
	Serializer serializer.MetricSerializer

	// LogsAgent specifies a logs agent
	LogsAgent optional.Option[logsagentpipeline.Component]

	// InventoryAgent require the inventory metadata payload, allowing otelcol to add data to it.
	InventoryAgent inventoryagent.Component

	Tagger tagger.Component
}

// Provides specifics the types returned by the constructor
type Provides struct {
	compdef.Out

	Comp           collector.Component
	FlareProvider  flaretypes.Provider
	StatusProvider status.InformationProvider
}

type collectorImpl struct {
	col            *otlp.Pipeline
	config         config.Component
	log            corelog.Component
	serializer     serializer.MetricSerializer
	logsAgent      optional.Option[logsagentpipeline.Component]
	inventoryAgent inventoryagent.Component
	tagger         tagger.Component
}

func (c *collectorImpl) start(context.Context) error {
	on := otlp.IsEnabled(c.config)
	c.inventoryAgent.Set(otlpEnabled, on)
	if !on {
		return nil
	}
	var logch chan *message.Message
	if v, ok := c.logsAgent.Get(); ok {
		if provider := v.GetPipelineProvider(); provider != nil {
			logch = provider.NextPipelineChan()
		}
	}
	var err error
	col, err := otlp.NewPipelineFromAgentConfig(c.config, c.serializer, logch, c.tagger)
	if err != nil {
		// failure to start the OTLP component shouldn't fail startup
		c.log.Errorf("Error creating the OTLP ingest pipeline: %v", err)
		return nil
	}
	c.col = col
	// the context passed to this function has a startup deadline which
	// will shutdown the collector prematurely
	ctx := context.Background()
	go func() {
		if err := col.Run(ctx); err != nil {
			c.log.Errorf("Error running the OTLP ingest pipeline: %v", err)
		}
	}()
	return nil
}

func (c *collectorImpl) stop(context.Context) error {
	if c.col != nil {
		c.col.Stop()
	}
	return nil
}

func (c *collectorImpl) fillFlare(fb flaretypes.FlareBuilder) error {
	if !c.config.GetBool("otel.enabled") {
		fb.AddFile("otel/otel-agent.log", []byte("'otel.enabled' is disabled in the configuration"))
		return nil
	}

	// request config from Otel-Agent
	responseBytes, err := c.requestOtelConfigInfo(c.config.GetString("otel.extension_url"))
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

// Status returns the status of the collector.
func (c *collectorImpl) Status() datatype.CollectorStatus {
	return c.col.GetCollectorStatus()
}

// NewComponent creates a new Component for this module and returns any errors on failure.
func NewComponent(reqs Requires) (Provides, error) {
	collector := &collectorImpl{
		config:         reqs.Config,
		log:            reqs.Log,
		serializer:     reqs.Serializer,
		logsAgent:      reqs.LogsAgent,
		inventoryAgent: reqs.InventoryAgent,
		tagger:         reqs.Tagger,
	}

	reqs.Lc.Append(compdef.Hook{
		OnStart: collector.start,
		OnStop:  collector.stop,
	})

	return Provides{
		Comp:           collector,
		FlareProvider:  flaretypes.NewProvider(collector.fillFlare),
		StatusProvider: status.NewInformationProvider(collector),
	}, nil
}
