// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build kubeapiserver

package redact

import (
	"testing"

	"github.com/stretchr/testify/assert"
	authenticationv1 "k8s.io/api/authentication/v1"
)

func TestScrubUserInfo(t *testing.T) {
	userInfo := &authenticationv1.UserInfo{
		Username: "alice",
		Extra: map[string]authenticationv1.ExtraValue{
			"api_key": {"sensitive-value", "other-value"},
			"team":    {"platform"},
		},
	}
	ScrubUserInfo(userInfo, NewDefaultDataScrubber())
	assert.Equal(t, "alice", userInfo.Username)
	assert.Equal(t, map[string]authenticationv1.ExtraValue{
		"api_key": {redactedSecret, redactedSecret},
		"team":    {"platform"},
	}, userInfo.Extra)
}
