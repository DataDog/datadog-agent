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
	"os"
	"strings"
	"time"

	diagnose "github.com/DataDog/datadog-agent/comp/core/diagnose/def"
)

// URL types for different services
const (
	httpProtocol = iota
	ociProtocol
)

type serviceURL struct {
	prefix   string
	protocol int
}

var (
	// Map of services to check
	services = map[string]serviceURL{
		"Installer HTTP": {prefix: "install", protocol: httpProtocol},
		"Installer OCI":  {prefix: "install", protocol: ociProtocol},
		"YUM":            {prefix: "yum", protocol: httpProtocol},
		"APT":            {prefix: "apt", protocol: httpProtocol},
	}
)

func init() {
	// Register all service checks
	for name := range services {
		diagnose.RegisterMetadataAvail(name+" connectivity", func(name string) func() error {
			return func() error { return checkServiceConnectivity(name) }
		}(name))
	}
}

// getDomainForSite returns the appropriate domain based on the site
// For example:
// - datadoghq.com -> datadoghq.com
// - datad0g.com -> datad0g.com
// - datadoghq.eu -> datadoghq.eu
func getDomainForSite(site string) string {
	// Extract the domain from the site
	parts := strings.Split(site, ".")
	if len(parts) < 2 {
		return "datadoghq.com" // Default to datadoghq.com if site is invalid
	}
	return strings.Join(parts[len(parts)-2:], ".")
}

// checkServiceConnectivity checks connectivity for a specific service
func checkServiceConnectivity(serviceName string) error {
	site := os.Getenv("DD_SITE")
	if site == "" {
		site = "datadoghq.com"
	}

	domain := getDomainForSite(site)
	service := services[serviceName]

	// Build URL based on service type
	var url string
	switch service.protocol {
	case httpProtocol:
		url = fmt.Sprintf("https://%s.%s", service.prefix, domain)
		return checkHTTPConnectivity(url)
	case ociProtocol:
		url = fmt.Sprintf("oci://%s.%s", service.prefix, domain)
		return checkURL(url)
	default:
		return fmt.Errorf("unknown URL type for service %s", serviceName)
	}
}

// checkURL verifies if a URL is accessible
func checkURL(url string) error {
	// Add https:// prefix if not present and not an OCI URL
	if !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") && !strings.HasPrefix(url, "oci://") {
		url = "https://" + url
	}

	// For OCI URLs, check DNS resolution only
	if strings.HasPrefix(url, "oci://") {
		host := strings.TrimPrefix(url, "oci://")
		return checkDNSResolution(host)
	}

	// For HTTP URLs, check both DNS resolution and HTTP connectivity
	host := strings.TrimPrefix(strings.TrimPrefix(url, "http://"), "https://")
	if err := checkDNSResolution(host); err != nil {
		return err
	}

	return checkHTTPConnectivity(url)
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

	client := &http.Client{
		Timeout: 10 * time.Second,
		Transport: &http.Transport{
			DialContext: (&net.Dialer{
				Timeout:   5 * time.Second,
				KeepAlive: 5 * time.Second,
			}).DialContext,
			TLSHandshakeTimeout:   5 * time.Second,
			ResponseHeaderTimeout: 5 * time.Second,
			ExpectContinueTimeout: 1 * time.Second,
			MaxIdleConns:          2,
			IdleConnTimeout:       30 * time.Second,
		},
	}

	req, err := http.NewRequestWithContext(ctx, "HEAD", url, nil)
	if err != nil {
		return fmt.Errorf("failed to create request for %s: %w", url, err)
	}

	resp, err := client.Do(req)
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
