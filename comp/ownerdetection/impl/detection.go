// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package ownerdetectionimpl

import (
	"context"
	"strings"
	"time"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/status/health"
	"github.com/DataDog/datadog-agent/pkg/util/clusteragent"
	k8stypes "github.com/DataDog/datadog-agent/pkg/util/kubernetes/types"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

func (c *ownerDetectionClient) start(ctx context.Context) {
	const name = "owner-detection"

	health := health.RegisterLiveness(name)
	defer func() {
		err := health.Deregister()
		if err != nil {
			c.log.Warnf("error de-registering health check: %s", err)
		}
	}()

	// Sleep during start just to give the DCA a chance to start up
	time.Sleep(time.Second * 10)

	filter := workloadmeta.NewFilterBuilder().
		SetSource(workloadmeta.SourceNodeOrchestrator).
		AddKind(workloadmeta.KindKubernetesPod).
		Build()
	eventCh := c.wmeta.Subscribe(name, workloadmeta.TaggerPriority, filter)

	// Every minute, scan all the pods to detect leaks
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			err := health.Deregister()
			if err != nil {
				c.log.Warnf("error de-registering health check: %s", err)
			}
			return
		case <-health.C:
		case evs := <-eventCh:
			evs.Acknowledge()
			c.handleEvents(evs)
		case <-ticker.C:
			pods := c.wmeta.ListKuberenetesPods()
			c.handlePods(c.getNewPods(pods))
		}
	}
}

func (c *ownerDetectionClient) handlePods(pods []*workloadmeta.KubernetesPod) {
	newEvents := make([]workloadmeta.Event, 0)

	dcaClient, err := clusteragent.GetClusterAgentClient()
	if err != nil {
		c.log.Error("Failed to get DCAClient")
		return
	}

	for _, pod := range pods {
		relatedOwners := make([]workloadmeta.KubernetesPodOwner, 0)
		objectRelations := make([]k8stypes.ObjectRelation, 0)

		for _, owner := range pod.Owners {

			// First: check the cache for the owner
			cachedItems := c.ownerCache.GetParentTree(pod.Namespace, owner.Kind, owner.Name)
			if len(cachedItems) == 1 {
				// If we have a single parent, it's already the owner we know about
				log.Infof("Gabe: Cache hit. No parents for %s/%s", owner.Kind, owner.Name)
				continue
			}
			if len(cachedItems) > 1 {
				// If we have multiple cached items, we have a parent tree (grandParents+)
				log.Infof("Gabe: Cache hit. Parents for %s/%s", owner.Kind, owner.Name)
				for _, item := range cachedItems {
					// Ignore the known parent
					if item.Name == owner.Name && item.GVKR.Kind == owner.Kind {
						continue
					}
					relatedOwners = append(relatedOwners, workloadmeta.KubernetesPodOwner{
						Name:       item.Name,
						Kind:       item.GVKR.Kind,
						APIVersion: item.GVKR.GetAPIVersion(),
					})
				}
				continue
			}

			ownerRelations, err := dcaClient.GetOwnerReferences(pod.Namespace, owner.Name, owner.APIVersion, owner.Kind)
			log.Infof("Gabe: Cache miss. Parents for %s/%s: %v", owner.Kind, owner.Name, ownerRelations)
			if err != nil {
				c.log.Debugf("Failed to get owner references for %s/%s: %s", pod.Namespace, owner.Name, err)
			}
			objectRelations = append(objectRelations, ownerRelations...)

			// Add the owner to the cache
			c.ownerCache.AddParentTree(pod.Namespace, objectRelations)

			// Edge case: the owner has no related entites, but we want to record it in the cache
			if len(objectRelations) == 0 {
				ownerGroup, ownerVersion := parseAPIVersion(owner.APIVersion)
				ownerGVRK := k8stypes.GroupVersionResourceKind{
					Kind:    owner.Kind,
					Group:   ownerGroup,
					Version: ownerVersion,
				}
				c.ownerCache.AddSingleParent(pod.Namespace, ownerGVRK, owner.Name)
			}
		}

		// TODO: check for duplicated owners potentially and parse them out
		for _, relation := range objectRelations {
			relatedOwners = append(relatedOwners, workloadmeta.KubernetesPodOwner{
				Name:       relation.ParentName,
				Kind:       relation.ParentGVRK.Kind,
				APIVersion: relation.ParentGVRK.GetAPIVersion(),
			})
		}

		if len(relatedOwners) == 0 {
			continue
		}

		pod.RelatedOwners = relatedOwners
		newEvent := workloadmeta.Event{Type: workloadmeta.EventTypeSet, Entity: pod}
		newEvents = append(newEvents, newEvent)
	}

	err = c.wmeta.Push(workloadmeta.SourceOwnerDetectionServer, newEvents...)
	if err != nil {
		c.log.Errorf("Failed to push events: %s", err)
	}
}

func (c *ownerDetectionClient) handleEvents(evs workloadmeta.EventBundle) {
	pods := make([]*workloadmeta.KubernetesPod, 0)
	for _, ev := range evs.Events {
		switch ev.Type {
		case workloadmeta.EventTypeSet:
			pod, ok := ev.Entity.(*workloadmeta.KubernetesPod)
			if !ok || pod == nil {
				continue
			}
			pods = append(pods, pod)
		}
	}
	c.handlePods(pods)
}

// getNewPods that are not detected in the cache
func (c *ownerDetectionClient) getNewPods(pods []*workloadmeta.KubernetesPod) []*workloadmeta.KubernetesPod {
	returnPods := make([]*workloadmeta.KubernetesPod, 0)
	for _, pod := range pods {
		for _, owner := range pod.Owners {
			cachedItems := c.ownerCache.GetParentTree(pod.Namespace, owner.Kind, owner.Name)
			if len(cachedItems) == 0 {
				returnPods = append(returnPods, pod)
			}
		}
	}
	return returnPods
}

func parseAPIVersion(apiVersion string) (group, version string) {
	parts := strings.Split(apiVersion, "/")
	if len(parts) == 2 {
		return parts[0], parts[1] // group and version
	}
	return "", apiVersion // core group
}

// func removeDuplicateStr(s []string) []string {
// 	keys := make(map[string]bool)
// 	list := []string{}
// 	for _, item := range s {
// 		if _, value := keys[item]; !value {
// 			keys[item] = true
// 			list = append(list, item)
// 		}
// 	}
// 	return list
// }
