// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build kubeapiserver

// Package hardening enforces workload hardening requests. The leader marks a
// Deployment's pod template with a request id, and the hardening admission
// webhook narrows the securityContext of the targeted container in each new pod.
package hardening

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	// EnabledLabel on a pod template routes its pods to the hardening webhook.
	EnabledLabel = "hardening.datadoghq.com/enabled"
	// RequestsAnnotation on a pod template lists the request ids the webhook may apply.
	RequestsAnnotation = "hardening.datadoghq.com/requests"
	// AppliedAnnotation on a pod records what the webhook did for each request id.
	AppliedAnnotation = "hardening.datadoghq.com/applied"
	// AppliedUnknown is the AppliedAnnotation value for a request the replica
	// serving the webhook has not loaded, as opposed to an expired one.
	AppliedUnknown = "skipped: unknown request"
)

// Action is what a request asks for.
type Action string

const (
	// ActionEnforce applies the control in every listed cluster.
	ActionEnforce Action = "enforce"
	// ActionRevert removes the control.
	ActionRevert Action = "revert"
	// ActionValidate runs the pre-flight checks and dry runs, and writes nothing.
	ActionValidate Action = "validate"
)

// Control is the hardening control a request applies.
type Control string

const (
	// ControlCapabilities drops every capability except Parameters.CapabilitiesAdd.
	ControlCapabilities Control = "capabilities"
	// ControlNonRoot sets runAsNonRoot and disallows privilege escalation.
	ControlNonRoot Control = "non_root"
	// ControlReadOnlyRootFS sets readOnlyRootFilesystem.
	ControlReadOnlyRootFS Control = "read_only_root_fs"
)

// defaultConfirmFor is the confirmation window of a request that sets none.
const defaultConfirmFor = 7 * 24 * time.Hour

// Workload is the workload definition a request applies to. It names no
// cluster and no image, so the control carries over to every later release.
type Workload struct {
	Kind      string `json:"kind"`
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
	Container string `json:"container"`
}

// Cluster is one cluster a request covers, by the ID of its datadog-cluster-id ConfigMap.
type Cluster struct {
	ID      string    `json:"id"`
	AddedAt time.Time `json:"added_at"`
}

// Parameters holds control-specific parameters.
type Parameters struct {
	// CapabilitiesAdd is the capability list the container keeps, for ControlCapabilities.
	CapabilitiesAdd []string `json:"capabilities_add,omitempty"`
}

// Request is one hardening request, as delivered by Remote Config or the requests file.
type Request struct {
	ID         string     `json:"id"`
	Action     Action     `json:"action"`
	Control    Control    `json:"control"`
	Workload   Workload   `json:"workload"`
	Parameters Parameters `json:"parameters"`
	Clusters   []Cluster  `json:"clusters"`
	// ConfirmFor is the confirmation window, a Go duration counted from the
	// rollout in each cluster.
	ConfirmFor string     `json:"confirm_for,omitempty"`
	ExpiresAt  *time.Time `json:"expires_at,omitempty"`

	// addedAt is when this cluster was added to Clusters, zero when it is not listed.
	addedAt    time.Time
	confirmFor time.Duration
	// hash identifies what the request means for this cluster, so the
	// controller can tell a changed request from one it already finished.
	hash string
}

// Listed reports whether the request covers this cluster.
func (r *Request) Listed() bool {
	return !r.addedAt.IsZero()
}

func (r *Request) inForce(now time.Time) bool {
	return r.ExpiresAt == nil || now.Before(*r.ExpiresAt)
}

// Active reports whether the request is to be enforced in this cluster.
func (r *Request) Active(now time.Time) bool {
	return r.Action == ActionEnforce && r.Listed() && r.inForce(now)
}

var idPattern = regexp.MustCompile(`^[A-Za-z0-9._-]{1,128}$`)

// knownCapabilities is the set of Linux capability names (without the CAP_
// prefix) that parseRequest accepts. ALL is deliberately not a member.
var knownCapabilities = map[string]bool{
	"CHOWN": true, "DAC_OVERRIDE": true, "DAC_READ_SEARCH": true, "FOWNER": true,
	"FSETID": true, "KILL": true, "SETGID": true, "SETUID": true, "SETPCAP": true,
	"LINUX_IMMUTABLE": true, "NET_BIND_SERVICE": true, "NET_BROADCAST": true,
	"NET_ADMIN": true, "NET_RAW": true, "IPC_LOCK": true, "IPC_OWNER": true,
	"SYS_MODULE": true, "SYS_RAWIO": true, "SYS_CHROOT": true, "SYS_PTRACE": true,
	"SYS_PACCT": true, "SYS_ADMIN": true, "SYS_BOOT": true, "SYS_NICE": true,
	"SYS_RESOURCE": true, "SYS_TIME": true, "SYS_TTY_CONFIG": true, "MKNOD": true,
	"LEASE": true, "AUDIT_WRITE": true, "AUDIT_CONTROL": true, "SETFCAP": true,
	"MAC_OVERRIDE": true, "MAC_ADMIN": true, "SYSLOG": true, "WAKE_ALARM": true,
	"BLOCK_SUSPEND": true, "AUDIT_READ": true, "PERFMON": true, "BPF": true,
	"CHECKPOINT_RESTORE": true,
}

// parseRequest decodes and validates one request. Unknown fields are rejected so
// that a payload this version does not understand is never half-applied. A
// request that does not list clusterID is kept, so the leader can remove it
// from a cluster that left the list.
func parseRequest(raw []byte, clusterID string) (*Request, error) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	var r Request
	if err := dec.Decode(&r); err != nil {
		return nil, fmt.Errorf("invalid request: %w", err)
	}
	if !idPattern.MatchString(r.ID) {
		return nil, fmt.Errorf("invalid request id %q", r.ID)
	}
	if r.Action != ActionEnforce && r.Action != ActionRevert && r.Action != ActionValidate {
		return nil, fmt.Errorf("request %s: unknown action %q", r.ID, r.Action)
	}
	if err := parseParameters(&r); err != nil {
		return nil, err
	}
	w := r.Workload
	if w.Kind != "Deployment" {
		return nil, fmt.Errorf("request %s: unsupported kind %q", r.ID, w.Kind)
	}
	if w.Namespace == "" || w.Name == "" || w.Container == "" {
		return nil, fmt.Errorf("request %s: incomplete workload", r.ID)
	}
	for _, c := range r.Clusters {
		if c.ID == "" || c.AddedAt.IsZero() {
			return nil, fmt.Errorf("request %s: a cluster has no id or added_at", r.ID)
		}
		if c.ID == clusterID {
			r.addedAt = c.AddedAt
		}
	}
	r.confirmFor = defaultConfirmFor
	if r.ConfirmFor != "" {
		d, err := time.ParseDuration(r.ConfirmFor)
		if err != nil || d <= 0 {
			return nil, fmt.Errorf("request %s: invalid confirm_for %q", r.ID, r.ConfirmFor)
		}
		r.confirmFor = d
	}
	r.hash = clusterHash(r)
	return &r, nil
}

func parseParameters(r *Request) error {
	switch r.Control {
	case ControlCapabilities:
		for i, c := range r.Parameters.CapabilitiesAdd {
			c = normalizeCapability(c)
			if !knownCapabilities[c] {
				return fmt.Errorf("request %s: invalid capability %q", r.ID, r.Parameters.CapabilitiesAdd[i])
			}
			r.Parameters.CapabilitiesAdd[i] = c
		}
	case ControlNonRoot, ControlReadOnlyRootFS:
		if len(r.Parameters.CapabilitiesAdd) > 0 {
			return fmt.Errorf("request %s: capabilities_add is only valid for %s", r.ID, ControlCapabilities)
		}
	default:
		return fmt.Errorf("request %s: unknown control %q", r.ID, r.Control)
	}
	return nil
}

// clusterHash hashes the request without the other clusters' entries, so that
// adding or dropping another cluster does not make this one reconcile it again.
func clusterHash(r Request) string {
	r.Clusters = nil
	raw, _ := json.Marshal(struct {
		Request Request
		AddedAt time.Time
	}{r, r.addedAt})
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

// normalizeCapability returns the Kubernetes spelling of a capability name.
func normalizeCapability(c string) string {
	return strings.TrimPrefix(strings.ToUpper(strings.TrimSpace(c)), "CAP_")
}

// ParseIDs splits a RequestsAnnotation value.
func ParseIDs(v string) []string {
	var ids []string
	for _, id := range strings.Split(v, ",") {
		if id = strings.TrimSpace(id); id != "" {
			ids = append(ids, id)
		}
	}
	return ids
}

// Store holds the hardening requests known to this Cluster Agent replica. Every
// replica keeps one, because the API server may send an admission request to any of them.
type Store struct {
	clusterID string

	mu   sync.RWMutex
	rc   map[string]*Request // by request id
	file map[string]*Request // by request id
	// defaults is what the cluster's runtimes grant a container that declares
	// no capabilities; the leader and the webhook must agree on it.
	defaults Defaults

	changes chan struct{}
}

// NewStore returns an empty Store for the cluster whose ID is clusterID.
func NewStore(clusterID string) *Store {
	return &Store{
		clusterID: clusterID,
		rc:        map[string]*Request{},
		file:      map[string]*Request{},
		defaults:  fallbackDefaults,
		changes:   make(chan struct{}, 1),
	}
}

// Defaults returns what the cluster's runtimes grant a container that declares
// no capabilities, as of the last node listing.
func (s *Store) Defaults() Defaults {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.defaults
}

// setDefaults reports whether d differs from the previous Defaults.
func (s *Store) setDefaults(d Defaults) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	changed := d.Exact != s.defaults.Exact || !slices.Equal(d.Capabilities, s.defaults.Capabilities)
	s.defaults = d
	return changed
}

// Get returns the request with this id. Remote Config wins over the requests file.
func (s *Store) Get(id string) (*Request, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if r, ok := s.rc[id]; ok {
		return r, true
	}
	r, ok := s.file[id]
	return r, ok
}

// List returns every request. Remote Config wins over the requests file.
func (s *Store) List() []*Request {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]*Request, 0, len(s.rc)+len(s.file))
	for _, r := range s.rc {
		out = append(out, r)
	}
	for id, r := range s.file {
		if _, ok := s.rc[id]; !ok {
			out = append(out, r)
		}
	}
	return out
}

// Changes is signalled after every update.
func (s *Store) Changes() <-chan struct{} {
	return s.changes
}

func (s *Store) set(fromRC bool, reqs map[string]*Request) {
	s.mu.Lock()
	if fromRC {
		s.rc = reqs
	} else {
		s.file = reqs
	}
	s.mu.Unlock()
	select {
	case s.changes <- struct{}{}:
	default:
	}
}

// OnRCUpdate is the Remote Config callback. Each update carries every config of
// the product, so it replaces the Remote Config requests wholesale.
func (s *Store) OnRCUpdate(updates map[string]state.RawConfig, applyStateCallback func(string, state.ApplyStatus)) {
	reqs := make(map[string]*Request, len(updates))
	for path, raw := range updates {
		r, err := parseRequest(raw.Config, s.clusterID)
		if err != nil {
			log.Warnf("hardening: ignoring remote config %s: %v", path, err)
			applyStateCallback(path, state.ApplyStatus{State: state.ApplyStateError, Error: err.Error()})
			continue
		}
		reqs[r.ID] = r
		applyStateCallback(path, state.ApplyStatus{State: state.ApplyStateAcknowledged})
	}
	s.set(true, reqs)
}

// setFileRequests replaces the requests-file requests with data, a JSON array of
// requests. Invalid entries are skipped; invalid JSON keeps the previous set.
func (s *Store) setFileRequests(data []byte) error {
	var raws []json.RawMessage
	if err := json.Unmarshal(data, &raws); err != nil {
		return err
	}
	reqs := make(map[string]*Request, len(raws))
	for _, raw := range raws {
		r, err := parseRequest(raw, s.clusterID)
		if err != nil {
			log.Warnf("hardening: ignoring entry in the requests file: %v", err)
			continue
		}
		reqs[r.ID] = r
	}
	s.set(false, reqs)
	return nil
}

// WatchFile loads path now, then again every interval when its content changes,
// until ctx is done.
// ponytail: polling, as the file is a dev/testing feed; use fsnotify if it ever matters.
func (s *Store) WatchFile(ctx context.Context, path string, interval time.Duration) {
	var last []byte
	var lastErr string
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		data, err := os.ReadFile(path)
		switch {
		case err != nil:
			if err.Error() != lastErr {
				log.Warnf("hardening: cannot read requests file: %v", err)
				lastErr = err.Error()
			}
		case !bytes.Equal(data, last):
			if err := s.setFileRequests(data); err != nil {
				log.Warnf("hardening: keeping previous requests, %s is invalid: %v", path, err)
			}
			last, lastErr = data, ""
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
