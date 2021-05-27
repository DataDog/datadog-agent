// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// +build kubelet

package providers

import (
	"fmt"
	"strings"
	"sync"

	"github.com/DataDog/datadog-agent/pkg/autodiscovery/common/utils"
	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/autodiscovery/providers/names"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/kubelet"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// KubeletConfigProvider implements the ConfigProvider interface for the kubelet.
type KubeletConfigProvider struct {
	kubelet      kubelet.KubeUtilInterface
	configErrors map[string]ErrorMsgSet
	sync.Mutex
}

// NewKubeletConfigProvider returns a new ConfigProvider connected to kubelet.
// Connectivity is not checked at this stage to allow for retries, Collect will do it.
func NewKubeletConfigProvider(config config.ConfigurationProviders) (ConfigProvider, error) {
	return &KubeletConfigProvider{
		configErrors: make(map[string]ErrorMsgSet),
	}, nil
}

// String returns a string representation of the KubeletConfigProvider
func (k *KubeletConfigProvider) String() string {
	return names.Kubernetes
}

// Collect retrieves templates from the kubelet's podlist, builds Config objects and returns them
// TODO: cache templates and last-modified index to avoid future full crawl if no template changed.
func (k *KubeletConfigProvider) Collect() ([]integration.Config, error) {
	var err error
	if k.kubelet == nil {
		k.kubelet, err = kubelet.GetKubeUtil()
		if err != nil {
			return []integration.Config{}, err
		}
	}

	pods, err := k.kubelet.GetLocalPodList()
	if err != nil {
		return []integration.Config{}, err
	}

	return k.parseKubeletPodlist(pods)
}

// IsUpToDate updates the list of AD templates versions in the Agent's cache and checks the list is up to date compared to Kubernetes's data.
func (k *KubeletConfigProvider) IsUpToDate() (bool, error) {
	return false, nil
}

func (k *KubeletConfigProvider) parseKubeletPodlist(podlist []*kubelet.Pod) ([]integration.Config, error) {
	var configs []integration.Config
	var ADErrors = make(map[string]ErrorMsgSet)
	for _, pod := range podlist {
		// Filter out pods with no AD annotation
		var adExtractFormat string
		var errs []error
		for name := range pod.Metadata.Annotations {
			if strings.HasPrefix(name, utils.NewPodAnnotationPrefix) {
				adExtractFormat = utils.NewPodAnnotationFormat
				break
			}
			if strings.HasPrefix(name, utils.LegacyPodAnnotationPrefix) {
				adExtractFormat = utils.LegacyPodAnnotationFormat
				// Don't break so we try to look for the new prefix
				// which will take precedence
			}
		}
		if adExtractFormat == "" {
			continue
		}
		if adExtractFormat == utils.LegacyPodAnnotationFormat {
			log.Warnf("found legacy annotations %s for %s, please use the new prefix %s",
				utils.LegacyPodAnnotationPrefix, pod.Metadata.Name, utils.NewPodAnnotationPrefix)
		}

		containerIdentifiers := map[string]struct{}{}
		containerNames := map[string]struct{}{}

		for _, container := range pod.Status.GetAllContainers() {
			adIdentifier := container.Name
			containerNames[container.Name] = struct{}{}
			if customADIdentifier, customIDFound := utils.GetCustomCheckID(pod.Metadata.Annotations, container.Name); customIDFound {
				adIdentifier = customADIdentifier
			}
			containerIdentifiers[adIdentifier] = struct{}{}

			c, errors := extractTemplatesFromMap(container.ID, pod.Metadata.Annotations,
				fmt.Sprintf(adExtractFormat, adIdentifier))

			for _, err := range errors {
				log.Errorf("Can't parse template for pod %s: %s", pod.Metadata.Name, err)
				errs = append(errs, err)
			}

			for idx := range c {
				c[idx].Source = "kubelet:" + container.ID
			}

			configs = append(configs, c...)
		}
		errs = append(errs, utils.ValidateAnnotationsMatching(pod.Metadata.Annotations, containerIdentifiers, containerNames)...)
		namespacedName := pod.Metadata.Namespace + "/" + pod.Metadata.Name
		for _, err := range errs {
			if _, found := ADErrors[namespacedName]; !found {
				ADErrors[namespacedName] = map[string]struct{}{err.Error(): {}}
			} else {
				ADErrors[namespacedName][err.Error()] = struct{}{}
			}
		}
		k.Lock()
		k.configErrors = ADErrors
		k.Unlock()
	}
	return configs, nil
}

func init() {
	RegisterProvider("kubelet", NewKubeletConfigProvider)
}

// GetConfigErrors returns a map of configuration errors for each namespace/pod
func (k *KubeletConfigProvider) GetConfigErrors() map[string]ErrorMsgSet {
	k.Lock()
	defer k.Unlock()
	return k.configErrors
}
