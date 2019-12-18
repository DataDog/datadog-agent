// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

// +build kubelet

package providers

import (
	"fmt"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/util/log"

	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/autodiscovery/providers/names"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/kubelet"
)

const (
	newPodAnnotationPrefix    = "ad.datadoghq.com/"
	newPodAnnotationFormat    = newPodAnnotationPrefix + "%s."
	legacyPodAnnotationPrefix = "service-discovery.datadoghq.com/"
	legacyPodAnnotationFormat = legacyPodAnnotationPrefix + "%s."
)

// KubeletConfigProvider implements the ConfigProvider interface for the kubelet.
type KubeletConfigProvider struct {
	kubelet *kubelet.KubeUtil
}

// NewKubeletConfigProvider returns a new ConfigProvider connected to kubelet.
// Connectivity is not checked at this stage to allow for retries, Collect will do it.
func NewKubeletConfigProvider(config config.ConfigurationProviders) (ConfigProvider, error) {
	return &KubeletConfigProvider{}, nil
}

// String returns a string representation of the KubeletConfigProvider
func (k *KubeletConfigProvider) String() string {
	return names.Kubernetes
}

// Collect retrieves templates from the kubelet's pdolist, builds Config objects and returns them
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

	return parseKubeletPodlist(pods)
}

// Updates the list of AD templates versions in the Agent's cache and checks the list is up to date compared to Kubernetes's data.
func (k *KubeletConfigProvider) IsUpToDate() (bool, error) {
	return false, nil
}

func parseKubeletPodlist(podlist []*kubelet.Pod) ([]integration.Config, error) {
	var configs []integration.Config
	for _, pod := range podlist {
		// Filter out pods with no AD annotation
		var adExtractFormat string
		for name := range pod.Metadata.Annotations {
			if strings.HasPrefix(name, newPodAnnotationPrefix) {
				adExtractFormat = newPodAnnotationFormat
				break
			}
			if strings.HasPrefix(name, legacyPodAnnotationPrefix) {
				adExtractFormat = legacyPodAnnotationFormat
				// Don't break so we try to look for the new prefix
				// which will take precedence
			}
		}
		if adExtractFormat == "" {
			continue
		}
		if adExtractFormat == legacyPodAnnotationFormat {
			log.Warnf("found legacy annotations %s for %s, please use the new prefix %s",
				legacyPodAnnotationPrefix, pod.Metadata.Name, newPodAnnotationPrefix)
		}

		for _, container := range pod.Status.GetAllContainers() {
			c, errors := extractTemplatesFromMap(container.ID, pod.Metadata.Annotations,
				fmt.Sprintf(adExtractFormat, container.Name))

			for _, err := range errors {
				log.Errorf("Can't parse template for pod %s: %s", pod.Metadata.Name, err)
			}

			for idx := range c {
				c[idx].Source = "kubelet:" + container.ID
			}

			configs = append(configs, c...)
		}
	}
	return configs, nil
}

func init() {
	RegisterProvider("kubelet", NewKubeletConfigProvider)
}
