// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018-present Datadog, Inc.

// Package main defines the main function
package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"time"
)

func main() {
	secretHandle := "handle1"
	secretValue := "arg_password"
	if len(os.Args) == 2 {
		subcommand := os.Args[1]
		switch subcommand {
		case "response_too_long":
			// 50 characters
			fmt.Printf("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
			os.Exit(0)
		case "timeout":
			// Take to long to respond
			time.Sleep(2 * time.Second)
		case "error":
			// Write to stdout but also exit code is non-zero
			fmt.Printf("{\"" + secretHandle + "\":{\"value\":\"" + secretValue + "\"}}")
			os.Exit(1)
		}
	} else {
		// Default: read handle from stdin, assert that version is 1.0
		reader := bufio.NewReader(os.Stdin)
		text, _ := reader.ReadString('\n')
		var obj map[string]interface{}
		_ = json.Unmarshal([]byte(text), &obj)
		if obj["version"] != "1.0" {
			fmt.Fprintf(os.Stderr, "invalid version, expected 1.0")
			os.Exit(1)
		}
		secretHandle = obj["secrets"].([]interface{})[0].(string)
	}
	fmt.Printf("{\"" + secretHandle + "\":{\"value\":\"" + secretValue + "\"}}")
}
