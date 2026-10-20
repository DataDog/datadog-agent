// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package main is a helper binary for TestConfigHostname/external that returns
// a response string and exits with a given code, simulating an external hostname
// provider.
package main

import (
	"fmt"
	"os"
	"strconv"
)

func main() {
	fmt.Println(os.Getenv("DD_TEST_HOSTNAME_RESPONSE"))
	code, _ := strconv.Atoi(os.Getenv("DD_TEST_HOSTNAME_EXIT"))
	os.Exit(code)
}
