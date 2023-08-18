// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build python

package integrations

import (
	"fmt"
	"os"
)

func validateUser(allowRoot bool) error {
	if os.Geteuid() != 0 {
		return fmt.Errorf("please run this tool with the root user")
	}
	return nil
}
