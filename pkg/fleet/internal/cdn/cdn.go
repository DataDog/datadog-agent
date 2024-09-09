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
	detectenv "github.com/DataDog/datadog-agent/pkg/config/env"
	"github.com/DataDog/datadog-agent/pkg/config/model"
	remoteconfig "github.com/DataDog/datadog-agent/pkg/config/remote/service"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/fleet/env"
	pbgo "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
	"github.com/DataDog/go-tuf/data"
	"go.uber.org/multierr"
	"gopkg.in/DataDog/dd-trace-go.v1/ddtrace/tracer"
	"regexp"
)

var datadogConfigIDRegexp = regexp.MustCompile(`^datadog/\d+/AGENT_CONFIG/([^/]+)/[^/]+$`)

const configOrderID = "configuration_order"

// CDN provides access to the Remote Config CDN.
type CDN struct {
	client                *remoteconfig.HTTPClient
	currentRootsVersion   uint64
	currentTargetsVersion uint64
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
func New(env *env.Env) (*CDN, error) {
	config := pkgconfigsetup.Datadog()
	config.Set("run_path", "/opt/datadog-packages/datadog-installer/stable/run", model.SourceAgentRuntime)

	detectenv.DetectFeatures(config)
	client, err := remoteconfig.NewHTTPClient(
		config,
		fmt.Sprintf("https://remote-config.%s", env.Site),
		fmt.Sprintf("https://config.%s", env.Site),
		env.Site,
		env.APIKey,
	)
	if err != nil {
		return nil, err
	}

	return &CDN{
		client:                client,
		currentTargetsVersion: 1,
		currentRootsVersion:   1,
	}, nil
}

// Get gets the configuration from the CDN.
func (c *CDN) Get(ctx context.Context) (_ *Config, err error) {
	span, ctx := tracer.StartSpanFromContext(ctx, "cdn.Get")
	defer func() { span.Finish(tracer.WithError(err)) }()
	configLayers, err := c.getOrderedLayers()
	if err != nil {
		return nil, err
	}
	return newConfig(configLayers...)
}

// getOrderedLayers calls the Remote Config service to get the ordered layers.
func (c *CDN) getOrderedLayers() ([]*layer, error) {
	agentConfigUpdate, err := c.client.GetCDNConfigUpdate(
		[]string{"AGENT_CONFIG"},
		// Always send 0 since we are relying on the CDN cache state instead of our own tracer cache. This will fetch the latest configs from the cache/CDN everytime.
		0,
		// Not using the roots; send the highest seen version of roots so don't received them all on every request
		c.currentRootsVersion,
		// Not using a client cache; fetch all the applicable target files every time.
		[]*pbgo.TargetFileMeta{},
	)
	if err != nil {
		return nil, err
	}

	// We always expect configurations.
	if agentConfigUpdate == nil {
		return nil, fmt.Errorf("no configurations found for AGENT_CONFIG")
	}

	// Update CDN root versions
	for _, root := range agentConfigUpdate.TUFRoots {
		var signedRoot data.Signed
		err = json.Unmarshal(root, &signedRoot)
		if err != nil {
			continue
		}
		var r data.Root
		err = json.Unmarshal(signedRoot.Signed, &r)
		if err != nil {
			continue
		}
		if uint64(r.Version) > c.currentRootsVersion {
			c.currentRootsVersion = uint64(r.Version)
		}
	}

	var signedTargets data.Signed
	err = json.Unmarshal(agentConfigUpdate.TUFTargets, &signedTargets)
	if err == nil {
		var targets data.Targets
		err = json.Unmarshal(signedTargets.Signed, &targets)
		if err == nil && uint64(targets.Version) > c.currentTargetsVersion {
			c.currentTargetsVersion = uint64(targets.Version)
		}
	}

	// Unmarshal RC results
	configLayers := map[string]*layer{}
	var configOrder *orderConfig
	var layersErr error
	paths := agentConfigUpdate.ClientConfigs
	targetFiles := agentConfigUpdate.TargetFiles
	for _, path := range paths {
		matched := datadogConfigIDRegexp.FindStringSubmatch(path)
		if len(matched) != 2 {
			layersErr = multierr.Append(layersErr, fmt.Errorf("invalid config path: %s", path))
			continue
		}
		configName := matched[1]

		file := targetFiles[path]
		if configName != configOrderID {
			configLayer := &layer{}
			err = json.Unmarshal(file, configLayer)
			if err != nil {
				// If a layer is wrong, fail later to parse the rest and check them all
				layersErr = multierr.Append(layersErr, err)
				continue
			}
			configLayers[configName] = configLayer
		} else {
			configOrder = &orderConfig{}
			err = json.Unmarshal(file, configOrder)
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
