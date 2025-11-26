// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux || windows

package net

import (
	"bytes"
	"fmt"
	"net/http"

	model "github.com/DataDog/agent-payload/v5/process"

	procEncoding "github.com/DataDog/datadog-agent/pkg/process/encoding"
	reqEncoding "github.com/DataDog/datadog-agent/pkg/process/encoding/request"
	pbgo "github.com/DataDog/datadog-agent/pkg/proto/pbgo/process"
	sysprobeclient "github.com/DataDog/datadog-agent/pkg/system-probe/api/client"
	sysconfig "github.com/DataDog/datadog-agent/pkg/system-probe/config"
)

// GetProcStats returns a set of process stats by querying system-probe
func GetProcStats(client *http.Client, pids []int32) (*model.ProcStatsWithPermByPID, error) {
	procReq := &pbgo.ProcessStatRequest{
		Pids: pids,
	}

	reqBody, err := reqEncoding.GetMarshaler(reqEncoding.ContentTypeProtobuf).Marshal(procReq)
	if err != nil {
		return nil, err
	}

	url := sysprobeclient.ModuleURL(sysconfig.ProcessModule, "/stats")
	req, err := http.NewRequest("POST", url, bytes.NewReader(reqBody))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Accept", procEncoding.ContentTypeProtobuf)
	req.Header.Set("Content-Type", procEncoding.ContentTypeProtobuf)
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("proc_stats request failed: url: %s, status code: %d", req.URL, resp.StatusCode)
	}

	body, err := sysprobeclient.ReadAllResponseBody(resp)
	if err != nil {
		return nil, err
	}

	contentType := resp.Header.Get("Content-type")
	results, err := procEncoding.GetUnmarshaler(contentType).Unmarshal(body)
	if err != nil {
		return nil, err
	}

	return results, nil
}
