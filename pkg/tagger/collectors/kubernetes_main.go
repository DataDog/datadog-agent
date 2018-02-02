// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

// +build kubeapiserver,kubelet

package collectors

import (
	"fmt"
	"time"

	log "github.com/cihub/seelog"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/kubelet"
)

const (
	kubeServiceCollectorName = "kube-service-collector"
)

type KubeServiceCollector struct {
	kubeUtil  *kubelet.KubeUtil
	apiClient *apiserver.APIClient
	infoOut   chan<- []*TagInfo

	// used to set a custom delay
	lastUpdate time.Time
	updateFreq time.Duration
}

// Detect tries to connect to the kubelet
// TODO refactor when we have the DCA
func (c *KubeServiceCollector) Detect(out chan<- []*TagInfo) (CollectionMode, error) {
	if config.Datadog.GetBool("kubernetes_collect_service_tags") == false {
		return NoCollection, fmt.Errorf("collection disabled by the configuration")
	}

	var err error
	c.kubeUtil, err = kubelet.GetKubeUtil()
	if err != nil {
		return NoCollection, err
	}
	c.apiClient, err = apiserver.GetAPIClient()
	if err != nil {
		return NoCollection, err
	}

	c.infoOut = out
	c.updateFreq = time.Duration(config.Datadog.GetInt("kubernetes_service_tag_update_freq")) * time.Second
	return PullCollection, nil
}

// Pull implements an additional time constraints to avoid exhausting the kube-apiserver
// TODO refactor when we have the DCA
func (c *KubeServiceCollector) Pull() error {
	// Time constraints, get the delta in seconds to display it in the logs:
	timeDelta := c.lastUpdate.Add(c.updateFreq).Unix() - time.Now().Unix()
	if timeDelta > 0 {
		log.Tracef("skipping, next effective Pull will be in %s seconds", timeDelta)
		return nil
	}

	pods, err := c.kubeUtil.GetLocalPodList()
	if err != nil {
		return err
	}

	// TODO, remove this because we are acting like the DCA API
	err = c.addToCacheServiceMapping(pods)
	if err != nil {
		log.Debugf("cannot add the serviceMapping to cache: %s", err)
	}

	c.infoOut <- getTagInfos(pods)
	c.lastUpdate = time.Now()
	return nil
}

// Fetch fetches tags for a given container by iterating on the whole podlist and
// the serviceMapper, TODO refactor when we have the DCA
func (c *KubeServiceCollector) Fetch(containerID string) ([]string, []string, error) {
	var lowCards, highCards []string

	pod, err := c.kubeUtil.GetPodForContainerID(containerID)
	if err != nil {
		return lowCards, highCards, err
	}

	if kubelet.IsPodReady(pod) == false {
		return lowCards, highCards, ErrNotFound
	}

	pods := []*kubelet.Pod{pod}
	c.addToCacheServiceMapping(pods)

	tagInfos := getTagInfos(pods)
	c.infoOut <- tagInfos
	for _, info := range tagInfos {
		if info.Entity == containerID {
			return info.LowCardTags, info.HighCardTags, nil
		}
	}
	return lowCards, highCards, ErrNotFound
}

func kubernetesFactory() Collector {
	return &KubeServiceCollector{}
}

func init() {
	registerCollector(kubeServiceCollectorName, kubernetesFactory)
}
