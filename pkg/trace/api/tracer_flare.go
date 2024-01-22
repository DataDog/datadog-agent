// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package api

import (
	stdlog "log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/pkg/trace/log"
)

const (
	serverlessFlareEndpointPath = "/api/ui/support/serverless/flare"
)

var ddURLRegexp = regexp.MustCompile(`^app(\.[a-z]{2}\d)?\.(datad(oghq|0g)\.(com|eu)|ddog-gov\.com)$`)
var ddNoSubDomainRegexp = regexp.MustCompile(`^(datad(oghq|0g)\.(com|eu)|ddog-gov\.com)$`)
var versionNumsddURLRegexp = regexp.MustCompile(`(\d+)\.(\d+)\.(\d+)`)

type endpointGetter func(url *url.URL, agentVersion string) *url.URL

// tracerFlareTransport forwards request to tracer flare endpoint.
type tracerFlareTransport struct {
	rt           http.RoundTripper
	getEndpoint  endpointGetter
	agentVersion string
	site         string
}

func getServerlessFlareEndpoint(url *url.URL, agentVersion string) *url.URL {
	// The DNS doesn't redirect to the proper endpoint when a subdomain is not present in the baseUrl.
	// Adding app. subdomain here for site like datadoghq.com
	if ddNoSubDomainRegexp.MatchString(url.Path) {
		url.Host = "app." + url.Path
	}

	if ddURLRegexp.MatchString(url.Host) {
		// Following exisiting logic to prefixes the domain with the agent version
		// https://github.com/DataDog/datadog-agent/blob/e9056abe94e8dbddd51bbc901036e7362442f02e/pkg/config/utils/endpoints.go#L129
		subdomain := strings.Split(url.Host, ".")[0]
		versionNums := strings.Join(versionNumsddURLRegexp.FindStringSubmatch(agentVersion)[1:], "-")
		newSubdomain := versionNums + "-flare"

		url.Host = strings.Replace(url.Host, subdomain, newSubdomain, 1)
	}

	url.Scheme = "https"
	url.Path = serverlessFlareEndpointPath

	return url
}

func (m *tracerFlareTransport) RoundTrip(req *http.Request) (rresp *http.Response, rerr error) {
	u, err := url.Parse(m.site)
	if err != nil {
		return nil, err
	}

	req.URL = m.getEndpoint(u, m.agentVersion)
	return m.rt.RoundTrip(req)
}

func (r *HTTPReceiver) tracerFlareHandler() http.Handler {
	apiKey := r.conf.APIKey()
	site := r.conf.Site

	director := func(req *http.Request) {
		req.Header.Set("DD-API-KEY", apiKey)
	}

	logger := log.NewThrottled(5, 10*time.Second) // limit to 5 messages every 10 seconds
	transport := r.conf.NewHTTPTransport()
	agentVersion := r.conf.AgentVersion
	return &httputil.ReverseProxy{
		Director:  director,
		ErrorLog:  stdlog.New(logger, "tracer_flare.Proxy: ", 0),
		Transport: &tracerFlareTransport{transport, getServerlessFlareEndpoint, agentVersion, site},
	}
}
