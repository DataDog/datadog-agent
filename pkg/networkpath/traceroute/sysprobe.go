// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux || windows

package traceroute

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/DataDog/datadog-agent/pkg/networkpath/payload"
	sysprobeclient "github.com/DataDog/datadog-agent/pkg/system-probe/api/client"
	sysconfig "github.com/DataDog/datadog-agent/pkg/system-probe/config"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

func getTraceroute(client *http.Client, clientID string, host string, port uint16, protocol payload.Protocol, maxTTL uint8, timeout time.Duration) ([]byte, error) {
	httpTimeout := timeout*time.Duration(maxTTL) + 10*time.Second // allow extra time for the system probe communication overhead, calculate full timeout for TCP traceroute
	log.Tracef("Network Path traceroute HTTP request timeout: %s", httpTimeout)
	ctx, cancel := context.WithTimeout(context.Background(), httpTimeout)
	defer cancel()

	url := sysprobeclient.ModuleURL(sysconfig.TracerouteModule, fmt.Sprintf("/traceroute/%s?client_id=%s&port=%d&max_ttl=%d&timeout=%d&protocol=%s", host, clientID, port, maxTTL, timeout, protocol))
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
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
			return nil, fmt.Errorf("traceroute request failed: url: %s, status code: %d", req.URL, resp.StatusCode)
		}
		return nil, fmt.Errorf("traceroute request failed: url: %s, status code: %d, error: %s", req.URL, resp.StatusCode, string(body))
	} else if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("traceroute request failed: url: %s, status code: %d", req.URL, resp.StatusCode)
	}

	body, err := sysprobeclient.ReadAllResponseBody(resp)
	if err != nil {
		return nil, err
	}

	return body, nil
}
