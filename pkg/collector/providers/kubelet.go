// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

// +build kubelet

package providers

import (
	"fmt"
	"strings"

	log "github.com/cihub/seelog"

	"github.com/DataDog/datadog-agent/pkg/collector/check"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/kubelet"
)

const (
	podAnnotationFormat = "service-discovery.datadoghq.com/%s."
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

func (k *KubeletConfigProvider) String() string {
	return "Kubernetes pod annotation"
}

// Collect retrieves templates from the kubelet's pdolist, builds Config objects and returns them
// TODO: cache templates and last-modified index to avoid future full crawl if no template changed.
func (k *KubeletConfigProvider) Collect() ([]check.Config, error) {
	var err error
	if k.kubelet == nil {
		k.kubelet, err = kubelet.GetKubeUtil()
		if err != nil {
			return []check.Config{}, err
		}
	}

	pods, err := k.kubelet.GetLocalPodList()
	if err != nil {
		return []check.Config{}, err
	}

	return parseKubeletPodlist(pods)
}

func parseKubeletPodlist(podlist []*kubelet.Pod) ([]check.Config, error) {
	var configs []check.Config
	for _, pod := range podlist {
		// Filter out pods with no AD annotation
		var hasAD bool
		for name := range pod.Metadata.Annotations {
			if strings.HasPrefix(name, "service-discovery.datadoghq.com/") {
				hasAD = true
				break
			}
		}
		if !hasAD {
			continue
		}

		for _, container := range pod.Status.Containers {
			c, err := extractTemplatesFromMap(container.ID, pod.Metadata.Annotations,
				fmt.Sprintf(podAnnotationFormat, container.Name))
			switch {
			case err != nil:
				log.Errorf("Can't parse template for pod %s: %s", pod.Metadata.Name, err)
				continue
			case len(c) == 0:
				continue
			default:
				configs = append(configs, c...)

			}
		}
	}
	return configs, nil
}

func init() {
	RegisterProvider("kubelet", NewKubeletConfigProvider)
}
