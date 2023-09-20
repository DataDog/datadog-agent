// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver && kubelet

package kubemetadata

import (
	"context"
	"fmt"
	"strings"
	"time"

	"k8s.io/apimachinery/pkg/util/sets"

	apiv1 "github.com/DataDog/datadog-agent/pkg/clusteragent/api/v1"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/errors"
	"github.com/DataDog/datadog-agent/pkg/util/clusteragent"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/kubelet"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/retry"
	"github.com/DataDog/datadog-agent/pkg/workloadmeta"
)

const (
	collectorID   = "kube_metadata"
	componentName = "workloadmeta-kube_metadata"
)

type collector struct {
	store                  workloadmeta.Store
	seen                   map[workloadmeta.EntityID]struct{}
	kubeUtil               kubelet.KubeUtilInterface
	apiClient              *apiserver.APIClient
	dcaClient              clusteragent.DCAClientInterface
	dcaEnabled             bool
	updateFreq             time.Duration
	lastUpdate             time.Time
	collectNamespaceLabels bool
}

func init() {
	workloadmeta.RegisterCollector(collectorID, func() workloadmeta.Collector {
		return &collector{
			seen: make(map[workloadmeta.EntityID]struct{}),
		}
	})
}

// Start tries to connect to the kubelet, the DCA and the API Server if the DCA is not available.
func (c *collector) Start(_ context.Context, store workloadmeta.Store) error {
	if !config.IsFeaturePresent(config.Kubernetes) {
		return errors.NewDisabled(componentName, "Agent is not running on Kubernetes")
	}

	c.store = store

	var err error
	c.kubeUtil, err = kubelet.GetKubeUtil()
	if err != nil {
		return err
	}

	// If DCA is enabled and can't communicate with the DCA, let worloadmeta retry.
	var errDCA error
	if config.Datadog.GetBool("cluster_agent.enabled") {
		c.dcaEnabled = false
		c.dcaClient, errDCA = clusteragent.GetClusterAgentClient()
		if errDCA != nil {
			log.Errorf("Could not initialise the communication with the cluster agent: %s", errDCA.Error())

			// Continue to retry while we can
			if retry.IsErrWillRetry(errDCA) {
				return errDCA
			}

			// We return the permanent fail only if fallback is disabled
			if retry.IsErrPermaFail(errDCA) && !config.Datadog.GetBool("cluster_agent.tagging_fallback") {
				return errDCA
			}

			log.Error("Permanent failure in communication with the cluster agent, will fallback to local service mapper")
		} else {
			c.dcaEnabled = true
		}
	}

	// Fallback to local metamapper if DCA not enabled, or in permafail state with fallback enabled.
	if !config.Datadog.GetBool("cluster_agent.enabled") || errDCA != nil {
		// Using GetAPIClient as error returned follows the IsErrWillRetry/IsErrPermaFail
		// Workloadmeta will retry calling this method until permafail
		c.apiClient, err = apiserver.GetAPIClient()
		if err != nil {
			return err
		}
	}

	c.updateFreq = time.Duration(config.Datadog.GetInt("kubernetes_metadata_tag_update_freq")) * time.Second
	c.collectNamespaceLabels = len(config.Datadog.GetStringMapString("kubernetes_namespace_labels_as_tags")) > 0

	return err
}

// Pull triggers an event collection from kubelet and the Datadog Cluster Agent.
func (c *collector) Pull(ctx context.Context) error {
	// Time constraints, get the delta in seconds to display it in the logs:
	timeDelta := c.lastUpdate.Add(c.updateFreq).Unix() - time.Now().Unix()
	if timeDelta > 0 {
		log.Tracef("skipping, next effective Pull will be in %d seconds", timeDelta)
		return nil
	}

	pods, err := c.kubeUtil.GetLocalPodList(ctx)
	if err != nil {
		return err
	}

	if !c.isDCAEnabled() {
		// If the DCA is not used, each agent stores a local cache of the MetadataMap.
		err = c.addToCacheMetadataMapping(pods)
		if err != nil {
			log.Debugf("Cannot add the metadataMapping to cache: %v", err)
		}
	}

	seen := make(map[workloadmeta.EntityID]struct{})
	events, err := c.parsePods(ctx, pods, seen)
	if err != nil {
		return err
	}

	for seenID := range c.seen {
		if _, ok := seen[seenID]; ok {
			continue
		}

		events = append(events, workloadmeta.CollectorEvent{
			Type:   workloadmeta.EventTypeUnset,
			Source: workloadmeta.SourceClusterOrchestrator,
			Entity: &workloadmeta.KubernetesPod{
				EntityID: seenID,
			},
		})
	}

	c.seen = seen

	c.store.Notify(events)

	c.lastUpdate = time.Now()

	return nil
}

// parsePods returns collection events based on a given podlist.
func (c *collector) parsePods(
	ctx context.Context,
	pods []*kubelet.Pod,
	seen map[workloadmeta.EntityID]struct{},
) ([]workloadmeta.CollectorEvent, error) {
	events := []workloadmeta.CollectorEvent{}

	var err error
	var metadataByNsPods apiv1.NamespacesPodsStringsSet
	if c.isDCAEnabled() && c.dcaClient.Version().Major >= 1 && c.dcaClient.Version().Minor >= 3 {
		var nodeName string
		nodeName, err = c.kubeUtil.GetNodename(ctx)
		if err != nil {
			log.Errorf("Could not retrieve the Nodename, err: %v", err)
			return events, err
		}

		metadataByNsPods, err = c.dcaClient.GetPodsMetadataForNode(nodeName)
		if err != nil {
			log.Debugf("Could not pull the metadata map of pods on node %s from the Datadog Cluster Agent: %s", nodeName, err.Error())
			return events, err
		}
	}

	for _, pod := range pods {
		if pod.Metadata.UID == "" {
			continue
		}

		metadata, err := c.getMetadata(apiserver.GetPodMetadataNames, metadataByNsPods, pod)
		if err != nil {
			log.Debugf("Could not fetch metadata for pod %s/%s: %v", pod.Metadata.Namespace, pod.Metadata.Name, err)
		}

		// Skip `kube_service` label for pods that are not ready (since their endpoint will be disabled from the service)
		// Skip pods with hostNetwork because we cannot use their IP to match endpoints.
		services := []string{}
		if !pod.Spec.HostNetwork && kubelet.IsPodReady(pod) {
			for _, data := range metadata {
				d := strings.Split(data, ":")
				switch len(d) {
				case 1:
					// c.dcaClient.GetPodsMetadataForNode returns only a list of services without tag key
					services = append(services, d[0])
				case 2:
					services = append(services, d[1])
				default:
					continue
				}
			}
		}

		var nsLabels map[string]string
		nsLabels, err = c.getNamespaceLabels(apiserver.GetNamespaceLabels, pod.Metadata.Namespace)
		if err != nil {
			log.Debugf("Could not fetch namespace labels for pod %s/%s: %v", pod.Metadata.Namespace, pod.Metadata.Name, err)
		}

		entityID := workloadmeta.EntityID{
			Kind: workloadmeta.KindKubernetesPod,
			ID:   pod.Metadata.UID,
		}

		seen[entityID] = struct{}{}

		entity := &workloadmeta.KubernetesPod{
			EntityID: entityID,
			EntityMeta: workloadmeta.EntityMeta{
				Name:        pod.Metadata.Name,
				Namespace:   pod.Metadata.Namespace,
				Annotations: pod.Metadata.Annotations,
				Labels:      pod.Metadata.Labels,
			},
			KubeServices:    services,
			NamespaceLabels: nsLabels,
		}

		events = append(events, workloadmeta.CollectorEvent{
			Source: workloadmeta.SourceClusterOrchestrator,
			Type:   workloadmeta.EventTypeSet,
			Entity: entity,
		})
	}

	return events, nil
}

// getMetadata returns the cluster level metadata (kube service only currently).
func (c *collector) getMetadata(getPodMetaDataFromAPIServerFunc func(string, string, string) ([]string, error), metadataByNsPods apiv1.NamespacesPodsStringsSet, po *kubelet.Pod) ([]string, error) {
	if !c.isDCAEnabled() {
		metadataNames, err := getPodMetaDataFromAPIServerFunc(po.Spec.NodeName, po.Metadata.Namespace, po.Metadata.Name)
		if err != nil {
			err = fmt.Errorf("could not fetch cluster level tags of pod: %s, %v", po.Metadata.Name, err)
		}
		return metadataNames, err
	}

	if metadataByNsPods != nil {
		if data, ok := metadataByNsPods[po.Metadata.Namespace][po.Metadata.Name]; ok && data != nil {
			return sets.List(data), nil
		}
		return nil, nil
	}

	metadataNames, err := c.dcaClient.GetKubernetesMetadataNames(po.Spec.NodeName, po.Metadata.Namespace, po.Metadata.Name)
	if err != nil {
		err = fmt.Errorf("could not pull the metadata map of pod %s on node %s, %v", po.Metadata.Name, po.Spec.NodeName, err)
	}

	return metadataNames, err
}

// getNamespaceLabels returns the namespace labels, fast return if namespace labels as tags is disabled.
func (c *collector) getNamespaceLabels(getNamespaceLabelsFromAPIServerFunc func(string) (map[string]string, error), ns string) (map[string]string, error) {
	if !c.collectNamespaceLabels {
		return nil, nil
	}

	if c.isDCAEnabled() {
		getNamespaceLabelsFromAPIServerFunc = c.dcaClient.GetNamespaceLabels
	}

	return getNamespaceLabelsFromAPIServerFunc(ns)
}

func (c *collector) isDCAEnabled() bool {
	if c.dcaEnabled && c.dcaClient != nil {
		v := c.dcaClient.Version()
		if v.String() != "0.0.0" { // means not initialized
			return true
		}
	}

	return false
}

// addToCacheMetadataMapping is acting like the DCA at the node level.
func (c *collector) addToCacheMetadataMapping(kubeletPodList []*kubelet.Pod) error {
	if len(kubeletPodList) == 0 {
		log.Debug("Empty kubelet pod list")
		return nil
	}

	reachablePods := make([]*kubelet.Pod, 0)
	nodeName := ""
	for _, p := range kubeletPodList {
		if p.Status.PodIP == "" {
			continue
		}
		if nodeName == "" && p.Spec.NodeName != "" {
			nodeName = p.Spec.NodeName
		}
		reachablePods = append(reachablePods, p)
	}

	return c.apiClient.NodeMetadataMapping(nodeName, reachablePods)
}
