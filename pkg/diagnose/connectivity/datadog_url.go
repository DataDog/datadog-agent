// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package connectivity

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httptrace"
	"strings"
	"time"

	diagnose "github.com/DataDog/datadog-agent/comp/core/diagnose/def"
	"github.com/DataDog/datadog-agent/comp/core/flare/helpers"
	"github.com/DataDog/datadog-agent/pkg/config/model"

	"crypto/tls"

	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
)

type method int

// URL types for different services
const (
	head method = iota
	intake
)

type serviceInfo struct {
	method method
	route  string
}

func getServicesInfo(cfg model.Reader) []serviceInfo {
	site := cfg.GetString("site")
	if site == "" {
		site = pkgconfigsetup.DefaultSite
	}

	return []serviceInfo{
		{route: buildFromPrefix("install", site), method: head},
		{route: buildFromPrefix("yum", site), method: head},
		{route: buildFromPrefix("apt", site), method: head},
		{route: buildFromPrefix("keys", site), method: head},
		{route: buildFromPrefix("process", site), method: head},
		{route: helpers.GetFlareEndpoint(cfg), method: head},
	}
}

func buildFromPrefix(prefix string, site string) string {
	return fmt.Sprintf("https://%s.%s", prefix, site)
}

// DiagnoseDatadogURL checks connectivity to Datadog endpoints
func DiagnoseDatadogURL(cfg model.Reader) []diagnose.Diagnosis {
	services := getServicesInfo(cfg)
	var diagnoses []diagnose.Diagnosis
	for _, service := range services {
		diagnosis, err := checkServiceConnectivity(service)
		diagnoses = append(diagnoses, diagnose.Diagnosis{
			Status:    diagnose.DiagnosisFail,
			Name:      service.route,
			Diagnosis: diagnosis,
			RawError:  err.Error(),
		})
	}
	return diagnoses
}

// checkServiceConnectivity checks connectivity for a specific service
func checkServiceConnectivity(serviceInfo serviceInfo) (string, error) {
	// Build URL based on service type
	switch serviceInfo.method {
	case head:
		return checkURL(serviceInfo.route)
	default:
		return "Unknown URL type", fmt.Errorf("unknown URL type for service %s", serviceInfo.route)
	}
}

// checkURL verifies if a URL is accessible
func checkURL(url string) (string, error) {
	host := strings.TrimPrefix(url, "https://")
	if err := checkDNSResolution(host); err != nil {
		return "Failed DNS resolution", err
	}

	if err := checkHTTPConnectivity(url); err != nil {
		return "Failed HTTP connectivity", err
	}

	return "Success", nil
}

// checkDNSResolution verifies if a hostname can be resolved
func checkDNSResolution(host string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := net.DefaultResolver.LookupHost(ctx, host)
	if err != nil {
		return fmt.Errorf("DNS resolution failed for %s: %w", host, err)
	}
	return nil
}

// checkHTTPConnectivity verifies if an HTTP URL is accessible
func checkHTTPConnectivity(url string) error {

	var httpTraces []string
	ctx := httptrace.WithClientTrace(context.Background(), createDiagnoseTraces(&httpTraces))

	client2 := &http.Client{
		Timeout: 5 * time.Second,
		Transport: &http.Transport{
			DialContext: (&net.Dialer{
				Timeout:   5 * time.Second,
				KeepAlive: 5 * time.Second,
			}).DialContext,
			TLSClientConfig: &tls.Config{
				MinVersion: tls.VersionTLS12,
			},
			TLSHandshakeTimeout:   5 * time.Second,
			ResponseHeaderTimeout: 5 * time.Second,
			ExpectContinueTimeout: 1 * time.Second,
			MaxIdleConns:          2,
			IdleConnTimeout:       30 * time.Second,
			ForceAttemptHTTP2:     true,
		},
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	req, err := http.NewRequestWithContext(ctx, "HEAD", url, nil)
	if err != nil {
		return fmt.Errorf("failed to create request for %s: %w", url, err)
	}

	resp, err := client2.Do(req)
	if err != nil {
		// Include HTTP traces in error message if available
		if len(httpTraces) > 0 {
			return fmt.Errorf("failed to connect to %s: %w\nTraces:\n%s", url, err, strings.Join(httpTraces, "\n"))
		}
		return fmt.Errorf("failed to connect to %s: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("URL %s returned invalid status code: %d", url, resp.StatusCode)
	}

	return nil
}
