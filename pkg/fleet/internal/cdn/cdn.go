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
	remoteconfig "github.com/DataDog/datadog-agent/pkg/config/remote/service"
	"github.com/DataDog/datadog-agent/pkg/fleet/env"
	pbgo "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
	"github.com/DataDog/datadog-agent/pkg/version"
	"github.com/DataDog/go-tuf/data"
	"go.uber.org/multierr"
	"gopkg.in/DataDog/dd-trace-go.v1/ddtrace/tracer"
	"regexp"
)

var datadogConfigIDRegexp = regexp.MustCompile(`^datadog/\d+/AGENT_CONFIG/([^/]+)/[^/]+$`)

const configOrderID = "configuration_order"

// CDN provides access to the Remote Config CDN.
type CDN struct {
	client              *remoteconfig.HTTPClient
	currentRootsVersion uint64
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

	client, err := remoteconfig.NewHTTPClient(
		"/opt/datadog-packages/run",
		env.Site,
		env.APIKey,
		version.AgentVersion,
	)
	if err != nil {
		return nil, err
	}

	return &CDN{
		client:              client,
		currentRootsVersion: 1,
	}, nil
}

// Get gets the configuration from the CDN.
func (c *CDN) Get(ctx context.Context) (_ *Config, err error) {
	span, _ := tracer.StartSpanFromContext(ctx, "cdn.Get")
	defer func() { span.Finish(tracer.WithError(err)) }()
	configLayers, err := c.getOrderedLayers(ctx)
	if err != nil {
		return nil, err
	}
	return newConfig(configLayers...)
}

// Close cleans up the CDN's resources
func (c *CDN) Close() error {
	return c.client.Close()
}

// getOrderedLayers calls the Remote Config service to get the ordered layers.
func (c *CDN) getOrderedLayers(ctx context.Context) ([]*layer, error) {
	agentConfigUpdate, err := c.client.GetCDNConfigUpdate(
		ctx,
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

	orderedLayers := []*layer{}
	if agentConfigUpdate == nil {
		return orderedLayers, nil
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

		file, ok := targetFiles[path]
		if !ok {
			layersErr = multierr.Append(layersErr, fmt.Errorf("missing expected target file in update response: %s", path))
			continue
		}
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
	for _, configName := range configOrder.Order {
		if configLayer, ok := configLayers[configName]; ok {
			orderedLayers = append(orderedLayers, configLayer)
		}
	}

	return orderedLayers, nil
}
