// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

package packaging

import (
	"fmt"
)

func linkRead(linkPath string) (string, error) {
	return "", fmt.Errorf("not supported on windows")
}

func linkExists(linkPath string) (bool, error) {
	return false, fmt.Errorf("not supported on windows")
}

func linkSet(linkPath string, targetPath string) error {
	return fmt.Errorf("not supported on windows")
}

func linkDelete(linkPath string) error {
	return fmt.Errorf("not supported on windows")
}
