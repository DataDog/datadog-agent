// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package api

import (
	"bytes"
	"fmt"
	"io"
	stdlog "log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/pkg/trace/config"
	"github.com/DataDog/datadog-agent/pkg/trace/log"
	"github.com/DataDog/datadog-go/v5/statsd"
)

const (
	openlineageURLTemplate = "https://data-obs-intake.%s/api/v1/lineage"
	openlineageURLDefault  = "https://data-obs-intake.datadoghq.com/api/v1/lineage"
)

// openLineageEndpoint returns the openlineage intake url and the corresponding API key.
func openLineageEndpoints(cfg *config.AgentConfig) (urls []*url.URL, apiKeys []string, err error) {
	host := openlineageURLDefault
	apiKey := cfg.OpenLineageProxy.APIKey
	if apiKey == "" {
		apiKey = cfg.APIKey()
	}
	site := cfg.OpenLineageProxy.DDURL
	if site != "" {
		// Directly passed URL
		if strings.HasPrefix(site, "http://") || strings.HasPrefix(site, "https://") {
			host = site
		} else {
			host = fmt.Sprintf(openlineageURLTemplate, site)
		}
	} else if cfg.Site != "" {
		// Fallback to the main agent site
		host = fmt.Sprintf(openlineageURLTemplate, cfg.Site)
	}
	log.Debugf("[openlineage] OpenLineage Host: %s", host)
	u, err := url.Parse(host)
	if err != nil {
		// if the main intake URL is invalid we don't use additional endpoints
		return nil, nil, fmt.Errorf("[openlineage] error parsing intake URL %s: %v", host, err)
	}

	if cfg.OpenLineageProxy.APIVersion >= 2 {
		addOpenLineageAPIVersion(u, cfg.OpenLineageProxy.APIVersion)
	}

	urls = append(urls, u)
	apiKeys = append(apiKeys, apiKey)

	for host, keys := range cfg.OpenLineageProxy.AdditionalEndpoints {
		for _, key := range keys {
			urlStr := fmt.Sprintf(openlineageURLTemplate, host)
			u, err := url.Parse(urlStr)
			if err != nil {
				log.Errorf("[openlineage] error parsing additional intake URL %s: %v", urlStr, err)
				continue
			}
			if cfg.OpenLineageProxy.APIVersion >= 2 {
				addOpenLineageAPIVersion(u, cfg.OpenLineageProxy.APIVersion)
			}
			urls = append(urls, u)
			apiKeys = append(apiKeys, key)
		}
	}
	return urls, apiKeys, nil
}

func addOpenLineageAPIVersion(u *url.URL, version int) {
	query := u.Query()
	query.Set("api-version", strconv.Itoa(version))
	u.RawQuery = query.Encode()
	log.Debugf("[openlineage] OpenLineage API version added, URL: %s", u.String())
}

func openLineageErrorHandler(message string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		msg := fmt.Sprintf("OpenLineage forwarder is OFF: %s", message)
		http.Error(w, msg, http.StatusInternalServerError)
	})
}

// openLineageProxyHandler returns a new HTTP handler which will proxy requests to the openlineage intake.
func (r *HTTPReceiver) openLineageProxyHandler() http.Handler {
	if !r.conf.OpenLineageProxy.Enabled {
		log.Debug("[openlineage] Proxy is disabled in config")
		return openLineageErrorHandler("Has been disabled in config")
	}
	log.Debug("[openlineage] Creating proxy handler")
	urls, apiKeys, err := openLineageEndpoints(r.conf)
	if err != nil {
		return openLineageErrorHandler(err.Error())
	}
	tags := fmt.Sprintf("host:%s,default_env:%s,agent_version:%s", r.conf.Hostname, r.conf.DefaultEnv, r.conf.AgentVersion)
	return newOpenLineageProxy(r.conf, urls, apiKeys, tags, r.statsd)
}

// newOpenLineageProxy creates an http.ReverseProxy which forwards requests to the openlineage intake.
// The tags will be added as a header to all proxied requests.
func newOpenLineageProxy(conf *config.AgentConfig, urls []*url.URL, keys []string, tags string, statsd statsd.ClientInterface) *httputil.ReverseProxy {
	log.Debug("[openlineage] Creating reverse proxy")
	cidProvider := NewIDProvider(conf.ContainerProcRoot, conf.ContainerIDFromOriginInfo)
	director := func(req *http.Request) {
		req.Header.Set("Via", fmt.Sprintf("trace-agent %s", conf.AgentVersion))
		if _, ok := req.Header["User-Agent"]; !ok {
			// explicitly disable User-Agent so it's not set to the default value
			// that net/http gives it: Go-http-client/1.1
			// See https://codereview.appspot.com/7532043
			req.Header.Set("User-Agent", "")
		}
		containerID := cidProvider.GetContainerID(req.Context(), req.Header)
		if ctags := getContainerTags(conf.ContainerTags, containerID); ctags != "" {
			ctagsHeader := normalizeHTTPHeader(ctags)
			req.Header.Set("X-Datadog-Container-Tags", ctagsHeader)
			log.Debugf("Setting header X-Datadog-Container-Tags=%s for openlineage proxy", ctagsHeader)
		}
		req.Header.Set("X-Datadog-Additional-Tags", tags)
		log.Debugf("Setting header X-Datadog-Additional-Tags=%s for openlineage proxy", tags)
		_ = statsd.Count("datadog.trace_agent.openlineage", 1, nil, 1)

	}
	logger := log.NewThrottled(5, 10*time.Second) // limit to 5 messages every 10 seconds
	return &httputil.ReverseProxy{
		Director:  director,
		ErrorLog:  stdlog.New(logger, "openlineage.Proxy: ", 0),
		Transport: &openLineageTransport{rt: conf.NewHTTPTransport(), urls: urls, keys: keys},
	}
}

// openLineageTransport sends HTTP requests to multiple targets using an
// underlying http.RoundTripper. API keys are set separately for each target.
// When multiple endpoints are in use the response from the main endpoint
// is proxied back to the client, while for all additional endpoints the
// response is discarded. There is no de-duplication done between endpoint
// hosts or api keys.
type openLineageTransport struct {
	rt   http.RoundTripper
	urls []*url.URL
	keys []string
}

func (m *openLineageTransport) RoundTrip(req *http.Request) (rresp *http.Response, rerr error) {
	setTarget := func(r *http.Request, u *url.URL, apiKey string) {
		r.Host = u.Host
		r.URL = u
		// OL endpoint follows where OL OSS client puts API key
		r.Header.Set("Authorization", "Bearer "+apiKey)
	}
	if len(m.urls) == 1 {
		setTarget(req, m.urls[0], m.keys[0])
		rresp, rerr = m.rt.RoundTrip(req)
		if rerr != nil {
			log.Errorf("[openlineage] RoundTrip failed: %v", rerr)
		} else {
			log.Debugf("[openlineage] Returned status: %s, from host: %s, path: %s, query %s", rresp.Status, m.urls[0].Host, m.urls[0].Path, m.urls[0].Query())
		}

		return rresp, rerr
	}
	slurp, err := io.ReadAll(req.Body)
	if err != nil {
		return nil, err
	}
	for i, u := range m.urls {
		newreq := req.Clone(req.Context())
		newreq.Body = io.NopCloser(bytes.NewReader(slurp))
		setTarget(newreq, u, m.keys[i])
		if i == 0 {
			// given the way we construct the list of targets the main endpoint
			// will be the first one called, we return its response and error
			rresp, rerr = m.rt.RoundTrip(newreq)
			if rerr != nil {
				log.Errorf("[openlineage] RoundTrip failed: %v", rerr)
			} else {
				log.Debugf("[openlineage] Returned status: %s, from host: %s, path: %s, query: %s", rresp.Status, u.Host, u.Path, u.Query())
			}
			continue
		}

		if resp, err := m.rt.RoundTrip(newreq); err == nil {
			// we discard responses for all subsequent requests
			io.Copy(io.Discard, resp.Body) //nolint:errcheck
			resp.Body.Close()
		} else {
			log.Error(err)
		}
	}
	return rresp, rerr
}
