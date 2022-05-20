// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package tag

import (
	"os"
	"testing"

	"gotest.tools/assert"
)

func TestGetBaseTagsNoEnv(t *testing.T) {
	assert.Equal(t, 0, len(GetBaseTags()))
}

func TestGetBaseTags(t *testing.T) {
	os.Setenv("K_SERVICE", "myService")
	defer os.Unsetenv("K_SERVICE")
	os.Setenv("K_REVISION", "FDGF34")
	defer os.Unsetenv("K_REVISION")
	tags := GetBaseTags()
	assert.Equal(t, 2, len(tags))
	assert.Equal(t, "revision:fdgf34", tags[0])
	assert.Equal(t, "service:myservice", tags[1])
}
