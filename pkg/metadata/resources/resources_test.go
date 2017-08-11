// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

package resources

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestGetPayload(t *testing.T) {

	hostname := "foo"
	processesPayload := GetPayload(hostname)

	assert.NotNil(t, processesPayload.Processes["snaps"])
	assert.Equal(t, hostname, processesPayload.Meta["host"])
}
