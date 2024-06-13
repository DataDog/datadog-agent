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

	"github.com/gocolly/colly/v2"

	"github.com/DataDog/datadog-agent/comp/core/config"
	flaretypes "github.com/DataDog/datadog-agent/comp/core/flare/types"
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
		return nil
	}

	// request config from Otel-Agent
	responseBytes, err := c.requestOtelConfigInfo(c.config.GetInt("otel.extension_url"))
	if err != nil {
		fb.AddFile("otel/otel-agent.log", []byte(fmt.Sprintf("did not get otel-agent configuration: %v", err))) //nolint:errcheck
		return nil
	}

	// add raw response to flare, and unmarshal it
	fb.AddFile("otel/otel-response.json", responseBytes) //nolint:errcheck
	var responseInfo configResponseInfo
	if err := json.Unmarshal(responseBytes, &responseInfo); err != nil {
		fb.AddFile("otel/otel-agent.log", []byte(fmt.Sprintf("could not read sources from otel-agent response: %s", responseBytes))) //nolint:errcheck
		return nil
	}

	fb.AddFile("otel/otel-flare/startup.cfg", []byte(toJSON(responseInfo.StartupConf)))     //nolint:errcheck
	fb.AddFile("otel/otel-flare/runtime.cfg", []byte(toJSON(responseInfo.RuntimeConf)))     //nolint:errcheck
	fb.AddFile("otel/otel-flare/environment.cfg", []byte(toJSON(responseInfo.Environment))) //nolint:errcheck
	fb.AddFile("otel/otel-flare/cmdline.txt", []byte(responseInfo.Cmdline))                 //nolint:errcheck

	// retrieve each source of configuration
	for _, src := range responseInfo.Sources {
		response, err := http.Get(src.URL)
		if err != nil {
			fb.AddFile(fmt.Sprintf("otel/otel-flare/%s.err", src.Name), []byte(err.Error())) //nolint:errcheck
			continue
		}
		defer response.Body.Close()

		data, err := io.ReadAll(response.Body)
		if err != nil {
			fb.AddFile(fmt.Sprintf("otel/otel-flare/%s.err", src.Name), []byte(err.Error())) //nolint:errcheck
			continue
		}
		fb.AddFile(fmt.Sprintf("otel/otel-flare/%s.dat", src.Name), data) //nolint:errcheck

		if !src.Crawl {
			continue
		}

		// crawl the url by following any hyperlinks
		col := colly.NewCollector()
		col.OnHTML("a", func(e *colly.HTMLElement) {
			// visit all links
			link := e.Attr("href")
			e.Request.Visit(e.Request.AbsoluteURL(link)) //nolint:errcheck
		})
		col.OnResponse(func(r *colly.Response) {
			// the root sources (from the configResponseInfo) were already fetched earlier
			// don't re-fetch them
			responseURL := r.Request.URL.String()
			if responseURL == src.URL {
				return
			}
			// use the url as the basis for the filename saved in the flare
			filename := strings.ReplaceAll(url.PathEscape(responseURL), ":", "_")
			fb.AddFile(fmt.Sprintf("otel/otel-flare/crawl-%s", filename), r.Body) //nolint:errcheck
		})
		if err := col.Visit(src.URL); err != nil {
			fb.AddFile("otel/otel-flare/crawl.err", []byte(err.Error())) //nolint:errcheck
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

type configSourceInfo struct {
	Name  string `json:"name"`
	URL   string `json:"url"`
	Crawl bool   `json:"crawl"`
}

type configResponseInfo struct {
	StartupConf interface{}        `json:"startup_configuration"`
	RuntimeConf interface{}        `json:"runtime_configuration"`
	Environment interface{}        `json:"environment"`
	Cmdline     string             `json:"cmdline"`
	Sources     []configSourceInfo `json:"sources"`
}

// Can be overridden for tests
var overrideConfigResponse = ""

// TODO: Will be removed once otel extension exists and is in use
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
			"url": "http://localhost:5788/one",
			"crawl": true
		},
		{
			"name": "zpages",
			"url": "http://localhost:5788/two",
			"crawl": false
		},
		{
			"name": "healthcheck",
			"url": "http://localhost:5788/three",
			"crawl": true
		},
		{
			"name": "pprof",
			"url": "http://localhost:5788/four",
			"crawl": true
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
	// Value to return for tests
	if overrideConfigResponse != "" {
		return []byte(overrideConfigResponse), nil
	}
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
		FlareProvider:  flaretypes.NewProvider(collector.fillFlare),
		StatusProvider: status.NewInformationProvider(collector),
	}, nil
}
