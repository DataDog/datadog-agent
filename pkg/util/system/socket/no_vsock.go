// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build clusterchecks

package socket

import (
	"errors"
)

// ParseVSockAddress parses a vsock address and returns the CID
func ParseVSockAddress(_ string) (uint32, error) {
	return 0, errors.New("unsupported")
}
