// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build clusterchecks

package providers

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"reflect"

	"github.com/google/cel-go/cel"
	"github.com/google/cel-go/ext"
	yaml "go.yaml.in/yaml/v3"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/providers/names"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/providers/types"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/telemetry"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup" //nolint:depguard
	"github.com/DataDog/datadog-agent/pkg/config/structure"
	httputils "github.com/DataDog/datadog-agent/pkg/util/http"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// httpSDTargetGroup represents a target group returned by an HTTP SD endpoint,
// following the Prometheus HTTP Service Discovery format.
type httpSDTargetGroup struct {
	Targets []string          `json:"targets"`
	Labels  map[string]string `json:"labels,omitempty"`
}

// httpSDTarget is the CEL variable type exposed to exclude_filter expressions.
// Fields use json tags so CEL resolves them as lowercase names (host, port, labels).
type httpSDTarget struct {
	Host   string            `json:"host"`
	Port   string            `json:"port"`
	Labels map[string]string `json:"labels"`
}

// httpSDCheckTemplate represents the check configuration template
// applied to each discovered target.
type httpSDCheckTemplate struct {
	Name       string                   `json:"name"`
	InitConfig map[string]interface{}   `json:"init_config"`
	Instances  []map[string]interface{} `json:"instances"`
}

// httpSDEntry represents a single Prometheus HTTP SD endpoint to poll along
// with the check template applied to each of its discovered targets.
type httpSDEntry struct {
	url           string
	client        *http.Client
	checkTemplate httpSDCheckTemplate
	filterProgram cel.Program // compiled exclude_filter CEL program; nil if no filter
}

// httpSDConfigEntry mirrors a single entry under prometheus_http_sd.configs in
// the agent's YAML configuration.
type httpSDConfigEntry struct {
	URL           string `mapstructure:"url" yaml:"url"`
	CheckTemplate string `mapstructure:"check_template" yaml:"check_template"`
	// ExcludeFilter is an optional CEL expression that, when it evaluates to true,
	// causes the target to be skipped. The expression receives a single variable
	// named "target" with fields "host" (string), "port" (string), and "labels" (map of string to string).
	ExcludeFilter string `mapstructure:"exclude_filter" yaml:"exclude_filter"`
}

// buildEntries validates each raw config entry and produces the corresponding
// httpSDEntry values that share a single HTTP client. Invalid entries are
// skipped and their errors returned alongside the valid entries so the caller
// can decide whether to abort (all failed) or continue with the remainder.
func buildEntries(rawConfigs []httpSDConfigEntry, sharedClient *http.Client) ([]*httpSDEntry, []error) {
	entries := make([]*httpSDEntry, 0, len(rawConfigs))
	var errs []error
	for i, raw := range rawConfigs {
		if raw.URL == "" {
			errs = append(errs, fmt.Errorf("prometheus_http_sd entry %d: url is required", i))
			continue
		}
		if raw.CheckTemplate == "" {
			errs = append(errs, fmt.Errorf("prometheus_http_sd entry %d: check_template is required", i))
			continue
		}
		tmpl, err := parseCheckTemplate(raw.CheckTemplate)
		if err != nil {
			errs = append(errs, fmt.Errorf("prometheus_http_sd entry %d: %v", i, err))
			continue
		}
		filterProg, err := compileExcludeFilter(raw.ExcludeFilter)
		if err != nil {
			errs = append(errs, fmt.Errorf("prometheus_http_sd entry %d: invalid exclude_filter: %v", i, err))
			continue
		}
		entries = append(entries, &httpSDEntry{
			url:           raw.URL,
			client:        sharedClient,
			checkTemplate: tmpl,
			filterProgram: filterProg,
		})
	}
	return entries, errs
}

// PrometheusHTTPSDConfigProvider polls one or more Prometheus HTTP Service
// Discovery endpoints and generates check configurations for each discovered
// target.
type PrometheusHTTPSDConfigProvider struct {
	entries        []*httpSDEntry
	configErrors   map[string]types.ErrorMsgSet
	configErrorsMu sync.RWMutex
}

// NewPrometheusHTTPSDConfigProvider creates a new PrometheusHTTPSDConfigProvider.
func NewPrometheusHTTPSDConfigProvider(
	providerConfig *pkgconfigsetup.ConfigurationProviders,
	_ *telemetry.Store,
) (types.ConfigProvider, error) {
	cfg := pkgconfigsetup.Datadog()

	var rawConfigs []httpSDConfigEntry
	if err := structure.UnmarshalKey(cfg, "prometheus_http_sd.configs", &rawConfigs); err != nil {
		return nil, fmt.Errorf("cannot parse prometheus_http_sd.configs: %v", err)
	}
	if legacyURL := cfg.GetString("prometheus_http_sd.url"); legacyURL != "" {
		rawConfigs = append([]httpSDConfigEntry{{
			URL:           legacyURL,
			CheckTemplate: cfg.GetString("prometheus_http_sd.check_template"),
		}}, rawConfigs...)
	}

	client, err := buildHTTPSDClient(providerConfig)
	if err != nil {
		return nil, err
	}

	entries, entryErrs := buildEntries(rawConfigs, client)
	for _, err := range entryErrs {
		log.Warnf("prometheus_http_sd: skipping invalid entry: %v", err)
	}
	if len(entries) == 0 {
		return nil, fmt.Errorf("prometheus_http_sd: all %d entry(ies) failed to initialize", len(rawConfigs))
	}

	return &PrometheusHTTPSDConfigProvider{
		entries:      entries,
		configErrors: make(map[string]types.ErrorMsgSet),
	}, nil
}

func parseCheckTemplate(templateJSON string) (httpSDCheckTemplate, error) {
	var tmpl httpSDCheckTemplate
	if err := json.Unmarshal([]byte(templateJSON), &tmpl); err != nil {
		return tmpl, fmt.Errorf("cannot parse check_template: %v", err)
	}
	if tmpl.Name == "" {
		return tmpl, errors.New("prometheus_http_sd check_template must specify a check name")
	}
	if len(tmpl.Instances) == 0 {
		return tmpl, errors.New("prometheus_http_sd check_template must specify one instance template")
	}
	return tmpl, nil
}

func buildHTTPSDClient(providerConfig *pkgconfigsetup.ConfigurationProviders) (*http.Client, error) {
	transport := httputils.CreateHTTPTransport(pkgconfigsetup.Datadog())

	// Layer on provider-specific TLS configuration (custom CA, mTLS)
	if providerConfig != nil && providerConfig.CAFile != "" {
		caCert, err := os.ReadFile(providerConfig.CAFile)
		if err != nil {
			return nil, fmt.Errorf("cannot read ca_file %s: %v", providerConfig.CAFile, err)
		}
		caCertPool := x509.NewCertPool()
		if ok := caCertPool.AppendCertsFromPEM(caCert); !ok {
			return nil, fmt.Errorf("cannot parse any certificates from ca_file %s", providerConfig.CAFile)
		}
		transport.TLSClientConfig.RootCAs = caCertPool
	}
	if providerConfig != nil && providerConfig.CertFile != "" && providerConfig.KeyFile != "" {
		cert, err := tls.LoadX509KeyPair(providerConfig.CertFile, providerConfig.KeyFile)
		if err != nil {
			return nil, fmt.Errorf("cannot load client certificate: %v", err)
		}
		transport.TLSClientConfig.Certificates = []tls.Certificate{cert}
	}

	return &http.Client{
		Transport: transport,
		Timeout:   30 * time.Second,
	}, nil
}

// String returns the provider name.
func (h *PrometheusHTTPSDConfigProvider) String() string {
	return names.PrometheusHTTPSD
}

// GetConfigErrors returns a map of config errors.
func (h *PrometheusHTTPSDConfigProvider) GetConfigErrors() map[string]types.ErrorMsgSet {
	h.configErrorsMu.RLock()
	defer h.configErrorsMu.RUnlock()
	result := make(map[string]types.ErrorMsgSet, len(h.configErrors))
	for k, v := range h.configErrors {
		result[k] = v
	}
	return result
}

// IsUpToDate always returns false to force re-polling on every interval.
// The autodiscovery infrastructure diffs against the previous Collect result.
func (h *PrometheusHTTPSDConfigProvider) IsUpToDate(_ context.Context) (bool, error) {
	return false, nil
}

// Collect fetches every configured HTTP SD endpoint and returns check
// configurations for each discovered target.
func (h *PrometheusHTTPSDConfigProvider) Collect(_ context.Context) ([]integration.Config, error) {
	h.configErrorsMu.Lock()
	h.configErrors = make(map[string]types.ErrorMsgSet)
	h.configErrorsMu.Unlock()

	var configs []integration.Config
	var entryErrors []string
	successes := 0
	for _, entry := range h.entries {
		entryConfigs, err := entry.collect()
		if err != nil {
			log.Warnf("http_sd: failed to fetch targets from %s: %v", entry.url, err)
			h.configErrorsMu.Lock()
			h.configErrors["fetch:"+entry.url] = types.ErrorMsgSet{err.Error(): struct{}{}}
			h.configErrorsMu.Unlock()
			entryErrors = append(entryErrors, fmt.Sprintf("%s: %v", entry.url, err))
			continue
		}
		configs = append(configs, entryConfigs...)
		successes++
	}

	if successes == 0 && len(h.entries) > 0 {
		return nil, fmt.Errorf("prometheus_http_sd: all %d endpoint(s) failed: %s", len(h.entries), strings.Join(entryErrors, "; "))
	}

	log.Infof("http_sd: collected %d configs from %d/%d endpoint(s)", len(configs), successes, len(h.entries))
	return configs, nil
}

func (e *httpSDEntry) collect() ([]integration.Config, error) {
	targetGroups, err := e.fetchTargets()
	if err != nil {
		return nil, err
	}

	var configs []integration.Config
	for _, tg := range targetGroups {
		tags := labelsToTags(tg.Labels)

		for _, target := range tg.Targets {
			host, port, splitErr := net.SplitHostPort(target)
			if splitErr != nil {
				host = target
				port = ""
			}

			if excluded, filterErr := e.isExcluded(host, port, tg.Labels); filterErr == nil && excluded {
				log.Debugf("http_sd: target %s excluded by filter", target)
				continue
			}

			config, buildErr := e.buildConfig(host, port, tags)
			if buildErr != nil {
				log.Warnf("http_sd: failed to build config for target %s: %v", target, buildErr)
				continue
			}
			configs = append(configs, config)
		}
	}

	return configs, nil
}

func (e *httpSDEntry) fetchTargets() ([]httpSDTargetGroup, error) {
	req, err := http.NewRequest(http.MethodGet, e.url, nil)
	if err != nil {
		return nil, fmt.Errorf("cannot create request: %v", err)
	}

	resp, err := e.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return nil, fmt.Errorf("unexpected status %d: %s", resp.StatusCode, string(body))
	}

	var targetGroups []httpSDTargetGroup
	if err := json.NewDecoder(resp.Body).Decode(&targetGroups); err != nil {
		return nil, fmt.Errorf("cannot decode response: %v", err)
	}

	return targetGroups, nil
}

func (e *httpSDEntry) buildConfig(host, port string, tags []string) (integration.Config, error) {
	initConfigBytes, err := yaml.Marshal(e.checkTemplate.InitConfig)
	if err != nil {
		return integration.Config{}, fmt.Errorf("cannot marshal init_config: %v", err)
	}

	// Apply template substitution on the first instance template
	instanceTemplate := e.checkTemplate.Instances[0]
	instance := make(map[string]interface{}, len(instanceTemplate))
	for k, v := range instanceTemplate {
		instance[k] = substituteTemplateVars(v, host, port)
	}

	// Merge target group labels as tags
	if len(tags) > 0 {
		existingTags, _ := instance["tags"].([]interface{})
		for _, t := range tags {
			existingTags = append(existingTags, t)
		}
		instance["tags"] = existingTags
	}

	instanceBytes, err := yaml.Marshal(instance)
	if err != nil {
		return integration.Config{}, fmt.Errorf("cannot marshal instance: %v", err)
	}

	return integration.Config{
		Name:         e.checkTemplate.Name,
		InitConfig:   integration.Data(initConfigBytes),
		Instances:    []integration.Data{instanceBytes},
		ClusterCheck: true,
		Source:       "prometheus_http_sd:" + e.url,
		Provider:     names.PrometheusHTTPSDRegisterName,
	}, nil
}

// labelsToTags converts HTTP SD labels to Datadog tags.
// Internal labels (prefixed with __) except __meta_ are skipped.
// Tags are sorted for stable config digests across polls.
func labelsToTags(labels map[string]string) []string {
	var tags []string
	for k, v := range labels {
		tagKey := k
		if strings.HasPrefix(k, "__meta_") {
			tagKey = strings.TrimPrefix(k, "__meta_")
		} else if strings.HasPrefix(k, "__") {
			continue
		}
		tags = append(tags, tagKey+":"+v)
	}
	sort.Strings(tags)
	return tags
}

// substituteTemplateVars replaces %%host%% and %%port%% placeholders in a value.
// Only string values are substituted; all other types are returned as-is.
func substituteTemplateVars(v interface{}, host, port string) interface{} {
	s, ok := v.(string)
	if !ok {
		return v
	}
	s = strings.ReplaceAll(s, "%%host%%", host)
	s = strings.ReplaceAll(s, "%%port%%", port)
	return s
}

// compileExcludeFilter compiles a CEL expression into a reusable program.
// The expression receives a single "target" variable of type httpSDTarget with
// fields "host" (string), "port" (string), and "labels" (map<string, string>).
// Returns nil for an empty expression.
func compileExcludeFilter(expr string) (cel.Program, error) {
	if expr == "" {
		return nil, nil
	}
	env, err := cel.NewEnv(
		ext.NativeTypes(reflect.TypeOf(httpSDTarget{}), ext.ParseStructTag("json")),
		cel.Variable("target", cel.ObjectType("providers.httpSDTarget")),
	)
	if err != nil {
		return nil, err
	}
	ast, issues := env.Compile(expr)
	if issues != nil && issues.Err() != nil {
		return nil, issues.Err()
	}
	if ast.OutputType() != cel.BoolType {
		return nil, fmt.Errorf("exclude_filter must return bool, got %s", ast.OutputType())
	}
	return env.Program(ast)
}

// isExcluded evaluates the entry's exclude_filter against the given target fields.
// Returns false (not excluded) when no filter is set or evaluation fails.
func (e *httpSDEntry) isExcluded(host, port string, labels map[string]string) (bool, error) {
	if e.filterProgram == nil {
		return false, nil
	}
	out, _, err := e.filterProgram.Eval(map[string]any{
		"target": httpSDTarget{Host: host, Port: port, Labels: labels},
	})
	if err != nil {
		return false, err
	}
	return out.Value().(bool), nil
}
