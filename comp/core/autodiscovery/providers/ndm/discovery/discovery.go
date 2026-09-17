// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package discovery turns the "discovery" key of the NDM Remote Configuration
// document into the ranges the discovery component sweeps.
package discovery

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	ndmdiscovery "github.com/DataDog/datadog-agent/comp/ndmdiscovery/def"
)

// Key is the document key this handler owns.
const Key = "discovery"

// Handler hands the discovery component the complete set of ranges every NDM
// document asks it to sweep.
type Handler struct {
	disco ndmdiscovery.Component
	log   log.Component
}

// NewHandler builds the discovery document-key handler.
func NewHandler(disco ndmdiscovery.Component, logComp log.Component) *Handler {
	return &Handler{disco: disco, log: logComp}
}

// Key returns the document key this handler owns.
func (h *Handler) Key() string { return Key }

// Snapshot makes the ranges carried by docs the complete set of swept ranges.
func (h *Handler) Snapshot(docs map[string]json.RawMessage) map[string]error {
	errs := make(map[string]error, len(docs))
	pathByID := make(map[string]string)
	var ranges []ndmdiscovery.Range

	for _, path := range sortedPaths(docs) {
		decoded, err := decodeRanges(docs[path])
		if err != nil {
			errs[path] = err
			continue
		}

		for _, r := range decoded {
			if owner, claimed := pathByID[r.ID]; claimed {
				errs[path] = fmt.Errorf("the range id %q is already declared by %s", r.ID, owner)
				errs[owner] = fmt.Errorf("the range id %q is also declared by %s", r.ID, path)
				continue
			}
			pathByID[r.ID] = path
			ranges = append(ranges, r)
		}
	}

	for id, err := range h.disco.Schedule(ranges) {
		path, known := pathByID[id]
		if !known {
			h.log.Warnf("ndm: the discovery component rejected the unknown range %s: %v", id, err)
			continue
		}
		errs[path] = appendError(errs[path], fmt.Sprintf("range %s: %s", id, err.Error()))
	}

	return errs
}

// keyConfig is the wire shape of the discovery key.
type keyConfig struct {
	Ranges []rangePayload `json:"ranges"`
}

// snmpOptionsPayload mirrors the snake_case Remote Configuration payload.
type snmpOptionsPayload struct {
	Port      int  `json:"port"`
	TimeoutMs int  `json:"timeout_ms"`
	Retries   *int `json:"retries"`
}

// pingOptionsPayload mirrors the snake_case Remote Configuration payload.
type pingOptionsPayload struct {
	Count      int `json:"count"`
	IntervalMs int `json:"interval_ms"`
	TimeoutMs  int `json:"timeout_ms"`
}

// rangePayload is the wire shape of one range to sweep.
type rangePayload struct {
	AutodiscoveryID    string              `json:"autodiscovery_id"`
	Namespace          string              `json:"namespace"`
	CIDR               string              `json:"cidr"`
	CredentialIDs      []string            `json:"credential_ids"`
	IntervalSec        int                 `json:"interval_sec"`
	IgnoredIPAddresses []string            `json:"ignored_ip_addresses"`
	Tags               []string            `json:"tags"`
	SNMPOptions        *snmpOptionsPayload `json:"snmp_options"`
	PingOptions        *pingOptionsPayload `json:"ping_options"`
}

// decodeRanges turns one path's discovery key into component ranges. The
// component validates them, so this only rejects what it cannot address.
func decodeRanges(raw json.RawMessage) ([]ndmdiscovery.Range, error) {
	var doc keyConfig
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, fmt.Errorf("the discovery key is not an object: %w", err)
	}

	ranges := make([]ndmdiscovery.Range, 0, len(doc.Ranges))
	for _, payload := range doc.Ranges {
		if payload.AutodiscoveryID == "" {
			return nil, errors.New("a range has no autodiscovery_id")
		}
		ranges = append(ranges, ndmdiscovery.Range{
			ID:                 payload.AutodiscoveryID,
			Namespace:          payload.Namespace,
			CIDR:               payload.CIDR,
			CredentialIDs:      payload.CredentialIDs,
			IntervalSec:        payload.IntervalSec,
			IgnoredIPAddresses: payload.IgnoredIPAddresses,
			Tags:               payload.Tags,
			SNMPOptions:        snmpOptions(payload.SNMPOptions),
			PingOptions:        pingOptions(payload.PingOptions),
		})
	}
	return ranges, nil
}

func snmpOptions(p *snmpOptionsPayload) *ndmdiscovery.SNMPOptions {
	if p == nil {
		return nil
	}
	return &ndmdiscovery.SNMPOptions{Port: p.Port, TimeoutMs: p.TimeoutMs, Retries: p.Retries}
}

func pingOptions(p *pingOptionsPayload) *ndmdiscovery.PingOptions {
	if p == nil {
		return nil
	}
	return &ndmdiscovery.PingOptions{Count: p.Count, IntervalMs: p.IntervalMs, TimeoutMs: p.TimeoutMs}
}

// sortedPaths orders the paths so a range id claimed by two of them is always
// reported against the same one.
func sortedPaths(docs map[string]json.RawMessage) []string {
	paths := make([]string, 0, len(docs))
	for path := range docs {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	return paths
}

func appendError(existing error, message string) error {
	if existing == nil {
		return errors.New(message)
	}
	return errors.New(strings.Join([]string{existing.Error(), message}, "; "))
}
