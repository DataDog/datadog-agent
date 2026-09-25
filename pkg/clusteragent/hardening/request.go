// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build kubeapiserver

// Package hardening applies workload hardening trials. The leader marks a
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
)

// Action is what a request asks for.
type Action string

const (
	// ActionTrial applies the control.
	ActionTrial Action = "trial"
	// ActionRevert removes a trial.
	ActionRevert Action = "revert"
)

// Control is the hardening control a request applies.
type Control string

const (
	// ControlCapabilities drops every capability except Parameters.CapabilitiesAdd.
	ControlCapabilities Control = "capabilities"
	// ControlReadOnlyRootFS sets readOnlyRootFilesystem.
	ControlReadOnlyRootFS Control = "read_only_root_fs"
	// ControlSeccompRuntimeDefault sets the RuntimeDefault seccomp profile.
	ControlSeccompRuntimeDefault Control = "seccomp_runtime_default"
)

// Target is the container a request applies to.
type Target struct {
	Cluster   string `json:"cluster"`
	Kind      string `json:"kind"`
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
	UID       string `json:"uid"`
	Container string `json:"container"`
}

// Parameters holds control-specific parameters.
type Parameters struct {
	// CapabilitiesAdd is the capability list the container keeps, for ControlCapabilities.
	CapabilitiesAdd []string `json:"capabilities_add,omitempty"`
}

// Request is one hardening trial, as delivered by Remote Config or the requests file.
type Request struct {
	ID         string     `json:"id"`
	Action     Action     `json:"action"`
	Control    Control    `json:"control"`
	Target     Target     `json:"k8s_target"`
	Parameters Parameters `json:"parameters"`
	ExpiresAt  time.Time  `json:"expires_at"`

	// hash identifies the request's content, so the controller can tell a
	// changed request from one it already rejected.
	hash string
}

// Active reports whether the request may be applied.
func (r *Request) Active(now time.Time) bool {
	return r.Action == ActionTrial && now.Before(r.ExpiresAt)
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
// that a payload this version does not understand is never half-applied.
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
	if r.Action != ActionTrial && r.Action != ActionRevert {
		return nil, fmt.Errorf("request %s: unknown action %q", r.ID, r.Action)
	}
	switch r.Control {
	case ControlCapabilities:
		for i, c := range r.Parameters.CapabilitiesAdd {
			c = normalizeCapability(c)
			if !knownCapabilities[c] {
				return nil, fmt.Errorf("request %s: invalid capability %q", r.ID, r.Parameters.CapabilitiesAdd[i])
			}
			r.Parameters.CapabilitiesAdd[i] = c
		}
	case ControlReadOnlyRootFS, ControlSeccompRuntimeDefault:
		if len(r.Parameters.CapabilitiesAdd) > 0 {
			return nil, fmt.Errorf("request %s: capabilities_add is only valid for %s", r.ID, ControlCapabilities)
		}
	default:
		return nil, fmt.Errorf("request %s: unknown control %q", r.ID, r.Control)
	}
	t := r.Target
	if t.Kind != "Deployment" {
		return nil, fmt.Errorf("request %s: unsupported kind %q", r.ID, t.Kind)
	}
	if t.Cluster == "" || t.Namespace == "" || t.Name == "" || t.UID == "" || t.Container == "" {
		return nil, fmt.Errorf("request %s: incomplete target", r.ID)
	}
	if t.Cluster != clusterID {
		return nil, fmt.Errorf("request %s: targets cluster %q, not this cluster", r.ID, t.Cluster)
	}
	if r.ExpiresAt.IsZero() {
		return nil, fmt.Errorf("request %s: missing expires_at", r.ID)
	}
	sum := sha256.Sum256(raw)
	r.hash = hex.EncodeToString(sum[:])
	return &r, nil
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

	changes chan struct{}
}

// NewStore returns an empty Store that accepts requests for clusterID only.
func NewStore(clusterID string) *Store {
	return &Store{
		clusterID: clusterID,
		rc:        map[string]*Request{},
		file:      map[string]*Request{},
		changes:   make(chan struct{}, 1),
	}
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
