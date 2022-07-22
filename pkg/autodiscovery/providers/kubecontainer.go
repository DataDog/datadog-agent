// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !serverless
// +build !serverless

package providers

import (
	"context"
	"fmt"
	"strings"
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

// KubeContainerConfigProvider implements the ConfigProvider interface for both kubelet and containers
type KubeContainerConfigProvider struct {
	workloadmetaStore workloadmeta.Store
	podCache          map[string]*workloadmeta.KubernetesPod
	containerCache    map[string]*workloadmeta.Container
	configErrors      map[string]ErrorMsgSet
	upToDate          bool
	streaming         bool
	once              sync.Once
	sync.RWMutex
}

// NewKubeContainerConfigProvider returns a new ConfigProvider subscribed to both container
// and pods
func NewKubeContainerConfigProvider(*config.ConfigurationProviders) (ConfigProvider, error) {
	return &KubeContainerConfigProvider{
		workloadmetaStore: workloadmeta.GetGlobalStore(),
		configErrors:      make(map[string]ErrorMsgSet),
		podCache:          make(map[string]*workloadmeta.KubernetesPod),
		containerCache:    make(map[string]*workloadmeta.Container),
	}, nil
}

// String returns a string representation of the KubeContainerConfigProvider
func (k *KubeContainerConfigProvider) String() string {
	return names.KubeContainer
}

// Collect retrieves all running pods and extract AD templates from their annotations.
func (k *KubeContainerConfigProvider) Collect(ctx context.Context) ([]integration.Config, error) {
	k.once.Do(func() {
		go k.listen()
	})

	k.Lock()
	k.upToDate = true
	k.Unlock()

	return k.generateConfigs()
}

func (k *KubeContainerConfigProvider) listen() {
	const name = "ad-kubecontainerprovider"

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

	ch := k.workloadmetaStore.Subscribe(name, workloadmeta.NormalPriority, workloadmeta.NewFilter(
		[]workloadmeta.Kind{workloadmeta.KindKubernetesPod, workloadmeta.KindContainer},
		// TODO Don't actually need SourceAll, just need both SourceNodeOrchestrator and SourceRuntime
		// Should this be two separate subscriptions?
		// When requesting 'SourceAll', you get the 'cachedEntity' (aka merged) vs
		// when requesting a single Source you get only that version of the source.
		// The latter is more similar to how the Kubelet and Container provider work currently
		//
		// I believe separate subscriptions _or_ passing the 'source' in the EventBundle
		// is necessary to avoid a race between the kubelet's view of a container and the runtime's view of a container
		workloadmeta.SourceAll,
		workloadmeta.EventTypeAll,
	))

	for {
		select {
		case evBundle, ok := <-ch:
			if !ok {
				return
			}

			k.processEvents(evBundle)

		case <-health.C:

		}
	}
}

func (k *KubeContainerConfigProvider) processEvents(evBundle workloadmeta.EventBundle) {
	close(evBundle.Ch)

	for _, event := range evBundle.Events {
		switch event.Type {
		case workloadmeta.EventTypeSet:
			k.addEntity(event.Entity)
		case workloadmeta.EventTypeUnset:
			k.deleteEntity(event.Entity)

		default:
			log.Errorf("cannot handle event of type %d", event.Type)
		}
	}
}

func (k *KubeContainerConfigProvider) addEntity(entity workloadmeta.Entity) {
	k.Lock()
	defer k.Unlock()
	switch e := entity.(type) {
	case *workloadmeta.KubernetesPod:
		id := e.GetID().ID
		k.podCache[id] = e
		log.Debugf("adding pod with ID %s\n", id)
		k.upToDate = false
	case *workloadmeta.Container:
		// TODO do 5 second container delay thing here
		// Delay logic may need tweaking because in a k8s situation,
		// addEntity will be called twice with a given container, once for the pod (which we don't care about)
		// and once from the runtime.
		// This means that we'll _always_ see double events for a container, unless we can get a way to distinguish
		// which containers are coming from which source (see above comment about 2 subscriptions)
		containerID := e.ID
		k.containerCache[containerID] = e
		log.Debugf("Adding entity container %s with labels %v\n", containerID, e.EntityMeta.Labels)
		k.upToDate = false
	}
}

func (k *KubeContainerConfigProvider) deleteEntity(entity workloadmeta.Entity) {
	k.Lock()
	defer k.Unlock()
	switch e := entity.(type) {
	case *workloadmeta.KubernetesPod:
		delete(k.podCache, entity.GetID().ID)
		log.Debugf("deleting pod with ID %s\n", entity.GetID().ID)
		k.upToDate = false
	case *workloadmeta.Container:
		// TODO do 5 second container delay thing here
		containerID := e.ID
		log.Debugf("deleting container %s\n", containerID)
		delete(k.containerCache, containerID)
		k.upToDate = false
	}
}
func (k *KubeContainerConfigProvider) generateConfigs() ([]integration.Config, error) {
	k.Lock()
	defer k.Unlock()

	adErrors := make(map[string]ErrorMsgSet)

	var configs []integration.Config

	log.Debugf("Generating Configs for %d containers\n", len(k.containerCache))
	for containerID, container := range k.containerCache {
		containerEntityName := containers.BuildEntityName(string(container.Runtime), containerID)
		c, errors := utils.ExtractTemplatesFromContainerLabels(containerEntityName, container.Labels)

		for _, err := range errors {
			log.Errorf("Can't parse template for container %s: %s", containerID, err)
			// TODO should add to 'adErrors'? Different from 'ContainerConfigProvider' but seems more correct
		}

		if util.CcaInAD() {
			c = utils.AddContainerCollectAllConfigs(c, containerEntityName)
		}

		for idx := range c {
			c[idx].Source = names.Container + ":" + containerEntityName
		}

		configs = append(configs, c...)
	}

	log.Debugf("Generating Configs for %d pods\n", len(k.podCache))
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

			log.Debugf("Considering pod %s with container id %s\n", pod.Name, podContainer.ID)
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
				continue
			}

			if util.CcaInAD() {
				_, trackedByContainer := k.containerCache[podContainer.ID]
				if !trackedByContainer {
					c = utils.AddContainerCollectAllConfigs(c, containerEntity)
				} else {
					log.Debugf("Pod %q has container %q, however container is tracked by containerCache, skipping log config creation...", pod.Name, podContainer.ID)
				}
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

	bldr := strings.Builder{}
	for _, c := range configs {
		fmt.Fprintf(&bldr, "  %s %s %s\n", c.Name, c.Source, c.Digest())
	}
	log.Infof("KubeContainerConfigProvider#generateConfigs generated:\n%s", bldr.String())

	k.configErrors = adErrors
	telemetry.Errors.Set(float64(len(adErrors)), names.Kubernetes)

	return configs, nil
}

// GetConfigErrors returns a map of configuration errors for each namespace/pod
func (k *KubeContainerConfigProvider) GetConfigErrors() map[string]ErrorMsgSet {
	k.RLock()
	defer k.RUnlock()
	return k.configErrors
}

// IsUpToDate checks whether we have new pods to parse, based on events
// received by the listen goroutine. If listening fails, we fallback to
// collecting everytime.
func (k *KubeContainerConfigProvider) IsUpToDate(ctx context.Context) (bool, error) {
	k.RLock()
	defer k.RUnlock()
	return k.streaming && k.upToDate, nil
}

func init() {
	RegisterProvider(names.KubeContainer, NewKubeContainerConfigProvider)
}
