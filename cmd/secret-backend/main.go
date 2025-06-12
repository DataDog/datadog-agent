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
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"github.com/DataDog/datadog-secret-backend/backend"
	"github.com/DataDog/datadog-secret-backend/secret"
)

const appVersion = "0.2.4"

func init() {
	zerolog.TimestampFunc = func() time.Time {
		return time.Now().UTC()
	}
	output := zerolog.ConsoleWriter{
		Out:        os.Stderr,
		TimeFormat: time.RFC3339,
		FormatLevel: func(i interface{}) string {
			return strings.ToUpper(fmt.Sprintf("| %-6s|", i))
		},
	}
	log.Logger = zerolog.New(output).With().Timestamp().Logger()
}

func main() {
	program, _ := os.Executable()
	programPath := filepath.Dir(program)
	defaultConfigFile := filepath.Join(programPath, "datadog-secret-backend.yaml")

	version := flag.Bool("version", false, "Print the version info")
	configFile := flag.String("config", defaultConfigFile, "Path to backend configuration yaml")

	flag.Parse()

	if *version {
		fmt.Fprintf(os.Stdout, "%s v%s\n", filepath.Base(program), appVersion)
		os.Exit(0)
	}

	input, err := io.ReadAll(os.Stdin)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to read from stdin")
	}

	inputPayload := &secret.Input{}
	if err := json.Unmarshal(input, inputPayload); err != nil {
		log.Fatal().Err(err).Msg("failed to unmarshal input")
	}

	backends := backend.NewBackends(*configFile)
	secretOutputs := backends.GetSecretOutputs(inputPayload.Secrets)

	output, err := json.Marshal(secretOutputs)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to marshal output")
	}

	fmt.Print(string(output))
}
