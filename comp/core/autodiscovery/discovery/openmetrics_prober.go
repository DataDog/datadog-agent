// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package discovery

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/listeners"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	defaultPath        = "/metrics"
	defaultPerProbe    = 500 * time.Millisecond
	defaultBudget      = 2 * time.Second
	defaultMaxAttempts = 8
	defaultFailureTTL  = 30 * time.Second
)

var promLineRe = regexp.MustCompile(`^[a-zA-Z_:][a-zA-Z0-9_:]*(\{[^}]*\})?\s+\S+`)

// OpenMetricsProberOption configures an OpenMetricsProber.
type OpenMetricsProberOption func(*openMetricsProber)

// WithFailureTTL overrides the negative-cache TTL.
func WithFailureTTL(d time.Duration) OpenMetricsProberOption {
	return func(p *openMetricsProber) { p.failureTTL = d }
}

type openMetricsProber struct {
	client      *http.Client
	cache       *probeCache
	perProbe    time.Duration
	totalBudget time.Duration
	maxAttempts int
	failureTTL  time.Duration
}

// NewOpenMetricsProber returns a Prober that verifies OpenMetrics endpoints.
func NewOpenMetricsProber(opts ...OpenMetricsProberOption) Prober {
	p := &openMetricsProber{
		client:      &http.Client{Transport: &http.Transport{DisableKeepAlives: true}},
		cache:       newProbeCache(time.Now),
		perProbe:    defaultPerProbe,
		totalBudget: defaultBudget,
		maxAttempts: defaultMaxAttempts,
		failureTTL:  defaultFailureTTL,
	}
	for _, o := range opts {
		o(p)
	}
	return p
}

func (p *openMetricsProber) Probe(ctx context.Context, cfg *integration.DiscoveryConfig, svc listeners.Service) (ProbeResult, bool) {
	if cfg == nil || cfg.Type != "openmetrics" {
		return ProbeResult{}, false
	}
	host, ok := pickHost(svc)
	if !ok {
		log.Debugf("autodiscovery/discovery: %s has no host, skipping", svc.GetServiceID())
		return ProbeResult{}, false
	}
	exposed, err := svc.GetPorts()
	if err != nil || len(exposed) == 0 {
		return ProbeResult{}, false
	}

	cfgHash := hashDiscoveryConfig(cfg)
	if r, success, hit := p.cache.get(svc.GetServiceID(), cfgHash); hit {
		return r, success
	}

	path := cfg.Path
	if path == "" {
		path = defaultPath
	}
	candidates := candidatePorts(cfg.Ports, exposed)
	deadline := time.Now().Add(p.totalBudget)

	attempts := 0
	for _, port := range candidates {
		if attempts >= p.maxAttempts || time.Now().After(deadline) {
			break
		}
		attempts++
		if p.tryPort(ctx, host, port, path) {
			r := ProbeResult{Port: port}
			p.cache.putSuccess(svc.GetServiceID(), cfgHash, r)
			log.Infof("autodiscovery/discovery: probe matched %s:%d%s for %s", host, port, path, svc.GetServiceID())
			return r, true
		}
	}

	p.cache.putFailure(svc.GetServiceID(), cfgHash, p.failureTTL)
	log.Debugf("autodiscovery/discovery: %d candidate(s) for %s did not match", len(candidates), svc.GetServiceID())
	return ProbeResult{}, false
}

func (p *openMetricsProber) tryPort(ctx context.Context, host string, port uint16, path string) bool {
	url := "http://" + net.JoinHostPort(host, strconv.Itoa(int(port))) + path
	tctx, cancel := context.WithTimeout(ctx, p.perProbe)
	defer cancel()
	req, err := http.NewRequestWithContext(tctx, http.MethodGet, url, nil)
	if err != nil {
		return false
	}
	resp, err := p.client.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
	if err != nil {
		return false
	}
	return verifyOpenMetricsResponse(resp.StatusCode, resp.Header.Get("Content-Type"), body)
}

func verifyOpenMetricsResponse(status int, contentType string, body []byte) bool {
	if status != http.StatusOK {
		return false
	}
	ct := strings.ToLower(contentType)
	if !strings.HasPrefix(ct, "text/plain") && !strings.HasPrefix(ct, "application/openmetrics-text") {
		return false
	}
	for _, line := range strings.Split(string(body), "\n") {
		s := strings.TrimSpace(line)
		if s == "" || strings.HasPrefix(s, "#") {
			continue
		}
		return promLineRe.MatchString(s)
	}
	return false
}

func pickHost(svc listeners.Service) (string, bool) {
	hosts, err := svc.GetHosts()
	if err != nil || len(hosts) == 0 {
		return "", false
	}
	if h, ok := hosts["bridge"]; ok && h != "" {
		return h, true
	}
	for _, h := range hosts {
		if h != "" {
			return h, true
		}
	}
	return "", false
}

func hashDiscoveryConfig(cfg *integration.DiscoveryConfig) string {
	h := sha256.New()
	fmt.Fprintf(h, "%s|%s|", cfg.Type, cfg.Path)
	for _, p := range cfg.Ports {
		fmt.Fprintf(h, "%d,", p)
	}
	return hex.EncodeToString(h.Sum(nil))
}
