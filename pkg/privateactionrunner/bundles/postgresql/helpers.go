// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package com_datadoghq_postgresql

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"strings"

	credssupport "github.com/DataDog/datadog-agent/pkg/privateactionrunner/bundle-support/credentials"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

func extractInputsAndCredentialTokens[T any](task *types.Task, credential interface{}) (T, map[string]string, error) {
	var inputs T
	inputs, err := types.ExtractInputs[T](task)
	if err != nil {
		return inputs, nil, err
	}
	credentialTokens, err := credssupport.ToTokensMap(credential)
	if err != nil {
		return inputs, nil, err
	}
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
	var missingParams []string
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

func sanitizePGErrorMessage(error error) error {
	return errors.New(strings.Replace(error.Error(), "pq: ", "", 1))
}

func closeSafely(ctx context.Context, resource string, c io.Closer) {
	if err := c.Close(); err != nil {
		log.Errorf("Error closing %s: %v", resource, err)
	}
}
