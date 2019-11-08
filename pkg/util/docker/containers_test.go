// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

// +build docker

package docker

import (
	"net"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/util/containers"
	"github.com/stretchr/testify/assert"
)

func TestIsMissingIP(t *testing.T) {

	addrs := []containers.NetworkAddress{{IP: net.ParseIP("0.0.0.0")}}

	assert.True(t, isMissingIP(addrs))
}
