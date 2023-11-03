// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

// Package net provides local access to system probe
package net

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	sysconfig "github.com/DataDog/datadog-agent/cmd/system-probe/config"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/system/wincrashdetect/probe"
)

const (
	checksURL = "http://%s/%s/check"
)

// GetCheck returns the check output of the specified module
func (r *RemoteSysProbeUtil) GetCheck(module sysconfig.ModuleName) (interface{}, error) {

	switch module {
	default:
		return nil, fmt.Errorf("invalid check name: %s", module)
	case sysconfig.WindowsCrashDetectModule:
		// don't need to do anything

		// as additional checks are added, simply add case statements for
		// newly expected check names.
	}
	url := fmt.Sprintf(checksURL, r.path, module)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := r.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("conn request failed: socket %s, url %s, status code: %d", r.path, fmt.Sprintf(checksURL, r.path, module), resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if module == sysconfig.WindowsCrashDetectModule {
		var data probe.WinCrashStatus
		err = json.Unmarshal(body, &data)
		if err != nil {
			return nil, err
		}
		return data, nil
	}

	return nil, fmt.Errorf("invalid check name: %s", module)
}
