// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build otlp

// Package collector implements the collector component
package collector

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/DataDog/datadog-agent/comp/core/config"
	flarebuilder "github.com/DataDog/datadog-agent/comp/core/flare/builder"
	corelog "github.com/DataDog/datadog-agent/comp/core/log"
	"github.com/DataDog/datadog-agent/comp/core/status"
	"github.com/DataDog/datadog-agent/comp/core/tagger"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	"github.com/DataDog/datadog-agent/comp/metadata/inventoryagent"
	collector "github.com/DataDog/datadog-agent/comp/otelcol/collector/def"
	"github.com/DataDog/datadog-agent/comp/otelcol/logsagentpipeline"
	"github.com/DataDog/datadog-agent/comp/otelcol/otlp"
	"github.com/DataDog/datadog-agent/comp/otelcol/otlp/datatype"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/serializer"
	"github.com/DataDog/datadog-agent/pkg/util/optional"
	"github.com/gocolly/colly"
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
	FlareProvider  flarebuilder.Provider
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

func (c *collectorImpl) fillFlare(fb flarebuilder.FlareBuilder) error {
	otelFlareEnabled := c.config.GetBool("otel-agent.enabled")
	otelFlarePort := c.config.GetInt("otel-agent.flare_port")
	if !otelFlareEnabled {
		return nil
	}

	// request config from Otel-Agent
	responseBytes, err := c.requestOtelConfigInfo(otelFlarePort)
	if err != nil {
		fb.AddFile("otel/otel-agent.log", []byte(fmt.Sprintf("did not get otel-agent configuration: %v", err))) //nolint:errcheck
		return nil
	}

	// add raw response to flare, and unmarshal it
	fb.AddFile("otel/otel-response.json", responseBytes) //nolint:errcheck
	var responseInfo map[string]interface{}
	if err := json.Unmarshal(responseBytes, &responseInfo); err != nil {
		return err
	}

	// get list of additional sources to grab data from
	sources, ok := responseInfo["additionalSources"].([]map[string]string)
	if !ok {
		fb.AddFile("otel/otel-agent.log", []byte("could not read additional sources from otel-agent response")) //nolint:errcheck
		return nil
	}

	for _, src := range sources {
		sourceName := src["name"]
		sourceURL := src["url"]
		sourceCrawl := src["crawl"]

		if sourceCrawl != "true" {
			response, err := http.Get(sourceURL)
			if err != nil {
				fb.AddFile(fmt.Sprintf("otel/otel-flare/%s.err", sourceName), []byte(err.Error())) //nolint:errcheck
				continue
			}
			defer response.Body.Close()

			data, err := io.ReadAll(response.Body)
			if err != nil {
				fb.AddFile(fmt.Sprintf("otel/otel-flare/%s.err", sourceName), []byte(err.Error())) //nolint:errcheck
				continue
			}
			fb.AddFile(fmt.Sprintf("otel/otel-flare/%s.dat", sourceName), data) //nolint:errcheck
			continue
		}

		// crawl the url
		col := colly.NewCollector()
		col.OnHTML("a", func(e *colly.HTMLElement) {
			// visit all links
			link := e.Attr("href")
			e.Request.Visit(e.Request.AbsoluteURL(link)) //nolint:errcheck
		})
		col.OnRequest(func(r *colly.Request) {
			// save all visited pages in the flare
			filename := strings.ReplaceAll(url.PathEscape(r.URL.String()), ":", "_")
			data, err := io.ReadAll(r.Body)
			if err != nil {
				fb.AddFile(fmt.Sprintf("otel/otel-flare/%s.err", filename), []byte(err.Error())) //nolint:errcheck
				return
			}
			fb.AddFile(fmt.Sprintf("otel/otel-flare/%s", filename), data) //nolint:errcheck
		})
		if err := col.Visit(sourceURL); err != nil {
			fb.AddFile("otel/otel-flare/crawl.err", []byte(err.Error())) //nolint:errcheck
		}
	}
	return nil
}

const hardCodedConfigResponse = `{
	"startup_configuration": {
		"key1": "value1",
		"key2": "value2",
		"key3": "value3"
	},
	"runtime_configuration": {
		"key4": "value4",
		"key5": "value5",
		"key6": "value6"
	},
	"cmdline": "./otel-agent a b c",
	"sources": [
		{
			"name": "prometheus",
			"url": "http://localhost:5788/data",
			"crawl": "true"
		},
		{
			"name": "zpages",
			"url": "http://localhost:5789/data",
			"crawl": "false"
		},
		{
			"name": "healthcheck",
			"url": "http://localhost:5790/data",
			"crawl": "false"
		},
		{
			"name": "pprof",
			"url": "http://localhost:5791/data",
			"crawl": "true"
		}
	],
	"environment": {
		"DD_KEY7": "value7",
		"DD_KEY8": "value8",
		"DD_KEY9": "value9"
	}
}
`

func (c *collectorImpl) requestOtelConfigInfo(_ int) ([]byte, error) {
	// TODO: (components) In the future, contact the otel-agent flare extension on its configured port
	// it will respond with a JSON response that resembles this format. For now just use this
	// hard-coded response value
	return []byte(hardCodedConfigResponse), nil
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
		FlareProvider:  flarebuilder.NewProvider(collector.fillFlare),
		StatusProvider: status.NewInformationProvider(collector),
	}, nil
}
