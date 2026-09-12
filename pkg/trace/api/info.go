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
	"strconv"
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
	ElasticSearch        bool                         `json:"elastic_search"`
	Mongo                bool                         `json:"mongo"`
	SQLExecPlan          bool                         `json:"sql_exec_plan"`
	SQLExecPlanNormalize bool                         `json:"sql_exec_plan_normalize"`
	SQLObfuscationMode   obfuscate.ObfuscationMode    `json:"sql_obfuscation_mode"`
	TagReplaceRules      []*reducedTagReplaceRule     `json:"tag_replace_rules"`
	HTTP                 reducedHTTPConfig            `json:"http"`
	RemoveStackTraces    bool                         `json:"remove_stack_traces"`
	Redis                obfuscate.RedisConfig        `json:"redis"`
	Valkey               obfuscate.ValkeyConfig       `json:"valkey"`
	Memcached            obfuscate.MemcachedConfig    `json:"memcached"`
	CreditCards          obfuscate.CreditCardsConfig  `json:"credit_cards"`
	SQL                  reducedSQLConfig             `json:"sql"`
	Elasticsearch        reducedJSONObfuscationConfig `json:"elasticsearch"`
	OpenSearch           reducedJSONObfuscationConfig `json:"opensearch"`
	MongoDB              reducedJSONObfuscationConfig `json:"mongodb"`
}

type reducedTagReplaceRule struct {
	Name    string `json:"name"`
	Pattern string `json:"pattern"`
	Repl    string `json:"repl"`
}

type reducedHTTPConfig struct {
	RemoveQueryString bool `json:"remove_query_string"`
	RemovePathDigits  bool `json:"remove_path_digits"`
}

type reducedSQLConfig struct {
	ReplaceDigits                 bool                      `json:"replace_digits"`
	KeepSQLAlias                  bool                      `json:"keep_sql_alias"`
	DollarQuotedFunc              bool                      `json:"dollar_quoted_func"`
	KeepNull                      bool                      `json:"keep_null"`
	KeepBoolean                   bool                      `json:"keep_boolean"`
	KeepPositionalParameter       bool                      `json:"keep_positional_parameter"`
	KeepTrailingSemicolon         bool                      `json:"keep_trailing_semicolon"`
	KeepIdentifierQuotation       bool                      `json:"keep_identifier_quotation"`
	ReplaceBindParameter          bool                      `json:"replace_bind_parameter"`
	RemoveSpaceBetweenParentheses bool                      `json:"remove_space_between_parentheses"`
	KeepJSONPath                  bool                      `json:"keep_json_path"`
	ObfuscationMode               obfuscate.ObfuscationMode `json:"obfuscation_mode"`
}

type reducedJSONObfuscationConfig struct {
	Enabled  bool     `json:"enabled"`
	KeepKeys []string `json:"keep_keys"`
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
// As a side effect it initialises r.computeInfoAndHash and r.agentState so that
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
	if rules := r.conf.ReplaceTags; rules != nil {
		oconf.TagReplaceRules = make([]*reducedTagReplaceRule, len(rules))
		for i, rule := range rules {
			if rule == nil {
				continue
			}
			oconf.TagReplaceRules[i] = &reducedTagReplaceRule{
				Name:    rule.Name,
				Pattern: rule.Pattern,
				Repl:    rule.Repl,
			}
		}
	}
	if o := r.conf.Obfuscation; o != nil {
		exported := o.Export(r.conf)
		oconf.ElasticSearch = o.ES.Enabled
		oconf.Mongo = o.Mongo.Enabled
		oconf.SQLExecPlan = o.SQLExecPlan.Enabled
		oconf.SQLExecPlanNormalize = o.SQLExecPlanNormalize.Enabled
		oconf.SQLObfuscationMode = r.conf.EffectiveSQLObfuscationMode()
		oconf.HTTP = reducedHTTPConfig{
			RemoveQueryString: o.HTTP.RemoveQueryString,
			RemovePathDigits:  o.HTTP.RemovePathDigits,
		}
		oconf.RemoveStackTraces = o.RemoveStackTraces
		oconf.Redis = o.Redis
		oconf.Valkey = o.Valkey
		oconf.Memcached = o.Memcached
		oconf.CreditCards = o.CreditCards
		oconf.SQL = reducedSQLConfig{
			ReplaceDigits:                 exported.SQL.ReplaceDigits,
			KeepSQLAlias:                  exported.SQL.KeepSQLAlias,
			DollarQuotedFunc:              exported.SQL.DollarQuotedFunc,
			KeepNull:                      exported.SQL.KeepNull,
			KeepBoolean:                   exported.SQL.KeepBoolean,
			KeepPositionalParameter:       exported.SQL.KeepPositionalParameter,
			KeepTrailingSemicolon:         exported.SQL.KeepTrailingSemicolon,
			KeepIdentifierQuotation:       exported.SQL.KeepIdentifierQuotation,
			ReplaceBindParameter:          exported.SQL.ReplaceBindParameter,
			RemoveSpaceBetweenParentheses: exported.SQL.RemoveSpaceBetweenParentheses,
			KeepJSONPath:                  exported.SQL.KeepJSONPath,
			ObfuscationMode:               exported.SQL.ObfuscationMode,
		}
		oconf.Elasticsearch = reducedJSONObfuscationConfig{Enabled: o.ES.Enabled, KeepKeys: o.ES.KeepValues}
		oconf.OpenSearch = reducedJSONObfuscationConfig{Enabled: o.OpenSearch.Enabled, KeepKeys: o.OpenSearch.KeepValues}
		oconf.MongoDB = reducedJSONObfuscationConfig{Enabled: o.Mongo.Enabled, KeepKeys: o.Mongo.KeepValues}
	}

	// obfuscation_version is bumped to 2 to disable client-side stats obfuscation only when the
	// effective SQL config changes the obfuscated query produced by obfuscateStatsGroup.
	// TableNames only collects metadata which obfuscateStatsGroup discards, so it does not cause
	// a divergence between client-side and agent-side stats obfuscation.
	obfuscationVersion := obfuscate.Version
	effectiveSQLConfig := r.conf.EffectiveSQLConfig()
	effectiveSQLConfig.TableNames = false
	if effectiveSQLConfig != (obfuscate.SQLConfig{}) {
		obfuscationVersion = 2
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
		ObfuscationVersion:     obfuscationVersion,
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

	// computeInfoAndHashFn serialises the payload for a given OPM and returns
	// both the response body and its SHA-256 hash (the Datadog-Agent-State
	// header value), so the two are always derived from identical bytes.
	computeInfoAndHashFn := func(opm string) ([]byte, string) {
		p := staticPayload
		p.OrgPropMarker = opm
		body, _ := json.MarshalIndent(p, "", "\t")
		h := sha256.Sum256(body)
		return body, hex.EncodeToString(h[:])
	}

	// Hold the mutex across the entire assignment + read + store so that
	// setOrgPropMarker cannot interleave. Without this, setOrgPropMarker could
	// write the correct OPM-based agentState between our unlock and our Store,
	// and our Store would then revert it to the stale pre-OPM hash.
	r.computeInfoAndHashMu.Lock()
	r.computeInfoAndHash = computeInfoAndHashFn
	opm := r.orgPropMarker.Load()
	initialBody, initialHash := computeInfoAndHashFn(opm)
	r.cachedInfoResponse.Store(initialBody)
	r.agentState.Store(initialHash)
	r.computeInfoAndHashMu.Unlock()

	return initialHash, func(w http.ResponseWriter, req *http.Request) {
		containerID := r.containerIDProvider.GetContainerID(req.Context(), req.Header)
		if containerTags, err := r.conf.ContainerTags(containerID); err == nil {
			w.Header().Add(containerTagsHashHeader, computeContainerTagsHash(containerTags))
		}

		body := r.cachedInfoResponse.Load().([]byte)
		w.Header().Set("Content-Length", strconv.Itoa(len(body)))
		w.Write(body) //nolint:errcheck
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
