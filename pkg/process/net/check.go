// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package net

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	sysconfig "github.com/DataDog/datadog-agent/cmd/system-probe/config"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/ebpf/probe"
)

const (
	checksURL = "http://unix/%s/check"
)

// GetCheck returns the check output of the specified module
func (r *RemoteSysProbeUtil) GetCheck(module sysconfig.ModuleName) (interface{}, error) {
	req, err := http.NewRequest("GET", fmt.Sprintf(checksURL, module), nil)
	if err != nil {
		return nil, err
	}

	resp, err := r.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("conn request failed: socket %s, url %s, status code: %d", r.path, fmt.Sprintf(checksURL, module), resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if module == sysconfig.TCPQueueLengthTracerModule {
		var stats probe.TCPQueueLengthStats
		err = json.Unmarshal(body, &stats)
		if err != nil {
			return nil, err
		}
		return stats, nil
	} else if module == sysconfig.OOMKillProbeModule {
		var stats []probe.OOMKillStats
		err = json.Unmarshal(body, &stats)
		if err != nil {
			return nil, err
		}
		return stats, nil
	}

	return nil, fmt.Errorf("Invalid check name: %s", module)
}
