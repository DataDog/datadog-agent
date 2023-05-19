// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

package bytecode

import (
	"fmt"
)

// VerifyAssetPermissions is for verifying the permissions of bpf programs
func VerifyAssetPermissions(assetPath string) error {
	return fmt.Errorf("verification of bpf assets is not supported on windows")
}
