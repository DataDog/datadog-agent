// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build kubeapiserver

// Package hardening enforces workload hardening policies. A policy turns on
// one control for every Deployment of the clusters it lists, minus exceptions.
// The leader marks each Deployment's pod template with the policy, and the
// hardening admission webhook sets the control on the marked containers of
// each new pod.
package hardening

import (
	"bytes"
	"encoding/json"
	"fmt"
	"regexp"
	"slices"
	"time"

	"github.com/distribution/reference"
)

// Control is the hardening control a policy enforces.
type Control string

const (
	// ControlCapabilities drops every capability the manifest does not add.
	ControlCapabilities Control = "capabilities"
	// ControlNonRoot sets runAsNonRoot.
	ControlNonRoot Control = "non_root"
	// ControlNoPrivilegeEscalation sets allowPrivilegeEscalation to false.
	ControlNoPrivilegeEscalation Control = "no_privilege_escalation"
)

func (c Control) valid() bool {
	return c == ControlCapabilities || c == ControlNonRoot || c == ControlNoPrivilegeEscalation
}

// State is what a policy asks of the clusters it lists.
type State string

const (
	// StateEnforce hardens the listed clusters' Deployments.
	StateEnforce State = "enforce"
	// StateStopped rolls back what the policy hardened.
	StateStopped State = "stopped"
)

const (
	typePolicy     = "policy"
	typeExceptions = "exceptions"
)

// Cluster is one cluster a policy covers, by the ID of its
// datadog-cluster-id ConfigMap. AddedAt is when the team last added it:
// re-adding a paused cluster resumes it, since only reverts and failures
// after AddedAt count towards a pause.
type Cluster struct {
	ID      string    `json:"id"`
	AddedAt time.Time `json:"added_at"`
}

// Limits paces a leader's rollouts.
type Limits struct {
	// FirstBatch is how many Deployments the leader hardens before it waits
	// for their rollouts, and where it goes back to after a revert.
	FirstBatch int `json:"first_batch"`
	// HourlyCap bounds how many Deployments the leader marks in an hour.
	HourlyCap int `json:"hourly_cap"`
	// PauseThreshold is how many reverts pause the cluster.
	PauseThreshold int `json:"pause_threshold"`
}

// Policy is the hardening-policy-<control> config.
type Policy struct {
	Type     string    `json:"type"`
	ID       string    `json:"id"`
	Control  Control   `json:"control"`
	State    State     `json:"state"`
	Clusters []Cluster `json:"clusters"`
	Limits   Limits    `json:"limits"`
	// MinExceptionsGeneration is the oldest exceptions generation the leader
	// may start rollouts with.
	MinExceptionsGeneration int64 `json:"min_exceptions_generation"`

	// cluster is this cluster's entry in Clusters, nil when it is not listed.
	cluster *Cluster
}

// Listed reports whether the policy covers this cluster.
func (p *Policy) Listed() bool {
	return p.cluster != nil
}

// Active reports whether the policy is to be enforced in this cluster.
func (p *Policy) Active() bool {
	return p.State == StateEnforce && p.Listed()
}

// WorkloadRef names a workload definition. An empty Container covers every
// container of the workload.
type WorkloadRef struct {
	Kind      string `json:"kind"`
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
	Container string `json:"container,omitempty"`
}

func (w WorkloadRef) matches(namespace, name, container string) bool {
	return w.Kind == "Deployment" && w.Namespace == namespace && w.Name == name &&
		(w.Container == "" || w.Container == container)
}

// WorkloadException is a workload definition a policy skips, and why.
type WorkloadException struct {
	WorkloadRef
	// Reason is evidence, user or rollback; only the backend reads it.
	Reason string `json:"reason"`
}

// Matcher selects containers by namespace, image repository or workload.
type Matcher struct {
	Namespaces        []string            `json:"namespaces,omitempty"`
	ImageRepositories []string            `json:"image_repositories,omitempty"`
	Workloads         []WorkloadException `json:"workloads,omitempty"`
}

func (m *Matcher) empty() bool {
	return len(m.Namespaces) == 0 && len(m.ImageRepositories) == 0 && len(m.Workloads) == 0
}

func (m *Matcher) matches(namespace, name, container, image string) bool {
	return slices.Contains(m.Namespaces, namespace) ||
		slices.Contains(m.ImageRepositories, imageRepository(image)) ||
		slices.ContainsFunc(m.Workloads, func(w WorkloadException) bool { return w.matches(namespace, name, container) })
}

// Stop is an emergency stop: the leader removes the policy from what it
// matches at once, without pacing. No cluster means every cluster, and an
// empty matcher means every container.
type Stop struct {
	Matcher
	Clusters []string `json:"clusters,omitempty"`
}

func (s *Stop) matches(clusterID, namespace, name, container, image string) bool {
	if len(s.Clusters) > 0 && !slices.Contains(s.Clusters, clusterID) {
		return false
	}
	return s.Matcher.empty() || s.Matcher.matches(namespace, name, container, image)
}

// Exceptions is one shard of the hardening-exceptions-<control> config.
type Exceptions struct {
	Type       string  `json:"type"`
	Control    Control `json:"control"`
	Generation int64   `json:"generation"`
	Shard      int     `json:"shard"`
	Shards     int     `json:"shards"`
	Matcher
	EmergencyStops []Stop `json:"emergency_stops,omitempty"`
}

var policyIDPattern = regexp.MustCompile(`^[A-Za-z0-9._-]{1,128}$`)

// parseConfig decodes one policy or exceptions config. Unknown fields are
// rejected so that a payload this version does not understand is never
// half-applied.
func parseConfig(raw []byte, clusterID string) (*Policy, *Exceptions, error) {
	var head struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(raw, &head); err != nil {
		return nil, nil, fmt.Errorf("invalid config: %w", err)
	}
	switch head.Type {
	case typePolicy:
		p, err := parsePolicy(raw, clusterID)
		return p, nil, err
	case typeExceptions:
		e, err := parseExceptions(raw)
		return nil, e, err
	}
	return nil, nil, fmt.Errorf("unknown config type %q", head.Type)
}

func decodeStrict(raw []byte, v any) error {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	return dec.Decode(v)
}

func parsePolicy(raw []byte, clusterID string) (*Policy, error) {
	var p Policy
	if err := decodeStrict(raw, &p); err != nil {
		return nil, fmt.Errorf("invalid policy: %w", err)
	}
	if !policyIDPattern.MatchString(p.ID) {
		return nil, fmt.Errorf("invalid policy id %q", p.ID)
	}
	if !p.Control.valid() {
		return nil, fmt.Errorf("policy %s: unknown control %q", p.ID, p.Control)
	}
	if p.State != StateEnforce && p.State != StateStopped {
		return nil, fmt.Errorf("policy %s: unknown state %q", p.ID, p.State)
	}
	if l := p.Limits; l.FirstBatch < 1 || l.HourlyCap < l.FirstBatch || l.PauseThreshold < 1 {
		return nil, fmt.Errorf("policy %s: invalid limits %+v", p.ID, l)
	}
	for i, c := range p.Clusters {
		if c.ID == "" || c.AddedAt.IsZero() {
			return nil, fmt.Errorf("policy %s: a cluster has no id or added_at", p.ID)
		}
		if c.ID == clusterID {
			p.cluster = &p.Clusters[i]
		}
	}
	return &p, nil
}

func parseExceptions(raw []byte) (*Exceptions, error) {
	var e Exceptions
	if err := decodeStrict(raw, &e); err != nil {
		return nil, fmt.Errorf("invalid exceptions: %w", err)
	}
	if !e.Control.valid() {
		return nil, fmt.Errorf("exceptions: unknown control %q", e.Control)
	}
	if e.Generation < 1 || e.Shards < 1 || e.Shard < 0 || e.Shard >= e.Shards {
		return nil, fmt.Errorf("exceptions for %s: invalid generation %d or shard %d of %d", e.Control, e.Generation, e.Shard, e.Shards)
	}
	matchers := []*Matcher{&e.Matcher}
	for i := range e.EmergencyStops {
		matchers = append(matchers, &e.EmergencyStops[i].Matcher)
	}
	for _, m := range matchers {
		if err := normalizeMatcher(m); err != nil {
			return nil, fmt.Errorf("exceptions for %s: %w", e.Control, err)
		}
	}
	return &e, nil
}

func normalizeMatcher(m *Matcher) error {
	for _, w := range m.Workloads {
		if w.Kind == "" || w.Namespace == "" || w.Name == "" {
			return fmt.Errorf("incomplete workload %+v", w.WorkloadRef)
		}
	}
	for i, r := range m.ImageRepositories {
		repo := imageRepository(r)
		if repo == "" {
			return fmt.Errorf("invalid image repository %q", r)
		}
		m.ImageRepositories[i] = repo
	}
	return nil
}

// imageRepository returns the normalized repository of an image reference,
// such as docker.io/library/nginx for nginx:1.25, or "" if it is invalid.
func imageRepository(image string) string {
	ref, err := reference.ParseNormalizedNamed(image)
	if err != nil {
		return ""
	}
	return ref.Name()
}

// ExceptionSet is every shard of one exceptions generation.
type ExceptionSet struct {
	Generation int64
	Matcher
	EmergencyStops []Stop
}

func mergeShards(shards []*Exceptions) *ExceptionSet {
	set := &ExceptionSet{Generation: shards[0].Generation}
	for _, s := range shards {
		set.Namespaces = append(set.Namespaces, s.Namespaces...)
		set.ImageRepositories = append(set.ImageRepositories, s.ImageRepositories...)
		set.Workloads = append(set.Workloads, s.Workloads...)
		set.EmergencyStops = append(set.EmergencyStops, s.EmergencyStops...)
	}
	return set
}

// Exempts reports whether an exception covers the container.
func (s *ExceptionSet) Exempts(namespace, name, container, image string) bool {
	return s.Matcher.matches(namespace, name, container, image)
}

// Stops reports whether an emergency stop covers the container in this cluster.
func (s *ExceptionSet) Stops(clusterID, namespace, name, container, image string) bool {
	return slices.ContainsFunc(s.EmergencyStops, func(stop Stop) bool {
		return stop.matches(clusterID, namespace, name, container, image)
	})
}
