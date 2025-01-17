// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package ownerdetectionimpl

import (
	"context"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/status/health"
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

	// create a workloadmeta filter for pods
	filter := workloadmeta.NewFilterBuilder().
		SetSource(workloadmeta.SourceNodeOrchestrator).
		AddKind(workloadmeta.KindKubernetesPod).
		Build()

	// Look into filtering out UNSET events?

	eventCh := c.wmeta.Subscribe(name, workloadmeta.TaggerPriority, filter)

	log.Error("OwnerDetectionClient is started")

	for {
		select {
		case <-ctx.Done():
			err := health.Deregister()
			if err != nil {
				c.log.Warnf("error de-registering health check: %s", err)
			}
			log.Error("GABE: CONTEXT DONE")
			return
		case <-health.C:
		case evs := <-eventCh:
			evs.Acknowledge()
			log.Error("GABE: received an event")
			c.handleEvents(evs)
		}
	}
}

func (c *ownerDetectionClient) handleEvents(evs workloadmeta.EventBundle) {
	newEvents := make([]workloadmeta.Event, 0)

	for _, ev := range evs.Events {
		switch ev.Type {
		case workloadmeta.EventTypeSet:
			pod, ok := ev.Entity.(*workloadmeta.KubernetesPod)
			if !ok || pod == nil {
				continue
			}

			relatedOwners := make([]workloadmeta.KubernetesPodOwner, 0)
			objectRelations := make([]k8stypes.ObjectRelation, 0)

			for _, owner := range pod.Owners {

				cachedItems := c.ownerCache.GetParentTree(pod.Namespace, owner.Kind, owner.Name)
				if len(cachedItems) == 1 {
					// If we have a single parent, it's already the owner we know about
					log.Info("Gabe: Cache hit. No parents for %s/%s", owner.Kind, owner.Name)
					continue
				}
				if len(cachedItems) > 1 {
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

				ownerRelations, err := c.dcaClient.GetOwnerReferences(pod.Namespace, owner.Name, owner.APIVersion, owner.Kind)
				log.Errorf("GABE: Owner relations from DCACLIENT: %s", ownerRelations)
				if err != nil {
					c.log.Debugf("Failed to get owner references for %s/%s: %s", pod.Namespace, owner.Name, err)
				}
				objectRelations = append(objectRelations, ownerRelations...)

				// Add the owner to the cache
				c.ownerCache.AddParentTree(pod.Namespace, objectRelations)
			}

			// TODO: check for duplicated owners potentially and parse them out
			for _, relation := range objectRelations {
				relatedOwners = append(relatedOwners, workloadmeta.KubernetesPodOwner{
					Name:       relation.ParentName,
					Kind:       relation.ParentGVRK.Kind,
					APIVersion: relation.ParentGVRK.GetAPIVersion(),
				})
			}

			log.Errorf("GABE: Related owners: %s", relatedOwners)

			if len(relatedOwners) == 0 {
				continue
			}

			pod.RelatedOwners = relatedOwners
			newEvent := workloadmeta.Event{Type: workloadmeta.EventTypeSet, Entity: pod}
			newEvents = append(newEvents, newEvent)
		}
	}

	err := c.wmeta.Push(workloadmeta.SourceOwnerDetectionServer, newEvents...)
	if err != nil {
		c.log.Errorf("Failed to push events: %s", err)
	}
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
