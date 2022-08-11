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
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/containers"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/workloadmeta"
)

// KubeContainerConfigProvider implements the ConfigProvider interface for both pods and containers
// This provider is meant to replace both the `ContainerConfigProvider` and the `KubeletConfigProvider` components.
// Once the rollout is complete, `pkg/autodiscovery/providers/container.go` and `pkg/autodiscovery/providers/kubelet.go`
// should be deleted and this provider should be renamed to something more generic such as
// `ContainerConfigProvider`
type KubeContainerConfigProvider struct {
	workloadmetaStore workloadmeta.Store
	configErrors      map[string]ErrorMsgSet                   // map[entity name]ErrorMsgSet
	configCache       map[string]map[string]integration.Config // map[entity name]map[config digest]integration.Config
	mu                sync.RWMutex
}

// NewKubeContainerConfigProvider returns a new ConfigProvider subscribed to both container
// and pods
func NewKubeContainerConfigProvider(*config.ConfigurationProviders) (ConfigProvider, error) {
	return &KubeContainerConfigProvider{
		workloadmetaStore: workloadmeta.GetGlobalStore(),
		configCache:       make(map[string]map[string]integration.Config),
		configErrors:      make(map[string]ErrorMsgSet),
	}, nil
}

// String returns a string representation of the KubeContainerConfigProvider
func (k *KubeContainerConfigProvider) String() string {
	return names.KubeContainer
}

// Stream starts listening to workloadmeta to generate configs as they come
// instead of relying on a periodic call to Collect.
func (k *KubeContainerConfigProvider) Stream(ctx context.Context) <-chan integration.ConfigChanges {
	const name = "ad-kubecontainerprovider"

	// outCh must be unbuffered. processing of workloadmeta events must not
	// proceed until the config is processed by autodiscovery, as configs
	// need to be generated before any associated services.
	outCh := make(chan integration.ConfigChanges)

	inCh := k.workloadmetaStore.Subscribe(name, workloadmeta.ConfigProviderPriority, workloadmeta.NewFilter(
		[]workloadmeta.Kind{workloadmeta.KindKubernetesPod, workloadmeta.KindContainer},
		workloadmeta.SourceAll,
		workloadmeta.EventTypeAll,
	))

	go func() {
		for {
			select {
			case <-ctx.Done():
				k.workloadmetaStore.Unsubscribe(inCh)

			case evBundle, ok := <-inCh:
				if !ok {
					return
				}

				changes := k.processEvents(evBundle)
				if !changes.IsEmpty() {
					outCh <- changes
				}

				close(evBundle.Ch)
			}
		}
	}()

	return outCh
}

func (k *KubeContainerConfigProvider) processEvents(evBundle workloadmeta.EventBundle) integration.ConfigChanges {
	k.mu.Lock()
	defer k.mu.Unlock()

	changes := integration.ConfigChanges{}

	for _, event := range evBundle.Events {
		entityName := buildEntityName(event.Entity)

		switch event.Type {
		case workloadmeta.EventTypeSet:
			configs, err := k.generateConfig(event.Entity)

			if err != nil {
				k.configErrors[entityName] = err
			} else if _, ok := k.configErrors[entityName]; ok {
				delete(k.configErrors, entityName)
			}

			configsToAdd := make(map[string]integration.Config)
			for _, config := range configs {
				configsToAdd[config.Digest()] = config
			}

			oldConfigs, found := k.configCache[entityName]
			if found {
				for digest, config := range oldConfigs {
					_, ok := configsToAdd[digest]
					if ok {
						delete(configsToAdd, digest)
					} else {
						delete(k.configCache[entityName], digest)
						changes.Unschedule = append(changes.Unschedule, config)
					}
				}
			} else {
				k.configCache[entityName] = configsToAdd
			}

			for _, config := range configsToAdd {
				changes.Schedule = append(changes.Schedule, config)
			}

		case workloadmeta.EventTypeUnset:
			oldConfigs, found := k.configCache[entityName]
			if !found {
				log.Debugf("entity %q removed from workloadmeta store but not found in cache. skipping", entityName)
				continue
			}

			for _, oldConfig := range oldConfigs {
				changes.Unschedule = append(changes.Unschedule, oldConfig)
			}

			delete(k.configCache, entityName)
			delete(k.configErrors, entityName)

		default:
			log.Errorf("cannot handle event of type %d", event.Type)
		}
	}

	return changes
}

func (k *KubeContainerConfigProvider) generateConfig(e workloadmeta.Entity) ([]integration.Config, ErrorMsgSet) {
	var (
		errMsgSet ErrorMsgSet
		errs      []error
		configs   []integration.Config
	)

	switch entity := e.(type) {
	case *workloadmeta.Container:
		// kubernetes containers need to be handled together with their
		// pod, so they generate a single []integration.Config.
		// otherwise, with container_collect_all, it's possible for a
		// container that belongs to an AD-annotated pod to briefly
		// have a container_collect_all when it shouldn't.
		if !findKubernetesInLabels(entity.Labels) {
			configs, errs = k.generateContainerConfig(entity)
		}

	case *workloadmeta.KubernetesPod:
		containerIdentifiers := map[string]struct{}{}
		containerNames := map[string]struct{}{}
		for _, podContainer := range entity.Containers {
			container, err := k.workloadmetaStore.GetContainer(podContainer.ID)
			if err != nil {
				log.Debugf("Pod %q has reference to non-existing container %q", entity.Name, podContainer.ID)
				continue
			}

			var (
				c      []integration.Config
				errors []error
			)

			c, errors = k.generateContainerConfig(container)
			configs = append(configs, c...)
			errs = append(errs, errors...)

			adIdentifier := podContainer.Name
			if customADID, found := utils.ExtractCheckIDFromPodAnnotations(entity.Annotations, podContainer.Name); found {
				adIdentifier = customADID
			}

			containerEntity := containers.BuildEntityName(string(container.Runtime), container.ID)
			c, errors = utils.ExtractTemplatesFromPodAnnotations(
				containerEntity,
				entity.Annotations,
				adIdentifier,
			)

			if len(errors) > 0 {
				errs = append(errs, errors...)
				if len(c) == 0 {
					// Only got errors, no valid configs so
					// let's move on to the next container.
					continue
				}
			}

			containerIdentifiers[adIdentifier] = struct{}{}
			containerNames[podContainer.Name] = struct{}{}

			for idx := range c {
				c[idx].Source = names.Container + ":" + containerEntity
			}

			configs = append(configs, c...)
		}

		errs = append(errs, utils.ValidateAnnotationsMatching(
			entity.Annotations,
			containerIdentifiers,
			containerNames)...)

	default:
		log.Errorf("cannot handle entity of kind %s", e.GetID().Kind)
	}

	if len(errs) > 0 {
		errMsgSet = make(ErrorMsgSet)
		for _, err := range errs {
			errMsgSet[err.Error()] = struct{}{}
		}
	}

	return configs, errMsgSet
}

func (k *KubeContainerConfigProvider) generateContainerConfig(container *workloadmeta.Container) ([]integration.Config, []error) {
	var (
		errs    []error
		configs []integration.Config
	)

	containerID := container.ID
	containerEntityName := containers.BuildEntityName(string(container.Runtime), containerID)
	configs, errs = utils.ExtractTemplatesFromContainerLabels(containerEntityName, container.Labels)

	// AddContainerCollectAllConfigs is only needed when handling
	// the container event, even when the container belongs to a
	// pod. Calling it when handling the KubernetesPod will always
	// result in a duplicated config, as each KubernetesPod will
	// also generate events for each of its containers.
	configs = utils.AddContainerCollectAllConfigs(configs, containerEntityName)

	for idx := range configs {
		configs[idx].Source = names.Container + ":" + containerEntityName
	}

	return configs, errs
}

// GetConfigErrors returns a map of configuration errors for each namespace/pod
func (k *KubeContainerConfigProvider) GetConfigErrors() map[string]ErrorMsgSet {
	k.mu.RLock()
	defer k.mu.RUnlock()

	errors := make(map[string]ErrorMsgSet, len(k.configErrors))

	for entity, errset := range k.configErrors {
		errors[entity] = errset
	}

	return errors
}

func buildEntityName(e workloadmeta.Entity) string {
	entityID := e.GetID()
	switch entity := e.(type) {
	case *workloadmeta.KubernetesPod:
		return fmt.Sprintf("%s/%s", entity.Namespace, entity.Name)
	case *workloadmeta.Container:
		return containers.BuildEntityName(string(entity.Runtime), entityID.ID)
	default:
		return fmt.Sprintf("%s://%s", entityID.Kind, entityID.ID)
	}
}

// findKubernetesInLabels traverses a map of container labels and
// returns true if a kubernetes label is detected
func findKubernetesInLabels(labels map[string]string) bool {
	for name := range labels {
		if strings.HasPrefix(name, "io.kubernetes.") {
			return true
		}
	}
	return false
}

func init() {
	RegisterProvider(names.KubeContainer, NewKubeContainerConfigProvider)
}
