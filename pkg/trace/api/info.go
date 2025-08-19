// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package api

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
	"sort"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/obfuscate"
	"github.com/DataDog/datadog-agent/pkg/trace/stats"
)

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

// makeInfoHandler returns a new handler for handling the discovery endpoint.
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

	txt, err := json.MarshalIndent(struct {
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
	}{
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
	}, "", "\t")
	if err != nil {
		panic(fmt.Errorf("Error making /info handler: %v", err))
	}
	h := sha256.Sum256(txt)
	return fmt.Sprintf("%x", h), func(w http.ResponseWriter, req *http.Request) {
		containerID := r.containerIDProvider.GetContainerID(req.Context(), req.Header)
		if containerTags, err := r.conf.ContainerTags(containerID); err == nil {
			hash := computeContainerTagsHash(containerTags)
			w.Header().Add(containerTagsHashHeader, hash)
		}
		fmt.Fprintf(w, "%s", txt)
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
