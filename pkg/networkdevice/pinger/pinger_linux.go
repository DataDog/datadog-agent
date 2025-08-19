// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package pinger

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	sysprobeclient "github.com/DataDog/datadog-agent/pkg/system-probe/api/client"
	sysconfig "github.com/DataDog/datadog-agent/pkg/system-probe/config"
)

const (
	clientID = "pinger-agent-linux"
)

// LinuxPinger implements the Pinger interface for
// Linux users
type LinuxPinger struct {
	cfg            Config
	sysprobeClient *http.Client
}

// New creates a LinuxPinger using the passed in
// config
func New(cfg Config) (Pinger, error) {
	return &LinuxPinger{
		cfg:            cfg,
		sysprobeClient: sysprobeclient.Get(pkgconfigsetup.SystemProbe().GetString("system_probe_config.sysprobe_socket")),
	}, nil
}

// Ping takes a host and depending on the config will either
// directly ping the host sending packets over a UDP socket
// or a raw socket
func (p *LinuxPinger) Ping(host string) (*Result, error) {
	if !p.cfg.UseRawSocket {
		return RunPing(&p.cfg, host)
	}

	return getPing(p.sysprobeClient, clientID, host, p.cfg.Count, p.cfg.Interval, p.cfg.Timeout)
}

func getPing(client *http.Client, clientID string, host string, count int, interval time.Duration, timeout time.Duration) (*Result, error) {
	url := sysprobeclient.ModuleURL(sysconfig.PingModule, fmt.Sprintf("/ping/%s?client_id=%s&count=%d&interval=%d&timeout=%d", host, clientID, count, interval, timeout))
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Accept", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusBadRequest {
		body, err := sysprobeclient.ReadAllResponseBody(resp)
		if err != nil {
			return nil, fmt.Errorf("ping request failed: url: %s, status code: %d", req.URL, resp.StatusCode)
		}
		return nil, fmt.Errorf("ping request failed: url: %s, status code: %d, error: %s", req.URL, resp.StatusCode, string(body))
	} else if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ping request failed: url: %s, status code: %d", req.URL, resp.StatusCode)
	}

	body, err := sysprobeclient.ReadAllResponseBody(resp)
	if err != nil {
		return nil, err
	}

	var result Result
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, err
	}
	return &result, nil
}
