// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package traceroute adds traceroute functionality to the agent
package traceroute

import (
	"context"
	"fmt"
	"net/http"
	"time"

	sysprobeclient "github.com/DataDog/datadog-agent/cmd/system-probe/api/client"
	sysconfig "github.com/DataDog/datadog-agent/cmd/system-probe/config"
	"github.com/DataDog/datadog-agent/pkg/networkpath/payload"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

type (
	// Config specifies the configuration of an instance
	// of Traceroute
	Config struct {
		// TODO: add common configuration
		// Destination Hostname
		DestHostname string
		// Destination Port number
		DestPort uint16
		// Destination service name
		DestinationService string
		// Source service name
		SourceService string
		// Source container ID
		SourceContainerID string
		// Max number of hops to try
		MaxTTL uint8
		// TODO: do we want to expose this?
		Timeout time.Duration
		// Protocol is the protocol to use
		// for traceroute, default is UDP
		Protocol payload.Protocol
	}

	// Traceroute defines an interface for running
	// traceroutes for the Network Path integration
	Traceroute interface {
		Run(context.Context) (payload.NetworkPath, error)
	}
)

func getTraceroute(client *http.Client, clientID string, host string, port uint16, protocol payload.Protocol, maxTTL uint8, timeout time.Duration) ([]byte, error) {
	httpTimeout := timeout*time.Duration(maxTTL) + 10*time.Second // allow extra time for the system probe communication overhead, calculate full timeout for TCP traceroute
	log.Tracef("Network Path traceroute HTTP request timeout: %s", httpTimeout)
	ctx, cancel := context.WithTimeout(context.Background(), httpTimeout)
	defer cancel()

	url := sysprobeclient.URL(sysconfig.TracerouteModule, "/traceroute/"+host) + fmt.Sprintf("?client_id=%s&port=%d&max_ttl=%d&timeout=%d&protocol=%s", clientID, port, maxTTL, timeout, protocol)
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
