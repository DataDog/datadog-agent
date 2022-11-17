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

	"github.com/DataDog/datadog-agent/pkg/autodiscovery/common/utils"
	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/autodiscovery/providers/names"
	"github.com/DataDog/datadog-agent/pkg/autodiscovery/telemetry"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/status/health"
	"github.com/DataDog/datadog-agent/pkg/util"
	"github.com/DataDog/datadog-agent/pkg/util/containers"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/workloadmeta"
)

// KubeletConfigProvider implements the ConfigProvider interface for the kubelet.
type KubeletConfigProvider struct {
	workloadmetaStore workloadmeta.Store
	podCache          map[string]*workloadmeta.KubernetesPod
	configErrors      map[string]ErrorMsgSet
	upToDate          bool
	streaming         bool
	once              sync.Once
	sync.RWMutex
}

// NewKubeletConfigProvider returns a new ConfigProvider connected to kubelet.
// Connectivity is not checked at this stage to allow for retries, Collect will do it.
func NewKubeletConfigProvider(*config.ConfigurationProviders) (ConfigProvider, error) {
	return &KubeletConfigProvider{
		workloadmetaStore: workloadmeta.GetGlobalStore(),
		configErrors:      make(map[string]ErrorMsgSet),
		podCache:          make(map[string]*workloadmeta.KubernetesPod),
	}, nil
}

// String returns a string representation of the KubeletConfigProvider
func (k *KubeletConfigProvider) String() string {
	return names.Kubernetes
}

// Collect retrieves all running pods and extract AD templates from their annotations.
func (k *KubeletConfigProvider) Collect(ctx context.Context) ([]integration.Config, error) {
	k.once.Do(func() {
		ch := make(chan struct{})
		go k.listen(ch)
		<-ch
	})

	k.Lock()
	k.upToDate = true
	k.Unlock()

	return k.generateConfigs()
}

// listen, closing the given channel after the initial set of events are received
func (k *KubeletConfigProvider) listen(ch chan struct{}) {
	const name = "ad-kubeletprovider"

	k.Lock()
	k.streaming = true
	health := health.RegisterLiveness(name)
	defer func() {
		err := health.Deregister()
		if err != nil {
			log.Warnf("error de-registering health check: %s", err)
		}
	}()
	k.Unlock()

	var ranOnce bool

	wlmetaCh := k.workloadmetaStore.Subscribe(name, workloadmeta.NormalPriority, workloadmeta.NewFilter(
		[]workloadmeta.Kind{workloadmeta.KindKubernetesPod},
		workloadmeta.SourceNodeOrchestrator,
		workloadmeta.EventTypeAll,
	))

	for {
		select {
		case evBundle, ok := <-wlmetaCh:
			if !ok {
				return
			}

			k.processEvents(evBundle)

			if !ranOnce {
				close(ch)
				ranOnce = true
			}

		case <-health.C:

		}
	}
}
func (k *KubeletConfigProvider) processEvents(evBundle workloadmeta.EventBundle) {
	close(evBundle.Ch)

	for _, event := range evBundle.Events {
		switch event.Type {
		case workloadmeta.EventTypeSet:
			k.addPod(event.Entity)
		case workloadmeta.EventTypeUnset:
			k.deletePod(event.Entity)

		default:
			log.Errorf("cannot handle event of type %d", event.Type)
		}
	}

}

func (k *KubeletConfigProvider) addPod(entity workloadmeta.Entity) {
	k.Lock()
	defer k.Unlock()
	pod := entity.(*workloadmeta.KubernetesPod)
	k.podCache[pod.GetID().ID] = pod
	k.upToDate = false
}

func (k *KubeletConfigProvider) deletePod(entity workloadmeta.Entity) {
	k.Lock()
	defer k.Unlock()
	delete(k.podCache, entity.GetID().ID)
	k.upToDate = false
}

func (k *KubeletConfigProvider) generateConfigs() ([]integration.Config, error) {
	k.Lock()
	defer k.Unlock()

	adErrors := make(map[string]ErrorMsgSet)

	var configs []integration.Config
	for _, pod := range k.podCache {
		var errs []error
		containerIdentifiers := map[string]struct{}{}
		containerNames := map[string]struct{}{}
		for _, podContainer := range pod.Containers {
			container, err := k.workloadmetaStore.GetContainer(podContainer.ID)
			if err != nil {
				log.Debugf("Pod %q has reference to non-existing container %q", pod.Name, podContainer.ID)
				continue
			}

			adIdentifier := podContainer.Name
			if customADID, found := utils.ExtractCheckIDFromPodAnnotations(pod.Annotations, podContainer.Name); found {
				adIdentifier = customADID
			}

			containerEntity := containers.BuildEntityName(string(container.Runtime), container.ID)
			c, errors := utils.ExtractTemplatesFromPodAnnotations(
				containerEntity,
				pod.Annotations,
				adIdentifier,
			)

			if len(errors) > 0 {
				for _, err := range errors {
					log.Errorf("Can't parse template for pod %s: %s", pod.Name, err)
					errs = append(errs, err)
				}
				if len(c) == 0 {
					// Only got errors, no valid configs so let's move on to the next container.
					continue
				}
			}

			if util.CcaInAD() {
				c = utils.AddContainerCollectAllConfigs(c, containerEntity)
			}

			containerIdentifiers[adIdentifier] = struct{}{}
			containerNames[podContainer.Name] = struct{}{}

			for idx := range c {
				c[idx].Source = "kubelet:" + containerEntity
			}

			configs = append(configs, c...)
		}

		errs = append(errs, utils.ValidateAnnotationsMatching(
			pod.Annotations,
			containerIdentifiers,
			containerNames)...)

		namespacedName := pod.Namespace + "/" + pod.Name
		for _, err := range errs {
			if _, found := adErrors[namespacedName]; !found {
				adErrors[namespacedName] = map[string]struct{}{err.Error(): {}}
			} else {
				adErrors[namespacedName][err.Error()] = struct{}{}
			}
		}
	}

	k.configErrors = adErrors
	telemetry.Errors.Set(float64(len(adErrors)), names.Kubernetes)

	return configs, nil
}

// IsUpToDate checks whether we have new pods to parse, based on events
// received by the listen goroutine. If listening fails, we fallback to
// collecting everytime.
func (k *KubeletConfigProvider) IsUpToDate(ctx context.Context) (bool, error) {
	k.RLock()
	defer k.RUnlock()
	return k.streaming && k.upToDate, nil
}

func init() {
	RegisterProvider(names.KubeletRegisterName, NewKubeletConfigProvider)
}

// GetConfigErrors returns a map of configuration errors for each namespace/pod
func (k *KubeletConfigProvider) GetConfigErrors() map[string]ErrorMsgSet {
	k.RLock()
	defer k.RUnlock()
	return k.configErrors
}
