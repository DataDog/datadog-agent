// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018-present Datadog, Inc.

//go:build windows

package winutil

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetSidFromUser(t *testing.T) {
	sid, err := GetSidFromUser()
	t.Logf("The SID found was: %v", sid)
	assert.Nil(t, err)
	assert.NotNil(t, sid)
}
