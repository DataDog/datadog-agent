// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package npcollectorimpl

import (
	"regexp"
	"strings"

	model "github.com/DataDog/agent-payload/v5/process"
	"github.com/DataDog/datadog-agent/pkg/networkpath/payload"
)

func convertProtocol(connType model.ConnectionType) payload.Protocol {
	if connType == model.ConnectionType_tcp {
		return payload.ProtocolTCP
	} else if connType == model.ConnectionType_udp {
		return payload.ProtocolUDP
	}
	return ""
}

var awsElbRegex = regexp.MustCompile(`.*\.elb(\.[a-z0-9-]*)?\.amazonaws\.com`)

func shouldSkipDomain(domain string, ddSite string) bool {
	if strings.HasSuffix(domain, ".ec2.internal") {
		return true
	}
	if ddSite != "" && strings.HasSuffix(domain, "."+ddSite) {
		return true
	}
	if awsElbRegex.MatchString(domain) {
		return true
	}
	return false
}
