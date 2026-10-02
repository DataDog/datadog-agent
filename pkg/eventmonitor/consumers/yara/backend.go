// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux

package yara

import (
	"encoding/hex"
	"encoding/json"

	"github.com/DataDog/datadog-agent/pkg/eventmonitor"
	"github.com/DataDog/datadog-agent/pkg/security/events"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
	"github.com/DataDog/datadog-agent/pkg/security/serializers"
	"github.com/DataDog/datadog-agent/pkg/security/utils"
)

// dispatcher dispatches a CWS custom event to the probe's custom event handlers (rate limiter →
// APIServer → backend). *probe.Probe satisfies it. When CWS is disabled no handler is registered
// and the dispatch is a silent no-op, which is acceptable: the structured log line still fires.
type dispatcher interface {
	DispatchCustomEvent(rule *rules.Rule, event *events.CustomEvent)
}

// yaraMatchSerializer describes one matched YARA rule
type yaraMatchSerializer struct {
	Rule      string   `json:"rule"`
	Namespace string   `json:"namespace,omitempty"`
	Tags      []string `json:"tags,omitempty"`
}

// yaraBlock is the YARA-specific section added next to the process-activity schema
type yaraBlock struct {
	SHA256       string                `json:"sha256"`
	Path         string                `json:"path"`
	Script       bool                  `json:"script"`
	RulesVersion string                `json:"rules_version"`
	Matches      []yaraMatchSerializer `json:"matches"`
}

// yaraMalwareEvent is the custom event sent to the CWS backend on a YARA match. It carries the
// standard CWS process-activity serializer (so the match has the full process/exec schema a
// normal CWS event would) merged at the top level with the common fields and the yara block.
//
// The serializer is a named, unexported field, not an embedded one, on purpose:
// *serializers.EventSerializer has a MarshalJSON method, which embedding would promote onto this
// struct and so hijack the whole marshaling (dropping the yara block, and panicking on a nil
// serializer). ToJSON instead marshals the two halves and merges them, with the common fields
// winning the shared keys (date, container).
type yaraMalwareEvent struct {
	events.CustomEventCommonFields
	serializer *serializers.EventSerializer
	yara       yaraBlock
}

// ToJSON implements events.EventMarshaler. It merges the process-activity schema with the common
// fields and the yara block into one top-level object.
func (e *yaraMalwareEvent) ToJSON() ([]byte, error) {
	base := struct {
		events.CustomEventCommonFields
		Yara yaraBlock `json:"yara"`
	}{CustomEventCommonFields: e.CustomEventCommonFields, Yara: e.yara}

	baseBytes, err := json.Marshal(base)
	if err != nil {
		return nil, err
	}
	if e.serializer == nil {
		return baseBytes, nil
	}
	procBytes, err := e.serializer.MarshalJSON()
	if err != nil {
		return nil, err
	}
	// merge the yara/common fields over the process-activity schema
	return mergeJSONObjects(procBytes, baseBytes)
}

// mergeJSONObjects overlays the top-level keys of over onto base, both JSON objects, and returns
// the merged object. Keys in over win.
func mergeJSONObjects(base, over []byte) ([]byte, error) {
	merged := map[string]json.RawMessage{}
	if err := json.Unmarshal(base, &merged); err != nil {
		return nil, err
	}
	overlay := map[string]json.RawMessage{}
	if err := json.Unmarshal(over, &overlay); err != nil {
		return nil, err
	}
	for k, v := range overlay {
		merged[k] = v
	}
	return json.Marshal(merged)
}

// BackendReporter reports YARA matches to the Datadog backend as CWS custom events with rule ID
// yara_malware. It is a Reporter wired behind the StructuredReporter (log) as the pipeline's
// ExtraReporter; it only emits on a match, and degrades to a no-op (no panic) when it has no
// dispatcher, no way to build an event, or no process cache entry.
type BackendReporter struct {
	dispatcher   dispatcher
	newEvent     func() *model.Event
	scrubber     *utils.Scrubber
	acc          *events.AgentContainerContext
	rulesVersion string
}

var _ Reporter = (*BackendReporter)(nil)

// NewBackendReporter builds a BackendReporter from an event monitor. It returns nil when the
// probe can't provide what the serializer needs; the caller then runs with the log reporter only.
func NewBackendReporter(evm *eventmonitor.EventMonitor, rulesVersion string) *BackendReporter {
	if evm == nil || evm.Probe == nil || evm.Probe.PlatformProbe == nil {
		return nil
	}
	return &BackendReporter{
		dispatcher:   evm.Probe,
		newEvent:     evm.Probe.PlatformProbe.NewEvent,
		scrubber:     evm.Probe.GetScrubber(),
		acc:          evm.Probe.GetAgentContainerContext(),
		rulesVersion: rulesVersion,
	}
}

// Report implements Reporter. Only scans with at least one match are reported; errors and
// no-match scans are left to the structured log reporter.
func (r *BackendReporter) Report(f ExecFile, sum [32]byte, matches []Match, err error) {
	if r == nil || r.dispatcher == nil || err != nil || len(matches) == 0 {
		return
	}
	rule, ce := r.buildEvent(f, sum, matches)
	r.dispatcher.DispatchCustomEvent(rule, ce)
}

// buildEvent builds the custom rule (carrying the yara_malware rule ID) and the lazily
// marshaled custom event for a match.
func (r *BackendReporter) buildEvent(f ExecFile, sum [32]byte, matches []Match) (*rules.Rule, *events.CustomEvent) {
	block := yaraBlock{
		SHA256:       hex.EncodeToString(sum[:]),
		Path:         f.Path,
		Script:       f.IsScript,
		RulesVersion: r.rulesVersion,
		Matches:      toMatchSerializers(matches),
	}

	// capture what the marshaler needs; it may run on a different goroutine, later
	acc := r.acc
	scrubber := r.scrubber
	newEvent := r.newEvent
	pce := f.ProcessCacheEntry

	marshalerCtor := func() events.EventMarshaler {
		evt := &yaraMalwareEvent{yara: block}
		evt.FillCustomEventCommonFields(acc)
		if pce != nil && newEvent != nil {
			// Build a minimal event bound to the probe's real field handlers, pointed at the
			// (non-pooled, GC-kept) exec cache entry. No process data is reconstructed: the
			// serializer resolves it from the live entry, exactly as a normal exec event would.
			e := newEvent()
			e.Type = uint32(model.ExecEventType)
			e.ProcessCacheEntry = pce
			e.ProcessContext = &pce.ProcessContext
			e.Exec.Process = &pce.Process
			evt.serializer = serializers.NewEventSerializer(e, nil, scrubber)
		}
		return evt
	}

	rule := events.NewCustomRule(events.YaraMalwareRuleID, events.YaraMalwareRuleDesc, nil)
	return rule, events.NewCustomEventLazy(model.CustomEventType, marshalerCtor)
}

// toMatchSerializers converts the pipeline matches into the serialized yara block entries
func toMatchSerializers(matches []Match) []yaraMatchSerializer {
	out := make([]yaraMatchSerializer, 0, len(matches))
	for _, m := range matches {
		// Match and yaraMatchSerializer have identical fields (differing only in JSON tags), so
		// a conversion carries every field and breaks at compile time if Match ever changes.
		out = append(out, yaraMatchSerializer(m))
	}
	return out
}
