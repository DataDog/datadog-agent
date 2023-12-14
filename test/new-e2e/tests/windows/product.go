// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package windows

import (
	"fmt"
	"strings"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/e2e/client"
)

// GetProductCodeByName returns the product code GUID for the given product name
func GetProductCodeByName(client client.VM, name string) (string, error) {
	// TODO: Don't use Win32_Product
	cmd := fmt.Sprintf("(Get-WmiObject Win32_Product | Where-Object -Property Name -Value '%s' -match).IdentifyingNumber", name)
	val, err := client.ExecuteWithError(cmd)
	if err != nil {
		fmt.Println(val)
		return "", err
	}
	return strings.TrimSpace(val), nil
}
