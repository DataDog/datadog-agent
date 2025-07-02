// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !serverless

package providers

import (
	"context"
	"sync"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/providers/names"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/providers/types"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/telemetry"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// gpuCheckName is the name of the GPU check, to avoid importing the code from the GPU package
const gpuCheckName = "gpu"

// GPUConfigProvider implements the ConfigProvider interface for GPUs. This provider listens
// in Workloadmeta for GPU events. If any GPU is detected, it will generate a config to
// schedule the GPU check. As the GPU check covers all GPUs automatically, further GPUs
// will not trigger new configs.
type GPUConfigProvider struct {
	workloadmetaStore workloadmeta.Component

	// scheduledConfig is the config that is scheduled for the GPU check. Stored here for
	// unscheduling purposes.
	scheduledConfig *integration.Config

	// gpuDeviceCache is a cache of GPU devices that have been seen. If we stop seeing all GPU
	// devices, we will unschedule the GPU check.
	gpuDeviceCache map[string]struct{}
	mu             sync.RWMutex
}

var _ types.ConfigProvider = &GPUConfigProvider{}
var _ types.StreamingConfigProvider = &GPUConfigProvider{}

// NewGPUConfigProvider returns a new ConfigProvider subscribed to GPU events
func NewGPUConfigProvider(_ *pkgconfigsetup.ConfigurationProviders, wmeta workloadmeta.Component, _ *telemetry.Store) (types.ConfigProvider, error) {
	return &GPUConfigProvider{
		workloadmetaStore: wmeta,
		gpuDeviceCache:    make(map[string]struct{}),
	}, nil
}

// String returns a string representation of the GPUConfigProvider
func (k *GPUConfigProvider) String() string {
	return names.GPU
}

// Stream starts listening to workloadmeta to generate configs as they come
// instead of relying on a periodic call to Collect.
func (k *GPUConfigProvider) Stream(ctx context.Context) <-chan integration.ConfigChanges {
	const name = "ad-gpuprovider"

	// outCh must be unbuffered. processing of workloadmeta events must not
	// proceed until the config is processed by autodiscovery, as configs
	// need to be generated before any associated services.
	outCh := make(chan integration.ConfigChanges)

	filter := workloadmeta.NewFilterBuilder().
		AddKind(workloadmeta.KindGPU).
		Build()
	inCh := k.workloadmetaStore.Subscribe(name, workloadmeta.ConfigProviderPriority, filter)

	go func() {
		for {
			select {
			case <-ctx.Done():
				k.workloadmetaStore.Unsubscribe(inCh)

			case evBundle, ok := <-inCh:
				if !ok {
					return
				}

				// send changes even when they're empty, as we
				// need to signal that an event has been
				// received, for flow control reasons
				outCh <- k.processEvents(evBundle)
				evBundle.Acknowledge()
			}
		}
	}()

	return outCh
}

func (k *GPUConfigProvider) processEvents(evBundle workloadmeta.EventBundle) integration.ConfigChanges {
	k.mu.Lock()
	defer k.mu.Unlock()

	changes := integration.ConfigChanges{}

	for _, event := range evBundle.Events {
		gpuUUID := event.Entity.GetID().ID

		switch event.Type {
		case workloadmeta.EventTypeSet:
			// Track seen GPU devices
			k.gpuDeviceCache[gpuUUID] = struct{}{}

			// We only need to schedule the check once
			if k.scheduledConfig != nil {
				continue
			}

			k.scheduledConfig = &integration.Config{
				Name:       gpuCheckName,
				Instances:  []integration.Data{[]byte{}},
				InitConfig: []byte{},
				Provider:   names.GPU,
				Source:     names.GPU,
			}

			changes.ScheduleConfig(*k.scheduledConfig)
		case workloadmeta.EventTypeUnset:
			delete(k.gpuDeviceCache, gpuUUID)

			// Unschedule the check if no more GPUs are detected
			if len(k.gpuDeviceCache) == 0 && k.scheduledConfig != nil {
				changes.UnscheduleConfig(*k.scheduledConfig)
			}
		default:
			log.Errorf("cannot handle event of type %d", event.Type)
		}
	}

	return changes
}

// GetConfigErrors returns a map of configuration errors, which is always empty for the GPUConfigProvider
func (k *GPUConfigProvider) GetConfigErrors() map[string]types.ErrorMsgSet {
	return make(map[string]types.ErrorMsgSet)
}
