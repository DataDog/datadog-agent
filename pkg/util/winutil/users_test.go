// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018-present Datadog, Inc.

// +build windows

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

func TestGetUserFromSid(t *testing.T) {
	sid, err := GetSidFromUser()
	assert.Nil(t, err)
	assert.NotNil(t, sid)

	username, domain, err := GetUserFromSid(sid)
	assert.Nil(t, err)

	t.Logf("username: %v\tdomain: %v", username, domain)
	assert.NotNil(t, username)
	assert.NotNil(t, domain)
	assert.NotEqual(t, "", username)
	assert.NotEqual(t, "", domain)
}
