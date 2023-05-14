// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package host

import (
	"testing"

	"github.com/shirou/gopsutil/v3/host"
	"github.com/stretchr/testify/assert"
)

func TestFillOsVersion(t *testing.T) {
	stats := &systemStats{}
	info, _ := host.Info()
	fillOsVersion(stats, info)
	assert.Len(t, stats.Macver, 3)
	assert.NotEmpty(t, stats.Macver[0])
}
