// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package resources

import (
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetPayload(t *testing.T) {

	hostname := "foo"
	processesPayload := GetPayload(hostname)

	if runtime.GOOS == "windows" {
		// re-enable expected output, below when windows implements process metadata
		assert.Nil(t, processesPayload)
	} else {
		assert.NotNil(t, processesPayload.Processes["snaps"])
		assert.Equal(t, hostname, processesPayload.Meta["host"])
	}
}
