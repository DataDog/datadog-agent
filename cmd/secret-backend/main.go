// Unless explicitly stated otherwise all files in this repository are licensed
// under the BSD 3-Clause License.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.
// Copyright (c) 2021, RapDev.IO

// main package for the datadog-secret-backend
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"time"

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

	if inputPayload.Config == nil {
		inputPayload.Config = make(map[string]interface{})
	}

	backend := backend.Get(inputPayload.Type, inputPayload.Config)

	// extract timeout from secret_backend_timeout field (default 30 seconds)
	timeout := 30 * time.Second
	if inputPayload.SecretBackendTimeout != nil && *inputPayload.SecretBackendTimeout > 0 {
		timeout = time.Duration(*inputPayload.SecretBackendTimeout) * time.Second
	}

	secretOutputs := make(map[string]secret.Output, 0)
	for _, secretString := range inputPayload.Secrets {
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		secretOutputs[secretString] = backend.GetSecretOutput(ctx, secretString)
		cancel()
	}

	output, err := json.Marshal(secretOutputs)
	if err != nil {
		log.Fatalf("failed to marshal output: %s", err)
	}

	fmt.Print(string(output))
}
