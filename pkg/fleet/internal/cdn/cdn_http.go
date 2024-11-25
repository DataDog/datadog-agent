// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package cdn provides access to the Remote Config CDN.
package cdn

import (
	"context"
	"encoding/json"

	remoteconfig "github.com/DataDog/datadog-agent/pkg/config/remote/service"
	"github.com/DataDog/datadog-agent/pkg/fleet/env"
	pbgo "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/version"
	"github.com/DataDog/go-tuf/data"
	"gopkg.in/DataDog/dd-trace-go.v1/ddtrace/tracer"
)

type cdnHTTP struct {
	client              *remoteconfig.HTTPClient
	currentRootsVersion uint64
	hostTagsGetter      hostTagsGetter
	env                 *env.Env
}

func newCDNHTTP(env *env.Env, configDBPath string) (CDN, error) {
	client, err := remoteconfig.NewHTTPClient(
		configDBPath,
		env.Site,
		env.APIKey,
		version.AgentVersion,
	)
	if err != nil {
		return nil, err
	}
	return &cdnHTTP{
		client:              client,
		currentRootsVersion: 1,
		hostTagsGetter:      newHostTagsGetter(env),
		env:                 env,
	}, nil
}

// Get gets the configuration from the CDN.
func (c *cdnHTTP) Get(ctx context.Context, pkg string) (cfg Config, err error) {
	span, _ := tracer.StartSpanFromContext(ctx, "cdn.Get")
	span.SetTag("cdn_type", "cdn")
	defer func() {
		spanErr := err
		if spanErr == ErrProductNotSupported {
			spanErr = nil
		}
		span.Finish(tracer.WithError(spanErr))
	}()

	switch pkg {
	case "datadog-agent":
		orderedLayers, err := c.get(ctx)
		if err != nil {
			return nil, err
		}
		cfg, err = newAgentConfig(orderedLayers...)
		if err != nil {
			return nil, err
		}
	case "datadog-apm-inject":
		orderedLayers, err := c.get(ctx)
		if err != nil {
			return nil, err
		}
		cfg, err = newAPMConfig(c.hostTagsGetter.get(), orderedLayers...)
		if err != nil {
			return nil, err
		}
	default:
		return nil, ErrProductNotSupported
	}

	return cfg, nil
}

// Close cleans up the CDN's resources
func (c *cdnHTTP) Close() error {
	return c.client.Close()
}

// get calls the Remote Config service to get the ordered layers.
func (c *cdnHTTP) get(ctx context.Context) ([][]byte, error) {
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

	if agentConfigUpdate == nil {
		return nil, nil
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

	files := map[string][]byte{}
	for path, content := range agentConfigUpdate.TargetFiles {
		pathMatches := datadogConfigIDRegexp.FindStringSubmatch(path)
		if len(pathMatches) != 2 {
			log.Warnf("invalid config path: %s", path)
			continue
		}
		files[pathMatches[1]] = content
	}

	return getOrderedScopedLayers(
		files,
		getScopeExprVars(c.env, c.hostTagsGetter),
	)
}
