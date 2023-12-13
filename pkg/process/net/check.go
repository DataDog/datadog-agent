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
	ebpfcheck "github.com/DataDog/datadog-agent/pkg/collector/corechecks/ebpf/probe/ebpfcheck/model"
	oomkill "github.com/DataDog/datadog-agent/pkg/collector/corechecks/ebpf/probe/oomkill/model"
	tcpqueuelength "github.com/DataDog/datadog-agent/pkg/collector/corechecks/ebpf/probe/tcpqueuelength/model"
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
		var stats tcpqueuelength.TCPQueueLengthStats
		err = json.Unmarshal(body, &stats)
		if err != nil {
			return nil, err
		}
		return stats, nil
	} else if module == sysconfig.OOMKillProbeModule {
		var stats []oomkill.OOMKillStats
		err = json.Unmarshal(body, &stats)
		if err != nil {
			return nil, err
		}
		return stats, nil
	} else if module == sysconfig.EBPFModule {
		var stats ebpfcheck.EBPFStats
		err = json.Unmarshal(body, &stats)
		if err != nil {
			return nil, err
		}
		return stats, nil
	}

	return nil, fmt.Errorf("invalid check name: %s", module)
}
