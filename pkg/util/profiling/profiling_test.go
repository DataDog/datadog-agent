/// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package profiling

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestProfiling(t *testing.T) {
	err := Start(
		"fake-api",
		"https://nowhere.testing.dev",
		"testing",
		ProfileCoreService,
		time.Minute,
		15*time.Second,
		"1.0.0",
	)
	assert.Nil(t, err)

	Stop()
}
