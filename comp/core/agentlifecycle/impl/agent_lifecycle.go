// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package agentlifecycleimpl implements the experimental prepared/active Agent lifecycle.
package agentlifecycleimpl

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
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
	rolloutStatePathKey = "experimental.node_agent_rollout.state_path"
)

type podOwner struct {
	kind       string
	uid        string
	controller bool
}

type localPod struct {
	uid       string
	name      string
	namespace string
	createdAt time.Time
	owners    []podOwner
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
	statePath     string
	processPID    int
	processStart  string
	log           log.Component
	pods          localPodSource
	pollInterval  time.Duration

	mu         sync.Mutex
	activating bool
	closed     bool
}

var _ agentlifecycle.Component = (*component)(nil)

// NewComponent creates the experimental Agent lifecycle component.
func NewComponent(deps dependencies) (agentlifecycle.Component, error) {
	return newComponent(deps, newLocalPodSource(), runtime.GOOS, currentProcessIdentity)
}

func newComponent(deps dependencies, pods localPodSource, goos string, processIdentity func() (int, string, error)) (agentlifecycle.Component, error) {
	if !deps.Config.GetBool(rolloutEnabledKey) {
		return &component{}, nil
	}
	if err := validatePlatform(goos); err != nil {
		return nil, err
	}

	if deps.Params.ComponentName == "" {
		return nil, errors.New("experimental node Agent rollout requires a component name")
	}
	podUID := strings.TrimSpace(deps.Config.GetString(rolloutPodUIDKey))
	if podUID == "" {
		return nil, fmt.Errorf("%s must identify this Pod", rolloutPodUIDKey)
	}
	statePath, err := resolveComponentPath(deps.Config.GetString(rolloutStatePathKey), deps.Params.ComponentName, ".state", rolloutStatePathKey)
	if err != nil {
		return nil, err
	}
	if !filepath.IsAbs(statePath) {
		return nil, fmt.Errorf("%s must be an absolute path", rolloutStatePathKey)
	}
	if err := os.MkdirAll(filepath.Dir(statePath), 0o755); err != nil {
		return nil, fmt.Errorf("create Agent rollout state directory: %w", err)
	}
	if err := os.Remove(statePath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("clear stale Agent rollout state: %w", err)
	}
	processPID, processStart, err := processIdentity()
	if err != nil {
		return nil, err
	}

	return &component{
		enabled:       true,
		componentName: deps.Params.ComponentName,
		podUID:        podUID,
		statePath:     statePath,
		processPID:    processPID,
		processStart:  processStart,
		log:           deps.Log,
		pods:          pods,
		pollInterval:  siblingPollInterval,
	}, nil
}

// currentProcessIdentity returns values that an exec probe can independently
// verify through /proc. Container filesystems such as an EmptyDir survive a
// container restart, so state alone is insufficient: it must be tied to the
// exact process generation that published it.
func currentProcessIdentity() (int, string, error) {
	pid := os.Getpid()
	contents, err := os.ReadFile("/proc/self/stat")
	if err != nil {
		return 0, "", fmt.Errorf("read Agent process identity: %w", err)
	}
	// The parenthesized comm field may contain spaces or right parentheses.
	// Fields after its final ") " begin at field 3; starttime is field 22.
	end := strings.LastIndex(string(contents), ") ")
	if end < 0 {
		return 0, "", errors.New("read Agent process identity: malformed /proc/self/stat")
	}
	fields := strings.Fields(string(contents)[end+2:])
	const startTimeIndex = 22 - 3
	if len(fields) <= startTimeIndex {
		return 0, "", errors.New("read Agent process identity: incomplete /proc/self/stat")
	}
	if _, err := strconv.ParseUint(fields[startTimeIndex], 10, 64); err != nil {
		return 0, "", fmt.Errorf("read Agent process identity: invalid start time: %w", err)
	}
	return pid, fields[startTimeIndex], nil
}

func validatePlatform(goos string) error {
	if goos != "linux" {
		return fmt.Errorf("experimental node Agent rollout is Linux-only (running on %s)", goos)
	}
	return nil
}

// resolveComponentPath makes the process identity part of every state path. A
// shared datadog.yaml can use the {component} token, while the Operator can
// continue supplying an already-expanded process path.
func resolveComponentPath(configuredPath, componentName, suffix, configKey string) (string, error) {
	if filepath.Base(componentName) != componentName || componentName == "." || componentName == ".." {
		return "", errors.New("experimental node Agent rollout component name must be a path-safe base name")
	}

	if strings.Contains(configuredPath, "{component}") {
		return strings.ReplaceAll(configuredPath, "{component}", componentName), nil
	}

	expectedBase := componentName + suffix
	if filepath.Base(configuredPath) != expectedBase {
		return "", fmt.Errorf("%s must contain {component} or end in %q", configKey, expectedBase)
	}
	return configuredPath, nil
}

func (c *component) Wait(ctx context.Context) error {
	if !c.enabled {
		return nil
	}

	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return errors.New("experimental Agent lifecycle is already closed")
	}
	if c.activating {
		c.mu.Unlock()
		return errors.New("experimental Agent lifecycle is already activating")
	}
	c.mu.Unlock()

	ticker := time.NewTicker(c.pollInterval)
	defer ticker.Stop()
	prepared := false

	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		pods, err := c.pods.ListLocalPods(ctx)
		if err == nil {
			var siblings []localPod
			siblings, err = siblingPods(pods, c.podUID)
			if err == nil && len(siblings) == 0 {
				if ctxErr := ctx.Err(); ctxErr != nil {
					return ctxErr
				}
				return c.beginActivation()
			}
			if err == nil && !prepared {
				if err = c.markPrepared(); err == nil {
					prepared = true
				}
			}
		}
		if err != nil {
			// A failed or incomplete kubelet response must never be interpreted as
			// proof that the old Pod is gone. Remain prepared and retry.
			c.log.Warnf("%s cannot verify node-local sibling Pods; remaining inactive: %v", c.componentName, err)
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

func (c *component) markPrepared() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return errors.New("cannot prepare a closed experimental Agent lifecycle")
	}
	if err := c.writeState(agentlifecycle.StatePrepared); err != nil {
		return err
	}
	c.log.Infof("%s verified an older DaemonSet sibling and is prepared while it waits", c.componentName)
	return nil
}

func siblingPods(pods []localPod, selfUID string) ([]localPod, error) {
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

	var siblings []localPod
	for i := range pods {
		pod := pods[i]
		if pod.uid == selfUID || pod.namespace != self.namespace {
			continue
		}
		candidateOwnerUID, ownerErr := daemonSetOwnerUID(pod)
		if ownerErr == nil && candidateOwnerUID == ownerUID {
			precedes, precedesErr := podPrecedes(pod, *self)
			if precedesErr != nil {
				return nil, precedesErr
			}
			if precedes {
				siblings = append(siblings, pod)
			}
		}
	}
	return siblings, nil
}

// podPrecedes fails closed when Kubernetes' second-precision creation
// timestamps cannot establish an order. Pod UIDs and resourceVersions are not
// creation-order values and must not be used to guess which process is active.
func podPrecedes(candidate, self localPod) (bool, error) {
	if candidate.createdAt.IsZero() {
		// An incomplete kubelet record must not be interpreted as a newer Pod.
		return true, nil
	}
	if candidate.createdAt.Before(self.createdAt) {
		return true, nil
	}
	if candidate.createdAt.After(self.createdAt) {
		return false, nil
	}
	return false, fmt.Errorf("cannot order same-timestamp Pods %s/%s and %s/%s", candidate.namespace, candidate.name, self.namespace, self.name)
}

func daemonSetOwnerUID(pod localPod) (string, error) {
	var ownerUID string
	for _, owner := range pod.owners {
		if owner.kind != "DaemonSet" || !owner.controller {
			continue
		}
		if owner.uid == "" {
			return "", fmt.Errorf("Pod %s/%s has a DaemonSet controller with an empty UID", pod.namespace, pod.name)
		}
		if ownerUID != "" {
			return "", fmt.Errorf("Pod %s/%s has multiple DaemonSet controllers", pod.namespace, pod.name)
		}
		ownerUID = owner.uid
	}
	if ownerUID == "" {
		return "", fmt.Errorf("Pod %s/%s is not controlled by a DaemonSet", pod.namespace, pod.name)
	}
	return ownerUID, nil
}

func (c *component) beginActivation() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return errors.New("cannot activate a closed experimental Agent lifecycle")
	}
	if c.activating {
		return errors.New("experimental Agent lifecycle is already activating")
	}
	if err := c.writeState(agentlifecycle.StateActivating); err != nil {
		return err
	}
	c.activating = true
	c.log.Infof("%s has no older DaemonSet sibling on the node and is activating", c.componentName)
	return nil
}

func (c *component) MarkActive() error {
	if !c.enabled {
		return nil
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return errors.New("cannot mark a closed experimental Agent lifecycle active")
	}
	if !c.activating {
		return errors.New("cannot mark the experimental Agent lifecycle active before the sibling check")
	}
	if err := c.writeState(agentlifecycle.StateActive); err != nil {
		return err
	}
	c.log.Infof("%s is active", c.componentName)
	return nil
}

func (c *component) Close() error {
	if !c.enabled {
		return nil
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return nil
	}
	c.closed = true
	return c.writeState(agentlifecycle.StateStopped)
}

func (c *component) writeState(state string) error {
	tmp, err := os.CreateTemp(filepath.Dir(c.statePath), ".agent-rollout-state-")
	if err != nil {
		return fmt.Errorf("create temporary Agent rollout state: %w", err)
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)

	if _, err := fmt.Fprintln(tmp, state, c.processPID, c.processStart); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write Agent rollout state: %w", err)
	}
	if err := tmp.Chmod(0o644); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("set Agent rollout state permissions: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close Agent rollout state: %w", err)
	}
	if err := os.Rename(tmpPath, c.statePath); err != nil {
		return fmt.Errorf("publish Agent rollout state: %w", err)
	}
	return nil
}
