// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018-2020 Datadog, Inc.

package main

import (
	"fmt"
	"os"
)

func main() {
	fmt.Printf("{\"handle1\":{\"value\":\"arg_password\"}}")
	os.Exit(1)
}
