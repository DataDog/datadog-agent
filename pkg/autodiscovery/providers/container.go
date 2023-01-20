// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !serverless
// +build !serverless

package providers

import (
	"context"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/autodiscovery/common/utils"
	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/autodiscovery/providers/names"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/status/health"
	"github.com/DataDog/datadog-agent/pkg/util"
	"github.com/DataDog/datadog-agent/pkg/util/containers"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/workloadmeta"
)

const (
	delayDuration = 5 * time.Second
)

// ContainerConfigProvider implements the ConfigProvider interface for container labels.
type ContainerConfigProvider struct {
	sync.RWMutex
	workloadmetaStore workloadmeta.Store
	upToDate          bool
	streaming         bool
	containerCache    map[string]*workloadmeta.Container
	containerFilter   *containers.Filter
	once              sync.Once
}

// NewContainerConfigProvider creates a new ContainerConfigProvider
func NewContainerConfigProvider(*config.ConfigurationProviders) (ConfigProvider, error) {
	containerFilter, err := containers.NewAutodiscoveryFilter(containers.GlobalFilter)
	if err != nil {
		log.Warnf("Can't get container include/exclude filter, no filtering will be applied: %v", err)
	}

	return &ContainerConfigProvider{
		workloadmetaStore: workloadmeta.GetGlobalStore(),
		containerCache:    make(map[string]*workloadmeta.Container),
		containerFilter:   containerFilter,
	}, nil
}

// String returns a string representation of the ContainerConfigProvider
func (d *ContainerConfigProvider) String() string {
	return names.Container
}

// Collect retrieves all running containers and extract AD templates from their labels.
func (d *ContainerConfigProvider) Collect(ctx context.Context) ([]integration.Config, error) {
	d.once.Do(func() {
		ch := make(chan struct{})
		go d.listen(ch)
		<-ch
	})

	d.Lock()
	d.upToDate = true
	d.Unlock()

	d.RLock()
	defer d.RUnlock()
	return d.generateConfigs()
}

// listen, closing the given channel after the initial set of events are received
func (d *ContainerConfigProvider) listen(ch chan struct{}) {
	d.Lock()
	d.streaming = true
	health := health.RegisterLiveness("ad-containerprovider")
	defer func() {
		err := health.Deregister()
		if err != nil {
			log.Warnf("error de-registering health check: %s", err)
		}
	}()
	d.Unlock()

	var ranOnce bool

	workloadmetaEventsChannel := d.workloadmetaStore.Subscribe("ad-containerprovider", workloadmeta.NormalPriority, workloadmeta.NewFilter(
		[]workloadmeta.Kind{workloadmeta.KindContainer},
		workloadmeta.SourceRuntime,
		workloadmeta.EventTypeAll,
	))

	for {
		select {
		case evBundle, ok := <-workloadmetaEventsChannel:
			if !ok {
				return
			}

			d.processEvents(evBundle)

			if !ranOnce {
				close(ch)
				ranOnce = true
			}

		case <-health.C:

		}
	}
}

func (d *ContainerConfigProvider) processEvents(eventBundle workloadmeta.EventBundle) {
	close(eventBundle.Ch)

	for _, event := range eventBundle.Events {
		containerID := event.Entity.GetID().ID

		switch event.Type {
		case workloadmeta.EventTypeSet:
			container := event.Entity.(*workloadmeta.Container)

			d.RLock()
			_, containerSeen := d.containerCache[container.ID]
			d.RUnlock()
			if containerSeen {
				// Container restarted with the same ID within 5 seconds.
				// This delay is needed because of the delay introduced in the
				// EventTypeUnset case.
				time.AfterFunc(delayDuration, func() {
					d.addContainer(containerID, container)
				})
			} else {
				d.addContainer(containerID, container)
			}

		case workloadmeta.EventTypeUnset:
			// delay for short-lived detection
			time.AfterFunc(delayDuration, func() {
				d.deleteContainer(containerID)
			})

		default:
			log.Errorf("cannot handle event of type %d", event.Type)
		}
	}

}

// IsUpToDate checks whether we have new containers to parse, based on events
// received by the listen goroutine. If listening fails, we fallback to
// collecting everytime.
func (d *ContainerConfigProvider) IsUpToDate(ctx context.Context) (bool, error) {
	d.RLock()
	defer d.RUnlock()
	return d.streaming && d.upToDate, nil
}

// addLabels updates the cache for a given container
func (d *ContainerConfigProvider) addContainer(containerID string, container *workloadmeta.Container) {
	d.Lock()
	defer d.Unlock()
	d.containerCache[containerID] = container
	d.upToDate = false
}

func (d *ContainerConfigProvider) deleteContainer(containerID string) {
	d.Lock()
	defer d.Unlock()
	delete(d.containerCache, containerID)
	d.upToDate = false
}

func (d *ContainerConfigProvider) generateConfigs() ([]integration.Config, error) {
	var configs []integration.Config
	for containerID, container := range d.containerCache {
		containerEntityName := containers.BuildEntityName(string(container.Runtime), containerID)
		c, errors := utils.ExtractTemplatesFromContainerLabels(containerEntityName, container.Labels)

		for _, err := range errors {
			log.Errorf("Can't parse template for container %s: %s", containerID, err)
		}

		if util.CcaInAD() {
			c = utils.AddContainerCollectAllConfigs(c, containerEntityName)
		}

		for idx := range c {
			c[idx].Source = names.Container + ":" + containerEntityName
		}

		configs = append(configs, c...)
	}
	return configs, nil
}

func init() {
	RegisterProvider(names.Container, NewContainerConfigProvider)
}

// GetConfigErrors is not implemented for the ContainerConfigProvider
func (d *ContainerConfigProvider) GetConfigErrors() map[string]ErrorMsgSet {
	return make(map[string]ErrorMsgSet)
}
