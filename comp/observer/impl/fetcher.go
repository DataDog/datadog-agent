// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package observerimpl implements the observer component.
package observerimpl

import (
	"context"
	"sync"
	"time"

	remoteagentregistry "github.com/DataDog/datadog-agent/comp/core/remoteagentregistry/def"
	observerdef "github.com/DataDog/datadog-agent/comp/observer/def"
	pbcore "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
	pkglog "github.com/DataDog/datadog-agent/pkg/util/log"
)

// FetcherConfig contains configuration for the observer fetcher.
type FetcherConfig struct {
	// ProfileFetchInterval is how often to fetch profiles from remote agents.
	ProfileFetchInterval time.Duration
	// MaxProfileBatch is the maximum number of profiles to fetch per request.
	MaxProfileBatch uint32
}

// DefaultFetcherConfig returns the default fetcher configuration.
func DefaultFetcherConfig() FetcherConfig {
	return FetcherConfig{
		ProfileFetchInterval: 10 * time.Second,
		MaxProfileBatch:      50,
	}
}

// observerFetcher periodically fetches profiles from remote trace-agents
// using the remoteAgentRegistry's GetObserverProfiles method.
type observerFetcher struct {
	registry remoteagentregistry.Component
	handle   observerdef.Handle
	config   FetcherConfig

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// newObserverFetcher creates a new observer fetcher.
func newObserverFetcher(
	registry remoteagentregistry.Component,
	handle observerdef.Handle,
) *observerFetcher {
	return &observerFetcher{
		registry: registry,
		handle:   handle,
		config:   DefaultFetcherConfig(),
	}
}

// Start begins the periodic fetching goroutines.
func (f *observerFetcher) Start() {
	if f.registry == nil {
		pkglog.Debug("[observer] fetcher not started: no registry")
		return
	}

	f.ctx, f.cancel = context.WithCancel(context.Background())

	// Start profile fetcher
	f.wg.Add(1)
	go f.runProfileFetcher()

	pkglog.Info("[observer] fetcher started")
}

// Stop stops the fetcher.
func (f *observerFetcher) Stop() {
	if f.cancel != nil {
		f.cancel()
	}
	f.wg.Wait()
	pkglog.Info("[observer] fetcher stopped")
}

// runProfileFetcher periodically fetches profiles from all registered trace-agents.
func (f *observerFetcher) runProfileFetcher() {
	defer f.wg.Done()

	ticker := time.NewTicker(f.config.ProfileFetchInterval)
	defer ticker.Stop()

	for {
		select {
		case <-f.ctx.Done():
			return
		case <-ticker.C:
			f.fetchProfiles()
		}
	}
}

// fetchProfiles fetches profiles from all registered trace-agents using the registry.
func (f *observerFetcher) fetchProfiles() {
	results := f.registry.GetObserverProfiles(f.config.MaxProfileBatch)

	hasMore := false
	for _, result := range results {
		if result.FailureReason != "" {
			pkglog.Warnf("[observer] failed to fetch profiles from %s: %s", result.DisplayName, result.FailureReason)
			continue
		}

		if result.DroppedCount > 0 {
			pkglog.Warnf("[observer] %d profiles were dropped in %s buffer", result.DroppedCount, result.DisplayName)
		}

		for _, profileData := range result.Profiles {
			f.handle.ObserveProfile(&profileDataView{data: profileData})
		}

		if result.HasMore {
			hasMore = true
		}
	}

	// If there's more data, immediately fetch again
	if hasMore {
		go f.fetchProfiles()
	}
}

// profileDataView adapts ProfileData proto to the ProfileView interface.
type profileDataView struct {
	data *pbcore.ProfileData
}

func (v *profileDataView) GetProfileID() string        { return v.data.ProfileId }
func (v *profileDataView) GetProfileType() string      { return v.data.ProfileType }
func (v *profileDataView) GetService() string          { return v.data.Service }
func (v *profileDataView) GetEnv() string              { return v.data.Env }
func (v *profileDataView) GetVersion() string          { return v.data.Version }
func (v *profileDataView) GetHostname() string         { return v.data.Hostname }
func (v *profileDataView) GetContainerID() string      { return v.data.ContainerId }
func (v *profileDataView) GetTimestampUnixNano() int64 { return v.data.TimestampNs }
func (v *profileDataView) GetDurationNano() int64      { return v.data.DurationNs }
func (v *profileDataView) GetTags() map[string]string  { return v.data.Tags }
func (v *profileDataView) GetContentType() string      { return v.data.ContentType }
func (v *profileDataView) GetRawData() []byte          { return v.data.InlineData }
func (v *profileDataView) GetExternalPath() string     { return "" }
