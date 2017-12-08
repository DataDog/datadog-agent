// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

package utils

import (
	"fmt"
	"log"
)

// logWriter conforms to Writer to change logs format
type logWriter struct {
}

// Write only returns the input with no additional information
func (w logWriter) Write(bytes []byte) (int, error) {
	return fmt.Print(string(bytes))
}

// SetupLogger overrides go built-in logger to change logs format
func SetupLogger() {
	log.SetFlags(0)
	log.SetOutput(new(logWriter))
}
