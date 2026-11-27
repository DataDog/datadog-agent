// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build kubeapiserver

package redact

import (
	authenticationv1 "k8s.io/api/authentication/v1"
)

// ScrubUserInfo scrubs extra user attributes.
func ScrubUserInfo(u *authenticationv1.UserInfo, scrubber *DataScrubber) {
	for key, values := range u.Extra {
		if scrubber.ContainsSensitiveWord(key) {
			for i := range values {
				values[i] = redactedSecret
			}
		}
	}
}
