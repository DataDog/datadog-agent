// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package cdn

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/DataDog/datadog-agent/comp/remote-config/rctelemetryreporter/rctelemetryreporterimpl"
	remoteconfig "github.com/DataDog/datadog-agent/pkg/config/remote/service"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/fleet/env"
	pbgo "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
	pkghostname "github.com/DataDog/datadog-agent/pkg/util/hostname"
	"github.com/DataDog/datadog-agent/pkg/version"
	"github.com/DataDog/go-tuf/data"
	"github.com/google/uuid"
	"go.uber.org/multierr"
	"gopkg.in/DataDog/dd-trace-go.v1/ddtrace/tracer"
)

type cdnDirect struct {
	rcService           *remoteconfig.CoreAgentService
	currentRootsVersion uint64
	clientUUID          string
	configDBPath        string
}

// newDirect creates a new direct CDN: it fetches the configuration from the remote config service instead of cloudfront
// note: naming is a bit misleading, it's not really a cdn, but we're following the convention
func newDirect(env *env.Env, configDBPath string) (CDN, error) {
	ctx := context.Background()
	ctx, cc := context.WithTimeout(ctx, 10*time.Second)
	defer cc()

	ht := newHostTagsGetter()
	hostname, err := pkghostname.Get(ctx)
	if err != nil {
		hostname = "unknown"
	}

	// Remove previous DB if needed
	err = os.RemoveAll(configDBPath)
	if err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("could not remove previous DB: %v", err)
	}

	options := []remoteconfig.Option{
		remoteconfig.WithAPIKey(env.APIKey),
		remoteconfig.WithConfigRootOverride(env.Site, ""),
		remoteconfig.WithDirectorRootOverride(env.Site, ""),
		remoteconfig.WithDatabaseFileName(filepath.Join(filepath.Base(configDBPath), "remote-config.db")),
	}

	service, err := remoteconfig.NewService(
		pkgconfigsetup.Datadog(), // May not be filled as we don't read the config when we're not in the daemon, in which case we'll use the defaults
		"Datadog Installer",
		fmt.Sprintf("https://config.%s", env.Site),
		hostname,
		ht.get,
		&rctelemetryreporterimpl.DdRcTelemetryReporter{}, // No telemetry for this client
		version.AgentVersion,
		options...,
	)
	if err != nil {
		return nil, err
	}
	cdn := &cdnDirect{
		rcService:           service,
		currentRootsVersion: 1,
		clientUUID:          uuid.New().String(),
		configDBPath:        configDBPath,
	}
	service.Start()
	return cdn, nil
}

func (c *cdnDirect) Get(ctx context.Context, pkg string) (cfg Config, err error) {
	span, _ := tracer.StartSpanFromContext(ctx, "cdn.Get")
	span.SetTag("cdn_type", "remote_config")
	defer func() { span.Finish(tracer.WithError(err)) }()

	switch pkg {
	case "datadog-agent":
		orderConfig, layers, err := c.get(ctx)
		if err != nil {
			return nil, err
		}
		cfg, err = newAgentConfig(orderConfig, layers...)
		if err != nil {
			return nil, err
		}
	default:
		return nil, ErrProductNotSupported
	}

	return cfg, nil
}

// get calls the Remote Config service to get the ordered layers.
func (c *cdnDirect) get(ctx context.Context) (*orderConfig, [][]byte, error) {
	agentConfigUpdate, err := c.rcService.ClientGetConfigs(ctx, &pbgo.ClientGetConfigsRequest{
		Client: &pbgo.Client{
			Id:        c.clientUUID,
			Products:  []string{"AGENT_CONFIG"},
			IsUpdater: true,
			ClientUpdater: &pbgo.ClientUpdater{
				Tags: []string{"installer:true"},
			},
			State: &pbgo.ClientState{
				RootVersion:    c.currentRootsVersion,
				TargetsVersion: 0,
			},
		},
	})
	if err != nil {
		return nil, nil, err
	}

	if agentConfigUpdate == nil {
		return &orderConfig{}, nil, nil
	}

	// Update root versions
	for _, root := range agentConfigUpdate.Roots {
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
	configLayers := make([][]byte, 0)
	var configOrder *orderConfig
	var layersErr error
	for _, file := range agentConfigUpdate.TargetFiles {
		matched := datadogConfigIDRegexp.FindStringSubmatch(file.GetPath())
		if len(matched) != 2 {
			layersErr = multierr.Append(layersErr, fmt.Errorf("invalid config path: %s", file.GetPath()))
			continue
		}
		configName := matched[1]

		if configName != configOrderID {
			configLayers = append(configLayers, file.GetRaw())
		} else {
			configOrder = &orderConfig{}
			err = json.Unmarshal(file.GetRaw(), configOrder)
			if err != nil {
				// Return first - we can't continue without the order
				return nil, nil, err
			}
		}
	}
	if layersErr != nil {
		return nil, nil, layersErr
	}
	return configOrder, configLayers, nil
}

func (c *cdnDirect) Close() error {
	return c.rcService.Stop()
}
