// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package com_datadoghq_postgresql

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"strings"

	log "github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/logging"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/libs/privateconnection"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
)

func extractInputsAndCredentialTokens[T any](task *types.Task, credential *privateconnection.PrivateCredentials) (T, map[string]string, error) {
	var inputs T
	inputs, err := types.ExtractInputs[T](task)
	if err != nil {
		return inputs, nil, err
	}
	credentialTokens := credential.AsTokenMap()
	return inputs, credentialTokens, nil
}

func openDB(credentialTokens map[string]string) (*sql.DB, error) {
	connectionString, err := getConnectionString(credentialTokens)
	if err != nil {
		return nil, err
	}
	err = validateConnectionParams(credentialTokens)
	if err != nil {
		return nil, err
	}
	db, err := sql.Open("postgres", connectionString)
	if err != nil {
		return nil, err
	}
	return db, nil
}

func getConnectionString(credentialTokens map[string]string) (string, error) {
	format := "host=%v port=%v user=%v password=%v dbname=%v sslmode=%v"
	connectionString := fmt.Sprintf(format, credentialTokens["host"], credentialTokens["port"], credentialTokens["user"], credentialTokens["password"], credentialTokens["database"], credentialTokens["sslmode"])
	if credentialTokens["applicationName"] != "" {
		connectionString = connectionString + fmt.Sprintf(" application_name=%v", credentialTokens["applicationName"])
	}
	if credentialTokens["searchPath"] != "" {
		connectionString = connectionString + fmt.Sprintf(" search_path=%v", credentialTokens["searchPath"])
	}
	return connectionString, nil
}

func validateConnectionParams(credentialTokens map[string]string) error {
	requiredParams := []string{"host", "port", "user", "password", "database", "sslmode"}
	missingParams := make([]string, 0, len(requiredParams))
	for _, param := range requiredParams {
		if _, ok := credentialTokens[param]; !ok {
			missingParams = append(missingParams, param)
		}
	}
	if len(missingParams) > 0 {
		return fmt.Errorf("missing required connection params: %v", missingParams)
	}
	if credentialTokens["sslmode"] != "require" && credentialTokens["sslmode"] != "disable" {
		return fmt.Errorf("invalid sslmode: '%v', supported modes are 'require' or 'disable'", credentialTokens["sslmode"])
	}
	return nil
}

func sanitizePGErrorMessage(err error) error {
	msg, _ := strings.CutPrefix(err.Error(), "pq: ")
	return errors.New(msg)
}

func closeSafely(ctx context.Context, resource string, c io.Closer) {
	if err := c.Close(); err != nil {
		log.FromContext(ctx).Errorf("Error closing %s: %v", resource, err)
	}
}

// normalizeSQL removes SQL comments and normalizes whitespace to prevent bypass attempts
func normalizeSQL(statement string) string {
	// Remove single-line comments
	lines := strings.Split(statement, "\n")
	cleanedLines := make([]string, 0, len(lines))
	for _, line := range lines {
		if idx := strings.Index(line, "--"); idx != -1 {
			line = line[:idx]
		}
		if trimmed := strings.TrimSpace(line); trimmed != "" {
			cleanedLines = append(cleanedLines, trimmed)
		}
	}
	result := strings.Join(cleanedLines, " ")

	// Remove multi-line comments
	for {
		start := strings.Index(result, "/*")
		if start == -1 {
			break
		}
		end := strings.Index(result[start:], "*/")
		if end == -1 {
			// Unclosed comment, remove everything after /*
			result = result[:start]
			break
		}
		result = result[:start] + " " + result[start+end+2:]
	}

	// Normalize whitespace and remove extra parentheses around keywords
	result = strings.TrimSpace(result)
	result = strings.Join(strings.Fields(result), " ")

	// Remove leading/trailing parentheses that might be used to bypass prefix checks
	for strings.HasPrefix(result, "(") && strings.HasSuffix(result, ")") {
		trimmed := strings.TrimSpace(result[1 : len(result)-1])
		if trimmed == "" {
			break
		}
		result = trimmed
	}

	return result
}
