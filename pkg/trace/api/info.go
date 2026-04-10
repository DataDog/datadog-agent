// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package api

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
	"sort"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/obfuscate"
	"github.com/DataDog/datadog-agent/pkg/trace/stats"
)

// infoPayload is the JSON structure served by the /info endpoint.
type infoPayload struct {
	Version                string        `json:"version"`
	GitCommit              string        `json:"git_commit"`
	Endpoints              []string      `json:"endpoints"`
	FeatureFlags           []string      `json:"feature_flags,omitempty"`
	ClientDropP0s          bool          `json:"client_drop_p0s"`
	SpanMetaStructs        bool          `json:"span_meta_structs"`
	LongRunningSpans       bool          `json:"long_running_spans"`
	SpanEvents             bool          `json:"span_events"`
	EvpProxyAllowedHeaders []string      `json:"evp_proxy_allowed_headers"`
	Config                 reducedConfig `json:"config"`
	PeerTags               []string      `json:"peer_tags"`
	SpanKindsStatsComputed []string      `json:"span_kinds_stats_computed"`
	ObfuscationVersion     int           `json:"obfuscation_version"`
	FilterTags             *filterTags   `json:"filter_tags,omitempty"`
	FilterTagsRegex        *filterTags   `json:"filter_tags_regex,omitempty"`
	IgnoreResources        []string      `json:"ignore_resources,omitempty"`
	OrgPropMarker          string        `json:"org_prop_marker,omitempty"`
}

const (
	containerTagsHashHeader = "Datadog-Container-Tags-Hash"
)

// serviceOriginTags is a set of tags that can be used in the backend to uniquely identify a service.
var serviceOriginTags = map[string]struct{}{
	"kube_job":            {},
	"kube_replica_set":    {},
	"kube_container_name": {},
	"kube_namespace":      {},
	"kube_app_name":       {},
	"kube_app_managed_by": {},
	"service":             {},
	"short_image":         {},
	"kube_cluster_name":   {},
}

type reducedObfuscationConfig struct {
	ElasticSearch        bool                      `json:"elastic_search"`
	Mongo                bool                      `json:"mongo"`
	SQLExecPlan          bool                      `json:"sql_exec_plan"`
	SQLExecPlanNormalize bool                      `json:"sql_exec_plan_normalize"`
	HTTP                 obfuscate.HTTPConfig      `json:"http"`
	RemoveStackTraces    bool                      `json:"remove_stack_traces"`
	Redis                obfuscate.RedisConfig     `json:"redis"`
	Valkey               obfuscate.ValkeyConfig    `json:"valkey"`
	Memcached            obfuscate.MemcachedConfig `json:"memcached"`
}

type reducedConfig struct {
	DefaultEnv             string                        `json:"default_env"`
	TargetTPS              float64                       `json:"target_tps"`
	MaxEPS                 float64                       `json:"max_eps"`
	ReceiverPort           int                           `json:"receiver_port"`
	ReceiverSocket         string                        `json:"receiver_socket"`
	ConnectionLimit        int                           `json:"connection_limit"`
	ReceiverTimeout        int                           `json:"receiver_timeout"`
	MaxRequestBytes        int64                         `json:"max_request_bytes"`
	StatsdPort             int                           `json:"statsd_port"`
	MaxMemory              float64                       `json:"max_memory"`
	MaxCPU                 float64                       `json:"max_cpu"`
	AnalyzedSpansByService map[string]map[string]float64 `json:"analyzed_spans_by_service"`
	Obfuscation            reducedObfuscationConfig      `json:"obfuscation"`
}

// filterTags contains trace sampling rules
type filterTags struct {
	Require []string `json:"require,omitempty"`
	Reject  []string `json:"reject,omitempty"`
}

// makeInfoHandler returns a new handler for handling the discovery endpoint.
// As a side effect it initialises r.computeStateHash and r.agentState so that
// the Datadog-Agent-State header reflects the current /info payload (including
// any already-fetched Org Propagation Marker).
func (r *HTTPReceiver) makeInfoHandler() (hash string, handler http.HandlerFunc) {
	var all []string
	for _, e := range endpoints {
		if e.IsEnabled != nil && !e.IsEnabled(r.conf) {
			continue
		}
		if !e.Hidden {
			all = append(all, e.Pattern)
		}
	}
	var oconf reducedObfuscationConfig
	if o := r.conf.Obfuscation; o != nil {
		oconf.ElasticSearch = o.ES.Enabled
		oconf.Mongo = o.Mongo.Enabled
		oconf.SQLExecPlan = o.SQLExecPlan.Enabled
		oconf.SQLExecPlanNormalize = o.SQLExecPlanNormalize.Enabled
		oconf.HTTP = o.HTTP
		oconf.RemoveStackTraces = o.RemoveStackTraces
		oconf.Redis = o.Redis
		oconf.Valkey = o.Valkey
		oconf.Memcached = o.Memcached
	}

	// We check that endpoints contains stats, even though we know this version of the
	// agent supports it. It's conceivable that the stats endpoint could be disabled at some point
	// so this is defensive against that case.
	canDropP0 := !r.conf.ProbabilisticSamplerEnabled && slices.Contains(all, "/v0.6/stats")

	var spanKindsStatsComputed []string
	if r.conf.ComputeStatsBySpanKind {
		for k := range stats.KindsComputed {
			spanKindsStatsComputed = append(spanKindsStatsComputed, k)
		}
	}

	filtertags := &filterTags{
		Require: make([]string, len(r.conf.RequireTags)),
		Reject:  make([]string, len(r.conf.RejectTags)),
	}
	for i, tag := range r.conf.RequireTags {
		if tag.V != "" {
			filtertags.Require[i] = fmt.Sprintf("%s:%s", tag.K, tag.V)
		} else {
			filtertags.Require[i] = tag.K
		}
	}
	for i, tag := range r.conf.RejectTags {
		if tag.V != "" {
			filtertags.Reject[i] = fmt.Sprintf("%s:%s", tag.K, tag.V)
		} else {
			filtertags.Reject[i] = tag.K
		}
	}

	filtertagsregex := &filterTags{
		Require: make([]string, len(r.conf.RequireTagsRegex)),
		Reject:  make([]string, len(r.conf.RejectTagsRegex)),
	}
	for i, tag := range r.conf.RequireTagsRegex {
		if tag.V != nil {
			filtertagsregex.Require[i] = fmt.Sprintf("%s:%s", tag.K, tag.V.String())
		} else {
			filtertagsregex.Require[i] = tag.K
		}
	}
	for i, tag := range r.conf.RejectTagsRegex {
		if tag.V != nil {
			filtertagsregex.Reject[i] = fmt.Sprintf("%s:%s", tag.K, tag.V.String())
		} else {
			filtertagsregex.Reject[i] = tag.K
		}
	}

	var ignoreResources []string
	if patterns, ok := r.conf.Ignore["resource"]; ok {
		ignoreResources = patterns
	}

	// staticPayload holds every field that does not change after startup.
	staticPayload := infoPayload{
		Version:                r.conf.AgentVersion,
		GitCommit:              r.conf.GitCommit,
		Endpoints:              all,
		FeatureFlags:           r.conf.AllFeatures(),
		ClientDropP0s:          canDropP0,
		SpanMetaStructs:        true,
		LongRunningSpans:       true,
		SpanEvents:             true,
		EvpProxyAllowedHeaders: EvpProxyAllowedHeaders,
		SpanKindsStatsComputed: spanKindsStatsComputed,
		ObfuscationVersion:     obfuscate.Version,
		FilterTags:             filtertags,
		FilterTagsRegex:        filtertagsregex,
		IgnoreResources:        ignoreResources,
		Config: reducedConfig{
			DefaultEnv:             r.conf.DefaultEnv,
			TargetTPS:              r.conf.TargetTPS,
			MaxEPS:                 r.conf.MaxEPS,
			ReceiverPort:           r.conf.ReceiverPort,
			ReceiverSocket:         r.conf.ReceiverSocket,
			ConnectionLimit:        r.conf.ConnectionLimit,
			ReceiverTimeout:        r.conf.ReceiverTimeout,
			MaxRequestBytes:        r.conf.MaxRequestBytes,
			StatsdPort:             r.conf.StatsdPort,
			MaxMemory:              r.conf.MaxMemory,
			MaxCPU:                 r.conf.MaxCPU,
			AnalyzedSpansByService: r.conf.AnalyzedSpansByService,
			Obfuscation:            oconf,
		},
		PeerTags: r.conf.ConfiguredPeerTags(),
	}

	// computeStateHash produces the Datadog-Agent-State hash for a given OPM.
	// Stored on the receiver so setOrgPropMarker can reuse it without re-parsing
	// the configuration.
	computeStateHashFn := func(opm string) string {
		p := staticPayload
		p.OrgPropMarker = opm
		b, _ := json.Marshal(p)
		h := sha256.Sum256(b)
		return hex.EncodeToString(h[:])
	}

	r.computeStateHashMu.Lock()
	r.computeStateHash = computeStateHashFn
	r.computeStateHashMu.Unlock()

	// Initialise agentState using the OPM that may have already been stored by
	// startOPMFetch (possible when the fetch completes before buildMux runs).
	initialHash := computeStateHashFn(r.OrgPropMarker())
	r.agentState.Store(initialHash)

	return initialHash, func(w http.ResponseWriter, req *http.Request) {
		opm := r.OrgPropMarker()
		payload := staticPayload
		payload.OrgPropMarker = opm

		containerID := r.containerIDProvider.GetContainerID(req.Context(), req.Header)
		if containerTags, err := r.conf.ContainerTags(containerID); err == nil {
			w.Header().Add(containerTagsHashHeader, computeContainerTagsHash(containerTags))
		}

		txt, err := json.MarshalIndent(payload, "", "\t")
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		w.Write(txt) //nolint:errcheck
	}
}

func computeContainerTagsHash(tags []string) string {
	filtered := make([]string, 0, len(tags))
	for _, tag := range tags {
		if strings.Contains(tag, ":") {
			kv := strings.SplitN(tag, ":", 2)
			if _, ok := serviceOriginTags[kv[0]]; ok {
				filtered = append(filtered, tag)
			}
		}
	}
	sort.Strings(filtered)
	return fmt.Sprintf("%x", sha256.Sum256([]byte(strings.Join(filtered, ","))))
}
