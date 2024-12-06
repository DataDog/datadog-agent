// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package cdn provides access to the Remote Config CDN.
package cdn

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/env"
	pbgo "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
	"gopkg.in/DataDog/dd-trace-go.v1/ddtrace/tracer"
)

const policyMetadataFilename = "policy.metadata"

var (
	// ErrProductNotSupported is returned when the product is not supported.
	ErrProductNotSupported = errors.New("product not supported")
)

// Config represents a configuration.
type Config interface {
	State() *pbgo.PoliciesState
	Write(dir string) error
}

// fetcher provides access to the Remote Config CDN.
type fetcher interface {
	get(ctx context.Context) ([][]byte, error)
	close() error
}

// CDN provides access to the Remote Config CDN.
type CDN struct {
	fetcher        fetcher
	hostTagsGetter hostTagsGetter
}

// New creates a new CDN and chooses the implementation depending
// on the environment
func New(env *env.Env, configDBPath string) (*CDN, error) {
	cdn := CDN{
		hostTagsGetter: newHostTagsGetter(env),
	}

	if runtime.GOOS == "windows" {
		// There's an assumption on windows that some directories are already there
		// but they are in fact created by the regular CDN implementation. Until
		// there is a fix on windows we keep the previous CDN behaviour for them
		fetcher, err := newHTTPFetcher(env, configDBPath)
		if err != nil {
			return nil, err
		}
		cdn.fetcher = fetcher
		return &cdn, nil
	}

	if !env.RemotePolicies {
		// Remote policies are not enabled -- we don't need the CDN
		// and we don't want to create the directories that the CDN
		// implementation would create. We return a no-op CDN to avoid
		// nil pointer dereference.
		fetcher, err := newNoopFetcher()
		if err != nil {
			return nil, err
		}
		cdn.fetcher = fetcher
		return &cdn, nil
	}

	if env.CDNLocalDirPath != "" {
		// Mock the CDN for local development or testing
		fetcher, err := newLocalFetcher(env)
		if err != nil {
			return nil, err
		}
		cdn.fetcher = fetcher
		return &cdn, nil
	}

	if !env.CDNEnabled {
		// Remote policies are enabled but we don't want to use the CDN
		// as it's still in development. We use standard remote config calls
		// instead (dubbed "direct" CDN).
		fetcher, err := newRCFetcher(env, configDBPath)
		if err != nil {
			return nil, err
		}
		cdn.fetcher = fetcher
		return &cdn, nil
	}

	// Regular CDN with the cloudfront distribution
	fetcher, err := newHTTPFetcher(env, configDBPath)
	if err != nil {
		return nil, err
	}
	cdn.fetcher = fetcher
	return &cdn, nil
}

// Get fetches the configuration for the given package.
func (c *CDN) Get(ctx context.Context, pkg string) (cfg Config, err error) {
	span, _ := tracer.StartSpanFromContext(ctx, "cdn.Get")
	defer func() {
		spanErr := err
		if spanErr == ErrProductNotSupported {
			spanErr = nil
		}
		span.Finish(tracer.WithError(spanErr))
	}()

	switch pkg {
	case "datadog-agent":
		orderedLayers, err := c.fetcher.get(ctx)
		if err != nil {
			return nil, err
		}
		cfg, err = newAgentConfig(orderedLayers...)
		if err != nil {
			return nil, err
		}
	case "datadog-apm-inject":
		orderedLayers, err := c.fetcher.get(ctx)
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

// Close closes the CDN.
func (c *CDN) Close() error {
	return c.fetcher.close()
}

// writePolicyMetadata writes the policy metadata to the given directory
// and makes it readable to dd-agent
func writePolicyMetadata(config Config, dir string) error {
	ddAgentUID, ddAgentGID, err := getAgentIDs()
	if err != nil {
		return fmt.Errorf("error getting dd-agent user and group IDs: %w", err)
	}

	state := config.State()
	stateBytes, err := json.Marshal(state)
	if err != nil {
		return fmt.Errorf("could not marshal state: %w", err)
	}
	err = os.WriteFile(filepath.Join(dir, policyMetadataFilename), stateBytes, 0440)
	if err != nil {
		return fmt.Errorf("could not write %s: %w", policyMetadataFilename, err)
	}
	if runtime.GOOS != "windows" {
		err = os.Chown(filepath.Join(dir, policyMetadataFilename), ddAgentUID, ddAgentGID)
		if err != nil {
			return fmt.Errorf("could not chown %s: %w", policyMetadataFilename, err)
		}
	}
	return nil
}
