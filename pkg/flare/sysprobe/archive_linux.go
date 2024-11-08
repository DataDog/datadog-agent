// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux

package sysprobe

import (
	"fmt"
	"io"
	"net/http"

	sysprobeclient "github.com/DataDog/datadog-agent/cmd/system-probe/api/client"
	sysconfig "github.com/DataDog/datadog-agent/cmd/system-probe/config"
)

// GetSystemProbeConntrackCached queries conntrack/cached, which uses our conntracker implementation (typically ebpf)
// to return the list of NAT'd connections
func GetSystemProbeConntrackCached(client *http.Client) ([]byte, error) {
	url := sysprobeclient.ModuleURL(sysconfig.NetworkTracerModule, "/debug/conntrack/cached")
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf(`GetConnTrackCached got non-success status code: url: %s, status_code: %d, response: "%s"`, req.URL, resp.StatusCode, data)
	}

	return data, nil
}

// GetSystemProbeConntrackHost queries conntrack/host, which uses netlink to return the list of NAT'd connections
func GetSystemProbeConntrackHost(client *http.Client) ([]byte, error) {
	url := sysprobeclient.ModuleURL(sysconfig.NetworkTracerModule, "/debug/conntrack/host")
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf(`GetConnTrackHost got non-success status code: url: %s, status_code: %d, response: "%s"`, req.URL, resp.StatusCode, data)
	}

	return data, nil
}

// GetSystemProbeBTFLoaderInfo queries ebpf_btf_loader_info which gets where the BTF data came from
func GetSystemProbeBTFLoaderInfo(client *http.Client) ([]byte, error) {
	url := sysprobeclient.DebugURL("/ebpf_btf_loader_info")
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf(`GetEbpfBtfInfo got non-success status code: url: %s, status_code: %d, response: "%s"`, req.URL, resp.StatusCode, data)
	}

	return data, nil
}
