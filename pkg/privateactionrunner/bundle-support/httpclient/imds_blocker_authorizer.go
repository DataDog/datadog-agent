// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package httpclient

import (
	"fmt"
	"net"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/httpclient"
)

// block IMDS endpoints (AWS doc https://docs.aws.amazon.com/AWSEC2/latest/UserGuide/configuring-instance-metadata-service.html#instance-metadata-v2-how-it-works)
var blockedIps = []net.IP{
	net.ParseIP("169.254.169.254"),
	net.ParseIP("fd00:ec2::254"),
}

type parAuthorizer struct{}

func newIMDSBlockerAuthorizer() httpclient.Authorizer {
	return &parAuthorizer{}
}

func (p parAuthorizer) IsNetworkAddressAuthorized(network, address string) (bool, error) {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return false, fmt.Errorf("failed to split host and port: %w", err)
	}
	parsedId := net.ParseIP(host)
	if parsedId == nil {
		return false, fmt.Errorf("failed to parse ip address: %s", address)
	}
	for _, ip := range blockedIps {
		if ip.Equal(parsedId) {
			return false, fmt.Errorf("ip address %s is blocked", address)
		}
	}
	return true, nil
}
