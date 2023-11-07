// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package api

import (
	"fmt"
	stdlog "log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/pkg/trace/log"
	"golang.org/x/exp/slices"
)

const (
	serverlessFlareEndpointPath = "/api/ui/support/serverless/flare"
	datadogSiteUS1              = "datadoghq.com"
	datadogSiteEU1              = "datadoghq.eu"
	datadogSiteUS3              = "us3.datadoghq.com"
	datadogSiteUS5              = "us5.datadoghq.com"
	datadogSiteAP1              = "ap1.datadoghq.com"
	datadogSiteGov              = "ddog-gov.com"
)

type EndpointGetter func(url *url.URL) error

// tracerFlareTransport forwards request to tracer flare endpoint.
type tracerFlareTransport struct {
	rt          http.RoundTripper
	getEndpoint EndpointGetter
}

func getServerlessFlareEndpoint(url *url.URL) error {
	reqHost := url.Host
	datadogSites := []string{datadogSiteUS1, datadogSiteEU1, datadogSiteUS3, datadogSiteUS5, datadogSiteAP1, datadogSiteGov}
	if !slices.Contains(datadogSites, reqHost) {
		return fmt.Errorf("tracer_flare.Proxy: invalid site: %s. Must be one of: %s", reqHost, strings.Join(datadogSites[:], ","))
	}

	// The DNS doesn't redirect to the proper endpoint when a subdomain is not present in the baseUrl.
	// See https://github.com/DataDog/datadog-ci/blob/master/src/helpers/flare.ts#L92
	noSubdomainSites := []string{datadogSiteUS1, datadogSiteEU1, datadogSiteGov}
	if slices.Contains(noSubdomainSites, reqHost) {
		url.Host = "app." + reqHost
	}

	url.Scheme = "https"
	url.Path = serverlessFlareEndpointPath

	return nil
}

func (m *tracerFlareTransport) RoundTrip(req *http.Request) (rresp *http.Response, rerr error) {
	err := m.getEndpoint(req.URL)
	if err != nil {
		return nil, err
	}
	return m.rt.RoundTrip(req)
}

func (r *HTTPReceiver) tracerFlareHandler() http.Handler {
	apiKey := r.conf.APIKey()

	director := func(req *http.Request) {
		req.Header.Set("DD-API-KEY", apiKey)
	}

	logger := log.NewThrottled(5, 10*time.Second) // limit to 5 messages every 10 seconds
	transport := r.conf.NewHTTPTransport()
	return &httputil.ReverseProxy{
		Director:  director,
		ErrorLog:  stdlog.New(logger, "tracer_flare.Proxy: ", 0),
		Transport: &tracerFlareTransport{transport, getServerlessFlareEndpoint},
	}
}
