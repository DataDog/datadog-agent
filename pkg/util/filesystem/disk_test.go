// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package filesystem

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDiskGetUsage(t *testing.T) {
	r := require.New(t)
	disk := NewDisk()
	usage, err := disk.GetUsage(".")
	r.NoError(err)
	r.GreaterOrEqual(usage.Total, usage.Available+usage.Used)
}
