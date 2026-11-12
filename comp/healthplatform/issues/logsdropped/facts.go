// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package logsdropped

import (
	"encoding/json"
	"slices"
	"sort"
	"time"

	"github.com/DataDog/datadog-agent/comp/healthplatform/issues/internal/logsissue"
	logsmetrics "github.com/DataDog/datadog-agent/comp/logs-library/metrics"
)

// keyFacts is the one IssueReport.Context key: the check's facts, as JSON.
const keyFacts = "facts"

// Extra keys: the issue details Fleet renders, and the placeholders catalog
// templates read.
const (
	keyMainLogs    = "main_endpoint_logs_dropped"
	keyMainBytes   = "main_endpoint_bytes_dropped"
	keyCopyLogs    = "log_copies_dropped"
	keyCopyBytes   = "log_copy_bytes_dropped"
	keyProcessed   = "logs_processed"
	keySourceCount = "source_count"
	keyTopSources  = "top_sources"
	keyReasons     = "reasons"
	keyStartedAt   = "started_at"
	keyLastDropAt  = "last_drop_at"
)

const (
	// maxTopSources bounds the sources named in the issue; source_count has the rest.
	maxTopSources = 5
	// maxEndpoints bounds the hosts named per reason. Endpoints are configured by
	// hand, so more than this is unusual.
	maxEndpoints = 5
	// A hostname is at most 253 characters.
	maxHostLen = 253
)

// reasonOrder is every DropReason, likeliest and cheapest fix first. Remediation
// steps follow it.
var reasonOrder = []logsmetrics.DropReason{
	logsmetrics.DropReasonAuth,
	logsmetrics.DropReasonTooLarge,
	logsmetrics.DropReasonBufferFull,
	logsmetrics.DropReasonSendFailed,
	logsmetrics.DropReasonMalformed,
}

// sourceLoss is one (source, service) pair's loss, copies included.
type sourceLoss struct {
	Source, Service string
	Logs            int64
}

// facts is everything the issue says, computed once by the check.
type facts struct {
	// Logs the main endpoint never got, including those dropped before sending.
	MainLogs, MainBytes int64
	// Copies lost at additional, best-effort or failover endpoints.
	CopyLogs, CopyBytes int64
	// Logs the Agent processed since the incident started: the severity denominator.
	Processed int64

	SourceCount int
	TopSources  []sourceLoss

	// Reasons seen, in reasonOrder; topReason lost the most logs.
	Reasons   []logsmetrics.DropReason
	TopReason logsmetrics.DropReason
	// Hosts each reason was seen at. Absent or empty when it happened before sending.
	Endpoints map[logsmetrics.DropReason][]string

	// When the earliest ongoing incident started, and the most recent drop.
	StartedAt  time.Time
	LastDropAt time.Time
}

func summarize(drops []logsmetrics.DroppedLogsSummary, processed int64) facts {
	f := facts{Processed: processed, Endpoints: map[logsmetrics.DropReason][]string{}}

	type origin struct{ Source, Service string }
	bySource := map[origin]int64{}
	sources := map[string]struct{}{}
	type reasonLoss struct{ main, all int64 }
	byReason := map[logsmetrics.DropReason]reasonLoss{}
	hosts := map[logsmetrics.DropReason]map[string]struct{}{}

	for _, d := range drops {
		if d.Target.Main {
			f.MainLogs += d.Logs
			f.MainBytes += d.Bytes
		} else {
			f.CopyLogs += d.Logs
			f.CopyBytes += d.Bytes
		}
		bySource[origin{d.Source, d.Service}] += d.Logs
		sources[d.Source] = struct{}{}
		rl := byReason[d.Reason]
		rl.all += d.Logs
		if d.Target.Main {
			rl.main += d.Logs
		}
		byReason[d.Reason] = rl
		if d.Target.Host != "" {
			if hosts[d.Reason] == nil {
				hosts[d.Reason] = map[string]struct{}{}
			}
			hosts[d.Reason][d.Target.Host] = struct{}{}
		}
		if f.StartedAt.IsZero() || d.StartedAt.Before(f.StartedAt) {
			f.StartedAt = d.StartedAt
		}
		if d.LastDropAt.After(f.LastDropAt) {
			f.LastDropAt = d.LastDropAt
		}
	}

	f.SourceCount = len(sources)
	for o, logs := range bySource {
		f.TopSources = append(f.TopSources, sourceLoss{o.Source, o.Service, logs})
	}
	sortSources(f.TopSources)
	if len(f.TopSources) > maxTopSources {
		f.TopSources = f.TopSources[:maxTopSources]
	}

	for _, r := range reasonOrder {
		if _, ok := byReason[r]; !ok {
			continue
		}
		f.Reasons = append(f.Reasons, r)
		// Main-endpoint loss outranks copies: it is what the customer's org misses.
		if cur, top := byReason[r], byReason[f.TopReason]; f.TopReason == "" ||
			cur.main > top.main || (cur.main == top.main && cur.all > top.all) {
			f.TopReason = r
		}
		if set := hosts[r]; len(set) > 0 {
			list := make([]string, 0, len(set))
			for h := range set {
				list = append(list, h)
			}
			sort.Strings(list)
			f.Endpoints[r] = list[:min(len(list), maxEndpoints)]
		}
	}
	return f
}

func sortSources(s []sourceLoss) {
	sort.Slice(s, func(i, j int) bool {
		if s[i].Logs != s[j].Logs {
			return s[i].Logs > s[j].Logs
		}
		if s[i].Source != s[j].Source {
			return s[i].Source < s[j].Source
		}
		return s[i].Service < s[j].Service
	})
}

// context carries the facts as one JSON value: IssueReport.Context is a flat string
// map, and the check and BuildIssue run in the same Agent.
func (f facts) context() map[string]string {
	encoded, _ := json.Marshal(f) // only numbers, strings and times: cannot fail
	return map[string]string{keyFacts: string(encoded)}
}

// parseFacts reads a context back. A missing or malformed value reads as zero or
// empty: the issue still renders, with less detail. Every value is bounded and
// sanitized here because the context is the trust boundary.
func parseFacts(ctx map[string]string) facts {
	var raw facts
	_ = json.Unmarshal([]byte(ctx[keyFacts]), &raw)

	f := facts{
		MainLogs:    max(raw.MainLogs, 0),
		MainBytes:   max(raw.MainBytes, 0),
		CopyLogs:    max(raw.CopyLogs, 0),
		CopyBytes:   max(raw.CopyBytes, 0),
		Processed:   max(raw.Processed, 0),
		SourceCount: max(raw.SourceCount, 0),
		Endpoints:   map[logsmetrics.DropReason][]string{},
		StartedAt:   raw.StartedAt,
		LastDropAt:  raw.LastDropAt,
	}
	for _, s := range raw.TopSources[:min(len(raw.TopSources), maxTopSources)] {
		f.TopSources = append(f.TopSources, sourceLoss{
			Source:  logsissue.SanitizeName(s.Source),
			Service: logsissue.SanitizeName(s.Service),
			Logs:    max(s.Logs, 0),
		})
	}
	// Only known reasons survive, in reasonOrder, so remediation stays a closed set.
	for _, r := range reasonOrder {
		if !slices.Contains(raw.Reasons, r) {
			continue
		}
		f.Reasons = append(f.Reasons, r)
		for _, h := range raw.Endpoints[r][:min(len(raw.Endpoints[r]), maxEndpoints)] {
			f.Endpoints[r] = append(f.Endpoints[r], logsissue.SanitizeTo(h, maxHostLen))
		}
	}
	if slices.Contains(f.Reasons, raw.TopReason) {
		f.TopReason = raw.TopReason
	} else if len(f.Reasons) > 0 {
		f.TopReason = f.Reasons[0]
	}
	return f
}
