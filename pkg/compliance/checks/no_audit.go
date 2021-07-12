// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build !linux

package checks

import "errors"

func newAuditClient() (AuditClient, error) {
	return nil, errors.New("audit client requires linux build flag")
}
