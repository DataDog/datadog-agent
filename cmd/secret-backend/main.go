// Unless explicitly stated otherwise all files in this repository are licensed
// under the BSD 3-Clause License.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.
// Copyright (c) 2021, RapDev.IO

// main package for the datadog-secret-backend
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"

	"github.com/DataDog/datadog-secret-backend/backend"
	"github.com/DataDog/datadog-secret-backend/secret"
)

var appVersion = "dev"

func main() {
	program, _ := os.Executable()

	version := flag.Bool("version", false, "Print the version info")

	flag.Parse()

	if *version {
		fmt.Fprintf(os.Stdout, "%s %s\n", filepath.Base(program), appVersion)
		os.Exit(0)
	}

	input, err := io.ReadAll(os.Stdin)
	if err != nil {
		log.Fatalf("failed to read from stdin: %s", err)
	}

	inputPayload := &secret.Input{}
	if err := json.Unmarshal(input, inputPayload); err != nil {
		log.Fatalf("failed to unmarshal input: %s", err)
	}

	backend := &backend.GenericConnector{}
	if inputPayload.Config == nil {
		inputPayload.Config = make(map[string]interface{})
	}
	backend.InitBackend(inputPayload.Type, inputPayload.Config)
	secretOutputs := backend.GetSecretOutputs(inputPayload.Secrets)

	output, err := json.Marshal(secretOutputs)
	if err != nil {
		log.Fatalf("failed to marshal output: %s", err)
	}

	fmt.Print(string(output))
}
