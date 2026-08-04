// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !linux

// Main package for the DogStatsD socket holder
package main

import (
	"fmt"
	"os"

	"github.com/DataDog/datadog-agent/pkg/dogstatsd/fdhandoff"
)

func main() {
	fmt.Fprintln(os.Stderr, fdhandoff.ErrUnsupported)
	os.Exit(1)
}
