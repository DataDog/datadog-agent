// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package expvars

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTimestamp(t *testing.T) {
	loc, err := time.LoadLocation("America/New_York")
	require.Nil(t, err)

	unixTimeUTC := time.Unix(1628195974, 0).In(loc)
	assert.Equal(t, "\"2021-08-05T16:39:34-04:00\"", timestamp(unixTimeUTC).String())

	unixTimeUTC = time.Unix(1234567890, 0).In(loc)
	assert.Equal(t, "\"2009-02-13T18:31:30-05:00\"", timestamp(unixTimeUTC).String())
}
