// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package agentlifecycleimpl implements the experimental Agent startup gate.
package agentlifecycleimpl

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"strings"
	"time"

	agentlifecycle "github.com/DataDog/datadog-agent/comp/core/agentlifecycle/def"
	"github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	compdef "github.com/DataDog/datadog-agent/comp/def"
)

const (
	siblingPollInterval = time.Second
	rolloutEnabledKey   = "experimental.node_agent_rollout.enabled"
	rolloutPodUIDKey    = "experimental.node_agent_rollout.pod_uid"

	freshInstallObservations = 2
	missingOlderObservations = 2
)

type podOwner struct {
	kind       string
	uid        string
	controller bool
}

type localContainer struct {
	name       string
	terminated bool
	ready      bool
}

type localPod struct {
	uid                string
	name               string
	namespace          string
	createdAt          time.Time
	deletionTimestamp  *time.Time
	owners             []podOwner
	declaredContainers []string
	containers         []localContainer
}

type localPodSource interface {
	ListLocalPods(context.Context) ([]localPod, error)
}

type dependencies struct {
	compdef.In

	Config config.Component
	Log    log.Component
	Params agentlifecycle.Params
}

type component struct {
	enabled       bool
	componentName string
	podUID        string
	log           log.Component
	pods          localPodSource
	pollInterval  time.Duration
}

var _ agentlifecycle.Component = (*component)(nil)

// NewComponent creates the experimental Agent startup gate.
func NewComponent(deps dependencies) (agentlifecycle.Component, error) {
	return newComponent(deps, newLocalPodSource(), runtime.GOOS)
}

func newComponent(deps dependencies, pods localPodSource, goos string) (agentlifecycle.Component, error) {
	if !deps.Config.GetBool(rolloutEnabledKey) {
		return &component{}, nil
	}
	if goos != "linux" {
		return nil, fmt.Errorf("experimental node Agent rollout is Linux-only (running on %s)", goos)
	}
	if deps.Params.ComponentName == "" || strings.ContainsAny(deps.Params.ComponentName, `/\\`) {
		return nil, errors.New("experimental node Agent rollout requires a path-safe component name")
	}
	podUID := strings.TrimSpace(deps.Config.GetString(rolloutPodUIDKey))
	if podUID == "" {
		return nil, fmt.Errorf("%s must identify this Pod", rolloutPodUIDKey)
	}
	c := &component{
		enabled:       true,
		componentName: deps.Params.ComponentName,
		podUID:        podUID,
		log:           deps.Log,
		pods:          pods,
		pollInterval:  siblingPollInterval,
	}
	return c, nil
}

// Wait leaves the executable asleep before later Fx construction until the
// older instance of this same container has stopped. Kubelet errors fail closed.
func (c *component) Wait(ctx context.Context) (err error) {
	if !c.enabled {
		return nil
	}
	ticker := time.NewTicker(c.pollInterval)
	defer ticker.Stop()
	// The value records that kubelet has shown deletion beginning for the Pod.
	// A later omission is only a safe handoff signal after that observation.
	knownOlder := map[string]bool{}
	missingOlder := map[string]int{}
	olderObservedLogged := false
	emptyObservations := 0
	pollErrorLogged := false

	for {
		pods, err := c.pods.ListLocalPods(ctx)
		if err == nil {
			var older []localPod
			older, err = olderSiblingPods(pods, c.podUID)
			if err == nil {
				for i := range older {
					knownOlder[older[i].uid] = knownOlder[older[i].uid] || older[i].deletionTimestamp != nil
				}

				if len(knownOlder) == 0 {
					emptyObservations++
					if emptyObservations >= freshInstallObservations && (c.componentName == "agent" || replacementCoreReady(pods, c.podUID)) {
						c.log.Infof("%s found no older container on the node and is starting", c.componentName)
						return nil
					}
				} else {
					emptyObservations = 0
					if !olderObservedLogged {
						olderObservedLogged = true
						c.log.Infof("%s is waiting for its older container to stop", c.componentName)
					}
					if err == nil && olderContainersStopped(older, knownOlder, missingOlder, c.componentName) {
						if c.componentName == "agent" {
							c.log.Info("the older core Agent container stopped; starting")
							return nil
						}
						if replacementCoreReady(pods, c.podUID) {
							c.log.Infof("the older %s container stopped and the replacement core Agent is ready; starting", c.componentName)
							return nil
						}
					}
				}
			}
		}
		if err != nil {
			if !pollErrorLogged {
				c.log.Warnf("%s cannot verify node-local containers; remaining asleep: %v", c.componentName, err)
				pollErrorLogged = true
			}
		} else {
			pollErrorLogged = false
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

func replacementCoreReady(pods []localPod, selfUID string) bool {
	for i := range pods {
		if pods[i].uid != selfUID {
			continue
		}
		for j := range pods[i].containers {
			container := pods[i].containers[j]
			if container.name == "agent" {
				return container.ready && !container.terminated
			}
		}
		return false
	}
	return false
}

func olderSiblingPods(pods []localPod, selfUID string) ([]localPod, error) {
	var self *localPod
	for i := range pods {
		if pods[i].uid == selfUID {
			if self != nil {
				return nil, fmt.Errorf("kubelet returned duplicate entries for self Pod UID %q", selfUID)
			}
			self = &pods[i]
		}
	}
	if self == nil {
		return nil, fmt.Errorf("self Pod UID %q is absent from the kubelet Pod list", selfUID)
	}
	ownerUID, err := daemonSetOwnerUID(*self)
	if err != nil {
		return nil, err
	}
	if self.createdAt.IsZero() {
		return nil, fmt.Errorf("self Pod %s/%s has no creation timestamp", self.namespace, self.name)
	}

	var older []localPod
	for i := range pods {
		candidate := pods[i]
		if candidate.uid == selfUID || candidate.namespace != self.namespace {
			continue
		}
		candidateOwnerUID, ownerErr := daemonSetOwnerUID(candidate)
		if ownerErr != nil || candidateOwnerUID != ownerUID {
			continue
		}
		if candidate.createdAt.IsZero() || candidate.createdAt.Before(self.createdAt) {
			older = append(older, candidate)
			continue
		}
		if candidate.createdAt.Equal(self.createdAt) {
			return nil, fmt.Errorf("cannot order same-timestamp Pods %s/%s and %s/%s", candidate.namespace, candidate.name, self.namespace, self.name)
		}
	}
	return older, nil
}

func olderContainersStopped(current []localPod, known map[string]bool, missing map[string]int, componentName string) bool {
	byUID := make(map[string]localPod, len(current))
	for i := range current {
		byUID[current[i].uid] = current[i]
	}
	for uid, deletionObserved := range known {
		pod, present := byUID[uid]
		if !present {
			missing[uid]++
			if !deletionObserved && missing[uid] < missingOlderObservations {
				return false
			}
			continue
		}
		missing[uid] = 0
		if olderContainerStillExists(pod, componentName) {
			return false
		}
	}
	return len(known) > 0
}

func olderContainerStillExists(pod localPod, componentName string) bool {
	for i := range pod.containers {
		container := pod.containers[i]
		if container.name != componentName {
			continue
		}
		// A crashed container may be Terminated briefly before kubelet restarts
		// it. Only use Terminated as a handoff signal once Pod deletion proves
		// that Kubernetes intends to stop the old generation.
		return pod.deletionTimestamp == nil || !container.terminated
	}
	declared := false
	for _, name := range pod.declaredContainers {
		declared = declared || name == componentName
	}
	if !declared {
		return false
	}
	// A missing status is not positive evidence that the old runtime process
	// exited. Wait for either a Terminated state or Pod removal after deletion
	// was observed.
	return true
}

func daemonSetOwnerUID(pod localPod) (string, error) {
	var ownerUID string
	for _, owner := range pod.owners {
		if owner.kind != "DaemonSet" || !owner.controller {
			continue
		}
		if owner.uid == "" || ownerUID != "" {
			return "", fmt.Errorf("Pod %s/%s has an invalid DaemonSet controller", pod.namespace, pod.name)
		}
		ownerUID = owner.uid
	}
	if ownerUID == "" {
		return "", fmt.Errorf("Pod %s/%s is not controlled by a DaemonSet", pod.namespace, pod.name)
	}
	return ownerUID, nil
}

func (c *component) Close() error {
	return nil
}
