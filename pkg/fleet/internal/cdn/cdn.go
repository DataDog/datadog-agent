// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package cdn provides access to the Remote Config CDN.
package cdn

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"time"

	"github.com/DataDog/datadog-agent/comp/metadata/host/hostimpl/hosttags"
	"github.com/DataDog/datadog-agent/comp/remote-config/rctelemetryreporter/rctelemetryreporterimpl"
	detectenv "github.com/DataDog/datadog-agent/pkg/config/env"
	"github.com/DataDog/datadog-agent/pkg/config/model"
	remoteconfig "github.com/DataDog/datadog-agent/pkg/config/remote/service"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/fleet/env"
	pbgo "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
	pkghostname "github.com/DataDog/datadog-agent/pkg/util/hostname"
	"github.com/DataDog/datadog-agent/pkg/version"
	"github.com/google/uuid"
	"go.uber.org/multierr"
	"gopkg.in/DataDog/dd-trace-go.v1/ddtrace/tracer"
)

var datadogConfigIDRegexp = regexp.MustCompile(`^datadog/\d+/AGENT_CONFIG/([^/]+)/[^/]+$`)

const configOrderID = "configuration_order"

// CDN provides access to the Remote Config CDN.
type CDN struct {
	env *env.Env
}

// Config represents the configuration from the CDN.
type Config struct {
	Version       string
	Datadog       []byte
	SecurityAgent []byte
	SystemProbe   []byte
}

type orderConfig struct {
	Order []string `json:"order"`
}

// New creates a new CDN.
func New(env *env.Env) *CDN {
	return &CDN{
		env: env,
	}
}

// Get gets the configuration from the CDN.
func (c *CDN) Get(ctx context.Context) (_ *Config, err error) {
	span, ctx := tracer.StartSpanFromContext(ctx, "cdn.Get")
	defer func() { span.Finish(tracer.WithError(err)) }()
	configLayers, err := c.getOrderedLayers(ctx)
	if err != nil {
		return nil, err
	}
	return newConfig(configLayers...)
}

// getOrderedLayers calls the Remote Config service to get the ordered layers.
// Today it doesn't use the CDN, but it should in the future
func (c *CDN) getOrderedLayers(ctx context.Context) ([]*layer, error) {
	// HACK(baptiste): Create a dedicated one-shot RC service just for the configuration
	// We should use the CDN instead
	config := pkgconfigsetup.Datadog()
	config.Set("run_path", "/opt/datadog-packages/datadog-installer/stable/run", model.SourceAgentRuntime)

	detectenv.DetectFeatures(config)
	hostname, err := pkghostname.Get(ctx)
	if err != nil {
		return nil, err
	}
	options := []remoteconfig.Option{
		remoteconfig.WithAPIKey(c.env.APIKey),
		remoteconfig.WithConfigRootOverride(c.env.Site, ""),
		remoteconfig.WithDirectorRootOverride(c.env.Site, ""),
	}
	service, err := remoteconfig.NewService(
		config,
		"Datadog Installer",
		fmt.Sprintf("https://config.%s", c.env.Site),
		hostname,
		getHostTags(ctx, config),
		&rctelemetryreporterimpl.DdRcTelemetryReporter{}, // No telemetry for this client
		version.AgentVersion,
		options...,
	)
	if err != nil {
		return nil, err
	}
	service.Start()
	defer func() { _ = service.Stop() }()
	// Force a cache bypass
	cfgs, err := service.ClientGetConfigs(ctx, &pbgo.ClientGetConfigsRequest{
		Client: &pbgo.Client{
			Id:            uuid.New().String(),
			Products:      []string{"AGENT_CONFIG"},
			IsUpdater:     true,
			ClientUpdater: &pbgo.ClientUpdater{},
			State: &pbgo.ClientState{
				RootVersion:    1,
				TargetsVersion: 1,
			},
		},
	})
	if err != nil {
		return nil, err
	}

	// Unmarshal RC results
	configLayers := map[string]*layer{}
	var configOrder *orderConfig
	var layersErr error
	for _, file := range cfgs.TargetFiles {
		matched := datadogConfigIDRegexp.FindStringSubmatch(file.GetPath())
		if len(matched) != 2 {
			layersErr = multierr.Append(layersErr, fmt.Errorf("invalid config path: %s", file.GetPath()))
			continue
		}
		configName := matched[1]

		if configName != configOrderID {
			configLayer := &layer{}
			err = json.Unmarshal(file.GetRaw(), configLayer)
			if err != nil {
				// If a layer is wrong, fail later to parse the rest and check them all
				layersErr = multierr.Append(layersErr, err)
				continue
			}
			configLayers[configName] = configLayer
		} else {
			configOrder = &orderConfig{}
			err = json.Unmarshal(file.GetRaw(), configOrder)
			if err != nil {
				// Return first - we can't continue without the order
				return nil, err
			}
		}
	}
	if layersErr != nil {
		return nil, layersErr
	}

	// Order configs
	if configOrder == nil {
		return nil, fmt.Errorf("no configuration_order found")
	}
	orderedLayers := []*layer{}
	for _, configName := range configOrder.Order {
		if configLayer, ok := configLayers[configName]; ok {
			orderedLayers = append(orderedLayers, configLayer)
		}
	}

	return orderedLayers, nil
}

func getHostTags(ctx context.Context, config model.Config) func() []string {
	return func() []string {
		// Host tags are cached on host, but we add a timeout to avoid blocking the RC request
		// if the host tags are not available yet and need to be fetched. They will be fetched
		// by the first agent metadata V5 payload.
		ctx, cc := context.WithTimeout(ctx, time.Second)
		defer cc()
		hostTags := hosttags.Get(ctx, true, config)
		tags := append(hostTags.System, hostTags.GoogleCloudPlatform...)
		tags = append(tags, "installer:true")
		return tags
	}
}
