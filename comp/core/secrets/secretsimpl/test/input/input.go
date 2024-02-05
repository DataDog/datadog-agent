// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018-present Datadog, Inc.

// Package main defines the main function
package main

import (
	"bufio"
	"fmt"
	"os"
)

func main() {
	reader := bufio.NewReader(os.Stdin)
	text, _ := reader.ReadString('\n')
	if text != "{\"version\": \"1.0\" , \"secrets\": [\"sec1\", \"sec2\"]}" {
		os.Exit(1)
	}
	fmt.Printf("{\"handle1\":{\"value\":\"input_password\"}}")
}
