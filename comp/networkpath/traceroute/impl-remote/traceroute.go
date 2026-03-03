// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package remoteimpl implements the traceroute component interface
package remoteimpl

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/DataDog/datadog-agent/comp/core/hostname"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	"github.com/DataDog/datadog-agent/comp/core/sysprobeconfig"
	traceroute "github.com/DataDog/datadog-agent/comp/networkpath/traceroute/def"
	"github.com/DataDog/datadog-agent/pkg/networkpath/payload"
	"github.com/DataDog/datadog-agent/pkg/networkpath/traceroute/config"
	sysprobeclient "github.com/DataDog/datadog-agent/pkg/system-probe/api/client"
	sysconfig "github.com/DataDog/datadog-agent/pkg/system-probe/config"
	"github.com/DataDog/datadog-agent/pkg/util/funcs"
)

// Requires defines the dependencies for the traceroute component
type Requires struct {
	SysprobeConfig sysprobeconfig.Component
	Log            log.Component
	Hostname       hostname.Component
}

// Provides defines the output of the traceroute component
type Provides struct {
	Comp traceroute.Component
}

// NewComponent creates a new traceroute component
func NewComponent(reqs Requires) (Provides, error) {
	rt := &remoteTraceroute{
		sysprobeClient: getSysProbeClient(reqs.SysprobeConfig.GetString("system_probe_config.sysprobe_socket")),
		log:            reqs.Log,
		hostname:       reqs.Hostname,
	}
	provides := Provides{Comp: rt}
	return provides, nil
}

const (
	clientID = "traceroute-agent"
)

type remoteTraceroute struct {
	sysprobeClient *http.Client
	log            log.Component
	hostname       hostname.Component
}

func (t *remoteTraceroute) Run(ctx context.Context, cfg config.Config) (payload.NetworkPath, error) {
	resp, err := t.getTracerouteFromSysProbe(ctx, clientID, cfg.DestHostname, cfg.DestPort, cfg.Protocol, cfg.TCPMethod, cfg.TCPSynParisTracerouteMode, cfg.DisableWindowsDriver, cfg.ReverseDNS, cfg.MaxTTL, cfg.Timeout, cfg.TracerouteQueries, cfg.E2eQueries)
	if err != nil {
		return payload.NetworkPath{}, fmt.Errorf("error getting traceroute: %s", err)
	}

	var path payload.NetworkPath
	if err := json.Unmarshal(resp, &path); err != nil {
		return payload.NetworkPath{}, fmt.Errorf("error unmarshalling response: %w", err)
	}
	agentHostname, err := t.hostname.Get(ctx)
	if err != nil {
		return payload.NetworkPath{}, fmt.Errorf("error getting the hostname: %w", err)
	}
	path.Source.Hostname = agentHostname
	path.Source.ContainerID = cfg.SourceContainerID
	return path, nil
}

var getSysProbeClient = funcs.MemoizeArgNoError(func(socket string) *http.Client {
	return &http.Client{
		Transport: &http.Transport{
			DialContext: sysprobeclient.DialContextFunc(socket),
		},
	}
})

func (t *remoteTraceroute) getTracerouteFromSysProbe(ctx context.Context, clientID string, host string, port uint16, protocol payload.Protocol, tcpMethod payload.TCPMethod, tcpSynParisTracerouteMode bool, disableWindowsDriver bool, reverseDNS bool, maxTTL uint8, timeout time.Duration, tracerouteQueries int, e2eQueries int) ([]byte, error) {
	httpTimeout := timeout*time.Duration(maxTTL) + 10*time.Second // allow extra time for the system probe communication overhead, calculate full timeout for TCP traceroute
	t.log.Tracef("Network Path traceroute HTTP request timeout: %s", httpTimeout)
	ctx, cancel := context.WithTimeout(ctx, httpTimeout)
	defer cancel()

	url := sysprobeclient.ModuleURL(sysconfig.TracerouteModule, fmt.Sprintf("/traceroute/%s?client_id=%s&port=%d&max_ttl=%d&timeout=%d&protocol=%s&tcp_method=%s&tcp_syn_paris_traceroute_mode=%t&disable_windows_driver=%t&reverse_dns=%t&traceroute_queries=%d&e2e_queries=%d", host, clientID, port, maxTTL, timeout, protocol, tcpMethod, tcpSynParisTracerouteMode, disableWindowsDriver, reverseDNS, tracerouteQueries, e2eQueries))
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Accept", "application/json")
	resp, err := t.sysprobeClient.Do(req)
	if err != nil {
		var opErr *net.OpError
		if errors.As(err, &opErr) && opErr.Op == "dial" {
			return nil, fmt.Errorf("%w, please check that the traceroute module is enabled in the system-probe.yaml config file and that system-probe is running", err)
		}
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("traceroute request failed: url: %s, status code: %d, please check that the traceroute module is enabled in the system-probe.yaml config file", req.URL, resp.StatusCode)
	} else if resp.StatusCode != http.StatusOK {
		body, err := sysprobeclient.ReadAllResponseBody(resp)
		if err != nil {
			return nil, fmt.Errorf("traceroute request failed: url: %s, status code: %d", req.URL, resp.StatusCode)
		}
		return nil, fmt.Errorf("traceroute request failed: url: %s, status code: %d, error: %s", req.URL, resp.StatusCode, string(body))
	}

	body, err := sysprobeclient.ReadAllResponseBody(resp)
	if err != nil {
		return nil, err
	}

	return body, nil
}
