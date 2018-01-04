// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package resources

import (
	"github.com/stretchr/testify/assert"
	"runtime"
	"testing"
)

func TestGetPayload(t *testing.T) {

	hostname := "foo"
	processesPayload := GetPayload(hostname)

	if runtime.GOOS == "windows" {
		// re-enable expected output, below when windows implements process metadata
		assert.Nil(t, processesPayload.Processes["snaps"])
		assert.Equal(t, "", processesPayload.Meta["host"])
	} else {
		assert.NotNil(t, processesPayload.Processes["snaps"])
		assert.Equal(t, hostname, processesPayload.Meta["host"])
	}
}
