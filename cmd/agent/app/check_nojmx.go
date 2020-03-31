// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build !jmx

package app

import "fmt"

// RunJmxListWithMetrics is not supported in the Puppy Agent because it doesn't
// support JMXFetch.
func RunJmxListWithMetrics() error {
	return fmt.Errorf("not supported in the Puppy Agent")
}

// RunJmxListWithRateMetrics is not supported in the Puppy Agent because it doesn't
// support JMXFetch.
func RunJmxListWithRateMetrics() error {
	return fmt.Errorf("not supported in the Puppy Agent")
}
