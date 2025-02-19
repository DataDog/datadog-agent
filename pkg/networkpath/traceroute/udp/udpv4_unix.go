// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build unix

package udp

import (
	"errors"

	"github.com/DataDog/datadog-agent/pkg/networkpath/traceroute/common"
)

// TracerouteSequential runs a traceroute
func (u *UDPv4) TracerouteSequential() (*common.Results, error) {
	return nil, errors.New("non-Dublin UDP not implemented for Unix")
}
