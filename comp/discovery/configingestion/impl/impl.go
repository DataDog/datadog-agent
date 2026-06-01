// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package configingestionimpl implements the config ingestion Fx component.
package configingestionimpl

import (
	"context"
	"time"

	"github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	configingestiondef "github.com/DataDog/datadog-agent/comp/discovery/configingestion/def"
	"github.com/DataDog/datadog-agent/pkg/discovery/configingestion"
	"github.com/DataDog/datadog-agent/pkg/util/hostname"
	"github.com/DataDog/datadog-agent/pkg/util/option"
)

// hostnameResolutionTimeout caps the hostname lookup during Fx initialization
// so a slow DNS query does not block agent startup indefinitely.
const hostnameResolutionTimeout = 3 * time.Second

// Requires defines the Fx dependencies.
type Requires struct {
	Lifecycle    compdef.Lifecycle
	Config       config.Component
	Log          log.Component
	Workloadmeta workloadmeta.Component
}

// Provides defines the Fx outputs.
type Provides struct {
	Comp option.Option[configingestiondef.Component]
}

type impl struct {
	watcher *configingestion.Watcher
	log     log.Component
}

// NewComponent creates the config ingestion component.
func NewComponent(reqs Requires) (Provides, error) {
	if !reqs.Config.GetBool("discovery.config_ingestion.enabled") {
		return Provides{Comp: option.None[configingestiondef.Component]()}, nil
	}

	intakeURL := reqs.Config.GetString("discovery.config_ingestion.intake_url")
	// Re-use the agent's main API key. The key is read from the main agent
	// config (datadog.yaml), consistent with discovery.config_ingestion.* keys
	// which are registered there as well (see pkg/config/setup/system_probe.go
	// until they are moved to the main config setup in a follow-up).
	apiKey := reqs.Config.GetString("api_key")

	hostCtx, cancel := context.WithTimeout(context.Background(), hostnameResolutionTimeout)
	defer cancel()
	hostID, err := hostname.Get(hostCtx)
	if err != nil {
		hostID = "unknown"
		reqs.Log.Warnf("configingestion: could not determine hostname: %v", err)
	}

	w := configingestion.NewWatcher(reqs.Workloadmeta, configingestion.Config{
		IntakeURL: intakeURL,
		APIKey:    apiKey,
		HostID:    hostID,
	})

	c := &impl{watcher: w, log: reqs.Log}

	reqs.Lifecycle.Append(compdef.Hook{
		OnStart: c.start,
		OnStop:  c.stop,
	})

	return Provides{Comp: option.New[configingestiondef.Component](c)}, nil
}

func (c *impl) start(_ context.Context) error {
	c.log.Info("Starting config file ingestion watcher")
	// Use context.Background() so the watcher goroutine is not tied to the
	// short-lived Fx OnStart context, which is cancelled once startup completes.
	// The goroutine is stopped explicitly by stop() via Watcher.Stop().
	// TODO(DSCVR Phase D): if Watcher.Start gains an error return, surface it here.
	c.watcher.Start(context.Background())
	return nil
}

func (c *impl) stop(_ context.Context) error {
	// Stop cancels the goroutine context and waits for the goroutine to exit,
	// guaranteeing workloadmeta resources are not accessed after this returns.
	c.watcher.Stop()
	return nil
}
