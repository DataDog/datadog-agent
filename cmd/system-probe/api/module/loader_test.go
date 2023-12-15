// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package module

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNameFromGRPCServiceName(t *testing.T) {
	assert.Equal(t, "", NameFromGRPCServiceName("a.b.c"))
	assert.Equal(t, "", NameFromGRPCServiceName("datadog.agent.systemprobe.asdf"))
	assert.Equal(t, "network_tracer", NameFromGRPCServiceName("datadog.agent.systemprobe.network_tracer.Usm"))
}
