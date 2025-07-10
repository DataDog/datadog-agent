// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !serverless

package providers

import (
	"context"
	"fmt"
	"maps"
	"strings"
	"sync"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/common/utils"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/providers/names"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/providers/types"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/telemetry"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/util/containers"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// ContainerConfigProvider implements the ConfigProvider interface for both pods and containers
type ContainerConfigProvider struct {
	workloadmetaStore workloadmeta.Component
	configErrors      map[string]types.ErrorMsgSet             // map[entity name]types.ErrorMsgSet
	configCache       map[string]map[string]integration.Config // map[entity name]map[config digest]integration.Config
	mu                sync.RWMutex
	telemetryStore    *telemetry.Store
}

// NewContainerConfigProvider returns a new ConfigProvider subscribed to both container
// and pods
func NewContainerConfigProvider(_ *pkgconfigsetup.ConfigurationProviders, wmeta workloadmeta.Component, telemetryStore *telemetry.Store) (types.ConfigProvider, error) {
	return &ContainerConfigProvider{
		workloadmetaStore: wmeta,
		configCache:       make(map[string]map[string]integration.Config),
		configErrors:      make(map[string]types.ErrorMsgSet),
		telemetryStore:    telemetryStore,
	}, nil
}

// String returns a string representation of the ContainerConfigProvider
func (k *ContainerConfigProvider) String() string {
	return names.KubeContainer
}

// Stream starts listening to workloadmeta to generate configs as they come
// instead of relying on a periodic call to Collect.
func (k *ContainerConfigProvider) Stream(ctx context.Context) <-chan integration.ConfigChanges {
	const name = "ad-kubecontainerprovider"

	// outCh must be unbuffered. processing of workloadmeta events must not
	// proceed until the config is processed by autodiscovery, as configs
	// need to be generated before any associated services.
	outCh := make(chan integration.ConfigChanges)

	filter := workloadmeta.NewFilterBuilder().
		AddKind(workloadmeta.KindContainer).
		AddKind(workloadmeta.KindKubernetesPod).
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

func (k *ContainerConfigProvider) processEvents(evBundle workloadmeta.EventBundle) integration.ConfigChanges {
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
			} else {
				delete(k.configErrors, entityName)
			}

			configCache, ok := k.configCache[entityName]
			if !ok {
				configCache = make(map[string]integration.Config)
				k.configCache[entityName] = configCache
			}

			configsToUnschedule := make(map[string]integration.Config)
			maps.Copy(configsToUnschedule, configCache)

			for _, config := range configs {
				digest := config.Digest()
				if _, ok := configCache[digest]; ok {
					delete(configsToUnschedule, digest)
				} else {
					configCache[digest] = config
					changes.ScheduleConfig(config)
				}
			}

			for oldDigest, oldConfig := range configsToUnschedule {
				delete(configCache, oldDigest)
				changes.UnscheduleConfig(oldConfig)
			}

		case workloadmeta.EventTypeUnset:
			oldConfigs, found := k.configCache[entityName]
			if !found {
				log.Debugf("entity %q removed from workloadmeta store but not found in cache. skipping", entityName)
				continue
			}

			for _, oldConfig := range oldConfigs {
				changes.UnscheduleConfig(oldConfig)
			}

			delete(k.configCache, entityName)
			delete(k.configErrors, entityName)

		default:
			log.Errorf("cannot handle event of type %d", event.Type)
		}
	}

	if k.telemetryStore != nil {
		k.telemetryStore.Errors.Set(float64(len(k.configErrors)), names.KubeContainer)
	}

	return changes
}

func (k *ContainerConfigProvider) generateConfig(e workloadmeta.Entity) ([]integration.Config, types.ErrorMsgSet) {
	var (
		errMsgSet types.ErrorMsgSet
		errs      []error
		configs   []integration.Config
	)

	switch entity := e.(type) {
	case *workloadmeta.Container:
		// kubernetes containers need to be handled together with their
		// pod, so they generate a single []integration.Config.
		// otherwise, it's possible for a container that belongs to an
		// AD-annotated pod to briefly be scheduled without its
		// annotations.
		if !findKubernetesInLabels(entity.Labels) {
			configs, errs = k.generateContainerConfig(entity)

			containerID := entity.ID
			containerEntityName := containers.BuildEntityName(string(entity.Runtime), containerID)
			configs = utils.AddContainerCollectAllConfigs(configs, containerEntityName)

			for idx := range configs {
				configs[idx].Source = names.Container + ":" + containerEntityName
			}
		}

	case *workloadmeta.KubernetesPod:
		containerIdentifiers := map[string]struct{}{}
		containerNames := map[string]struct{}{}
		for _, podContainer := range entity.GetAllContainers() {
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
			c, errors = utils.ExtractTemplatesFromAnnotations(
				containerEntity,
				entity.Annotations,
				adIdentifier,
			)

			// container_collect_all configs must be added after
			// configs generated from annotations, since services
			// are reconciled against configs one-by-one instead of
			// as a set, so if a container_collect_all config
			// appears before an annotation one, it'll cause a logs
			// config to be scheduled as container_collect_all,
			// unscheduled, and then re-scheduled correctly.
			c = utils.AddContainerCollectAllConfigs(c, containerEntity)

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
		errMsgSet = make(types.ErrorMsgSet)
		for _, err := range errs {
			errMsgSet[err.Error()] = struct{}{}
		}
	}

	return configs, errMsgSet
}

func (k *ContainerConfigProvider) generateContainerConfig(container *workloadmeta.Container) ([]integration.Config, []error) {
	var (
		errs    []error
		configs []integration.Config
	)

	containerID := container.ID
	containerEntityName := containers.BuildEntityName(string(container.Runtime), containerID)
	configs, errs = utils.ExtractTemplatesFromContainerLabels(containerEntityName, container.Labels)

	return configs, errs
}

// GetConfigErrors returns a map of configuration errors for each namespace/pod
func (k *ContainerConfigProvider) GetConfigErrors() map[string]types.ErrorMsgSet {
	k.mu.RLock()
	defer k.mu.RUnlock()

	errors := make(map[string]types.ErrorMsgSet, len(k.configErrors))

	maps.Copy(errors, k.configErrors)

	return errors
}

// buildEntityName is also used as display key in `agent status` "Configuration Errors" display.
// (for instance, incorrect annotation syntax or unknown container name).
// That's why it does not simply use Kind + ID.
// It needs to be unique over time.
// (for instance, namespace+name is not unique for a POD as it can be deleted/created with a different UID, see STS rollout)
func buildEntityName(e workloadmeta.Entity) string {
	entityID := e.GetID()
	switch entity := e.(type) {
	case *workloadmeta.KubernetesPod:
		return fmt.Sprintf("%s/%s (%s)", entity.Namespace, entity.Name, entity.ID)
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
