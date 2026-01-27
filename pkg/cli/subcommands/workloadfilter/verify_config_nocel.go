// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !cel

package workloadfilterlist

import (
	"fmt"
	"io"

	"github.com/fatih/color"
)

// verifyCELConfig is a no-op when CEL support is not built in.
func verifyCELConfig(_ io.Writer, _ io.Reader) error {
	fmt.Fprintf(color.Output, "%s CEL validation is not available in this build.\n", color.HiYellowString("Warning"))
	return nil
}
