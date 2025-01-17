// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package ownerdetectionimpl

import (
	"context"
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
	time.Sleep(time.Second * 5)

	// create a workloadmeta filter for pods
	filter := workloadmeta.NewFilterBuilder().
		SetSource(workloadmeta.SourceNodeOrchestrator).
		AddKind(workloadmeta.KindKubernetesPod).
		Build()

	// Look into filtering out UNSET events?

	eventCh := c.wmeta.Subscribe(name, workloadmeta.TaggerPriority, filter)

	log.Error("OwnerDetectionClient is started")

	// Subscribe and handle events
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case <-health.C:
			case evs := <-eventCh:
				evs.Acknowledge()
				log.Error("GABE: received an event")
				c.handleEvents(evs)
			}
		}
	}()

	// Periodically scan all pods
	for {
		select {
		case <-ctx.Done():
			err := health.Deregister()
			if err != nil {
				c.log.Warnf("error de-registering health check: %s", err)
			}
			log.Error("GABE: CONTEXT DONE")
			return
		default:
			pods := c.wmeta.ListKuberenetesPods()
			c.log.Infof("TODO handle pods: %v", pods)
			time.Sleep(time.Minute)
		}
	}
}

func (c *ownerDetectionClient) handleEvents(evs workloadmeta.EventBundle) {
	newEvents := make([]workloadmeta.Event, 0)

	log.Errorf("GABE: HANDLING EVENT BUNDLE %v", evs)

	// TODO: retry mechanism?
	dcaClient, err := clusteragent.GetClusterAgentClient()
	if err != nil {
		c.log.Error("Failed to get DCAClient")
		return
	}

	for _, ev := range evs.Events {
		switch ev.Type {
		case workloadmeta.EventTypeSet:
			pod, ok := ev.Entity.(*workloadmeta.KubernetesPod)
			if !ok {
				c.log.Errorf("GABE: Did not receieve pod")
				continue
			}
			if pod == nil {
				c.log.Errorf("GABE: Pod is nil")
				continue
			}

			log.Errorf("GABE: Received pod: %s", pod.Name)

			objectRelations := make([]k8stypes.ObjectRelation, 0)
			for _, owner := range pod.Owners {
				ownerRelations, err := dcaClient.GetOwnerReferences(pod.Namespace, owner.Name, owner.APIVersion, owner.Kind)
				log.Errorf("GABE: Owner relations from DCACLIENT: %s", ownerRelations)
				if err != nil {
					c.log.Debugf("Failed to get owner references for %s/%s: %s", pod.Namespace, owner.Name, err)
				}
				objectRelations = append(objectRelations, ownerRelations...)
			}

			// TODO: check for duplicated owners potentially and parse them out
			relatedOwners := make([]workloadmeta.KubernetesPodOwner, 0)
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
