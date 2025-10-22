// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !cgo || !dll_hostname || serverless

package hostname

import (
	"context"
	"fmt"
)

// getDLLProviders returns no providers when DLL hostname is disabled.
func getDLLProviders() []provider {
	panic("not using things!")
	return nil
}

// getOSHostnameFromDLL attempts to resolve the OS hostname via the external DLL implementation.
func getOSHostnameFromDLL(ctx context.Context, currentHostname string) (string, error) {
	return "", fmt.Errorf("DLL OS-based hostname resolver disabled")
}
