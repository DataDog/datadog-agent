// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux

package awsimds

import (
	"errors"
	"net"
	"net/http"
	"time"

	runnerdef "github.com/DataDog/datadog-agent/comp/healthplatform/runner/def"
	"github.com/DataDog/datadog-agent/pkg/config/env"
)

// requestTimeout is a var (not const) so tests can shorten it.
var requestTimeout = 2 * time.Second

// tokenTTLHeader and tokenTTLHeaderValue mirror the request the Agent's own IMDSv2
// client sends when resolving the EC2 hostname.
const (
	tokenTTLHeader      = "X-aws-ec2-metadata-token-ttl-seconds"
	tokenTTLHeaderValue = "21600"
)

// Check detects if the AWS IMDSv2 endpoint is unreachable from inside a container
// due to the default hop limit of 1. The check works by issuing the same token PUT
// request the Agent needs for IMDSv2: AWS's HttpPutResponseHopLimit setting only
// constrains the TTL of that request's *response*, not the TCP handshake, so the
// handshake always completes even when the hop limit is too low — only reading the
// response then hangs until it times out. A plain TCP dial can never observe this.
func Check() ([]runnerdef.IssueReport, error) {
	// Only relevant when running inside a container
	if !env.IsContainerized() {
		return nil, nil
	}

	req, err := http.NewRequest(http.MethodPut, "http://"+imdsAddress+"/latest/api/token", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set(tokenTTLHeader, tokenTTLHeaderValue)

	client := &http.Client{Timeout: requestTimeout}
	resp, err := client.Do(req)
	if err == nil {
		// Response received (regardless of status code) - IMDS is reachable, no hop limit issue
		resp.Body.Close()
		return nil, nil
	}

	// A timeout indicates the address is routable (i.e. we are on AWS) and the TCP
	// handshake succeeded, but the token response is being dropped before reaching
	// us - the classic hop limit symptom. Other errors (EHOSTUNREACH, ENETUNREACH,
	// ECONNREFUSED) mean IMDS is simply not present on this host, so we do not
	// report an issue.
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return []runnerdef.IssueReport{
			{
				IssueID:   IssueID,
				IssueName: IssueName,
				Context: map[string]string{
					"imds_address": imdsAddress,
				},
				Tags: []string{"aws", "imds", "hop-limit", "container"},
			},
		}, nil
	}

	return nil, nil
}
