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

	yaml "go.yaml.in/yaml/v2"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/providers/names"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/providers/types"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/telemetry"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup" //nolint:depguard
	httputils "github.com/DataDog/datadog-agent/pkg/util/http"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// httpSDTargetGroup represents a target group returned by an HTTP SD endpoint,
// following the Prometheus HTTP Service Discovery format.
type httpSDTargetGroup struct {
	Targets []string          `json:"targets"`
	Labels  map[string]string `json:"labels,omitempty"`
}

// httpSDCheckTemplate represents the check configuration template
// applied to each discovered target.
type httpSDCheckTemplate struct {
	Name       string                   `json:"name"`
	InitConfig map[string]interface{}   `json:"init_config"`
	Instances  []map[string]interface{} `json:"instances"`
}

// PrometheusHTTPSDConfigProvider polls a Prometheus HTTP Service Discovery endpoint
// and generates check configurations for each discovered target.
type PrometheusHTTPSDConfigProvider struct {
	url            string
	client         *http.Client
	checkTemplate  httpSDCheckTemplate
	configErrors   map[string]types.ErrorMsgSet
	configErrorsMu sync.RWMutex
}

// NewPrometheusHTTPSDConfigProvider creates a new PrometheusHTTPSDConfigProvider.
func NewPrometheusHTTPSDConfigProvider(
	providerConfig *pkgconfigsetup.ConfigurationProviders,
	_ *telemetry.Store,
) (types.ConfigProvider, error) {
	url := pkgconfigsetup.Datadog().GetString("prometheus_http_sd.url")
	if url == "" {
		return nil, errors.New("prometheus_http_sd provider requires a URL (set prometheus_http_sd.url)")
	}
	templateJSON := pkgconfigsetup.Datadog().GetString("prometheus_http_sd.check_template")
	if templateJSON == "" {
		return nil, errors.New("prometheus_http_sd provider requires a check template (set prometheus_http_sd.check_template)")
	}
	var tmpl httpSDCheckTemplate
	if err := json.Unmarshal([]byte(templateJSON), &tmpl); err != nil {
		return nil, fmt.Errorf("cannot parse check_template: %v", err)
	}
	if tmpl.Name == "" {
		return nil, errors.New("prometheus_http_sd check_template must specify a check name")
	}
	if len(tmpl.Instances) == 0 {
		return nil, errors.New("prometheus_http_sd check_template must specify one instance template")
	}

	client, err := buildHTTPSDClient(providerConfig)
	if err != nil {
		return nil, err
	}

	return &PrometheusHTTPSDConfigProvider{
		url:           url,
		client:        client,
		checkTemplate: tmpl,
		configErrors:  make(map[string]types.ErrorMsgSet),
	}, nil
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

// Collect fetches the HTTP SD endpoint and returns check configurations
// for each discovered target.
func (h *PrometheusHTTPSDConfigProvider) Collect(_ context.Context) ([]integration.Config, error) {
	h.configErrorsMu.Lock()
	h.configErrors = make(map[string]types.ErrorMsgSet)
	h.configErrorsMu.Unlock()

	targetGroups, err := h.fetchTargets()
	if err != nil {
		h.configErrorsMu.Lock()
		h.configErrors["fetch"] = types.ErrorMsgSet{err.Error(): struct{}{}}
		h.configErrorsMu.Unlock()
		return nil, fmt.Errorf("prometheus_http_sd: failed to fetch targets from %s: %v", h.url, err)
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

			config, buildErr := h.buildConfig(host, port, tags)
			if buildErr != nil {
				log.Warnf("http_sd: failed to build config for target %s: %v", target, buildErr)
				continue
			}
			configs = append(configs, config)
		}
	}

	log.Infof("http_sd: collected %d configs from %s", len(configs), h.url)
	return configs, nil
}

func (h *PrometheusHTTPSDConfigProvider) fetchTargets() ([]httpSDTargetGroup, error) {
	req, err := http.NewRequest(http.MethodGet, h.url, nil)
	if err != nil {
		return nil, fmt.Errorf("cannot create request: %v", err)
	}

	resp, err := h.client.Do(req)
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

func (h *PrometheusHTTPSDConfigProvider) buildConfig(host, port string, tags []string) (integration.Config, error) {
	initConfigBytes, err := yaml.Marshal(h.checkTemplate.InitConfig)
	if err != nil {
		return integration.Config{}, fmt.Errorf("cannot marshal init_config: %v", err)
	}

	// Apply template substitution on the first instance template
	instanceTemplate := h.checkTemplate.Instances[0]
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
		Name:         h.checkTemplate.Name,
		InitConfig:   integration.Data(initConfigBytes),
		Instances:    []integration.Data{instanceBytes},
		ClusterCheck: true,
		Source:       "prometheus_http_sd:" + h.url,
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
