// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// This program is used to test the retrieval of the hostname from an external
// binary. It prints TRACE_TEST_HOSTNAME_RESPONSE and exits with
// TRACE_TEST_HOSTNAME_EXIT_CODE, so a single compiled binary can be reused
// across TestConfigHostname/external's subtests instead of recompiling one
// per subtest.

package main

import (
	"fmt"
	"os"
	"strconv"
)

func main() {
	fmt.Println(os.Getenv("TRACE_TEST_HOSTNAME_RESPONSE"))
	code, _ := strconv.Atoi(os.Getenv("TRACE_TEST_HOSTNAME_EXIT_CODE"))
	os.Exit(code)
}
