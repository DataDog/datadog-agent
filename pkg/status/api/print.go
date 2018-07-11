// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package api

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/fatih/color"

	"github.com/DataDog/datadog-agent/pkg/status/health"
)

// PrintHealth prints the health Status as human-readable text
func PrintHealth(s *health.Status, name string) error {
	sort.Strings(s.Unhealthy)
	sort.Strings(s.Healthy)

	statusString := color.GreenString("PASS")
	if len(s.Unhealthy) > 0 {
		statusString = color.RedString("FAIL")
	}
	fmt.Fprintln(color.Output, fmt.Sprintf("%s health: %s", name, statusString))

	if len(s.Healthy) > 0 {
		fmt.Fprintln(color.Output, fmt.Sprintf("=== %s healthy components ===", color.GreenString(strconv.Itoa(len(s.Healthy)))))
		fmt.Fprintln(color.Output, strings.Join(s.Healthy, ", "))
	}
	if len(s.Unhealthy) > 0 {
		fmt.Fprintln(color.Output, fmt.Sprintf("=== %s unhealthy components ===", color.RedString(strconv.Itoa(len(s.Unhealthy)))))
		fmt.Fprintln(color.Output, strings.Join(s.Unhealthy, ", "))
		return fmt.Errorf("found %d unhealthy components", len(s.Unhealthy))
	}

	return nil
}
