// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package com_datadoghq_mongodb

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"

	"go.mongodb.org/mongo-driver/mongo/options"
	"go.mongodb.org/mongo-driver/x/mongo/driver/connstring"
)

func createMongoClientOptions(ctx context.Context, credentialTokens map[string]string) (*options.ClientOptions, *connstring.ConnString, error) {
	connectionUri, err := getConnectionUri(credentialTokens)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get connection string: %w", err)
	}

	cs, err := connstring.ParseAndValidate(connectionUri)
	if err != nil {
		return nil, nil, fmt.Errorf("unable to parse connection string: %w", err)
	}

	clientOptions := options.Client().ApplyURI(connectionUri)

	return clientOptions, cs, nil
}

func getConnectionUri(credentialTokens map[string]string) (string, error) {
	username, userOk := credentialTokens["username"]
	password, passOk := credentialTokens["password"]
	srvHost, srvHostOk := credentialTokens["srvHost"]
	host, hostOk := credentialTokens["host"]
	port, portOk := credentialTokens["port"]
	database := credentialTokens["database"]
	authSource := credentialTokens["authSource"]
	authMechanism := credentialTokens["authMechanism"]

	if !userOk || username == "" {
		return "", errors.New("invalid username: Username cannot be empty")
	}
	if !passOk || password == "" {
		return "", errors.New("invalid password: Password cannot be empty")
	}

	if srvHostOk {
		if srvHost == "" {
			return "", errors.New("invalid SRV host: SRV host cannot be empty")
		}
		return buildSRVConnectionURI(username, password, srvHost, database, authSource)
	}

	if !hostOk || host == "" {
		return "", errors.New("invalid host: Host cannot be empty")
	}
	if !portOk || port == "" {
		return "", errors.New("invalid port: Port cannot be empty")
	}

	return buildStandardConnectionURI(username, password, host, port, database, authSource, authMechanism)
}

func buildSRVConnectionURI(username, password, srvHost, database, authSource string) (string, error) {
	escapedUsername := url.QueryEscape(username)
	escapedPassword := url.QueryEscape(password)
	escapedSrvHost := url.QueryEscape(srvHost)
	escapedDatabase := url.QueryEscape(database)

	connectionUri := fmt.Sprintf("mongodb+srv://%v:%v@%v", escapedUsername, escapedPassword, escapedSrvHost)
	params := []string{}
	if database != "" {
		connectionUri = fmt.Sprintf("%s/%s", connectionUri, escapedDatabase)
	}
	if authSource != "" {
		params = append(params, "authSource="+url.QueryEscape(authSource))
	}
	if len(params) > 0 {
		connectionUri = fmt.Sprintf("%s?%s", connectionUri, strings.Join(params, "&"))
	}
	return connectionUri, nil
}

func buildStandardConnectionURI(username, password, host, port, database, authSource, authMechanism string) (string, error) {
	escapedUsername := url.QueryEscape(username)
	escapedPassword := url.QueryEscape(password)
	escapedHost := url.QueryEscape(host)
	escapedPort := url.QueryEscape(port)
	escapedDatabase := url.QueryEscape(database)

	connectionUri := fmt.Sprintf("mongodb://%v:%v@%v:%v", escapedUsername, escapedPassword, escapedHost, escapedPort)
	params := []string{}
	if database != "" {
		connectionUri = fmt.Sprintf("%s/%s", connectionUri, escapedDatabase)
	}
	if authSource != "" {
		params = append(params, "authSource="+url.QueryEscape(authSource))
	}
	if authMechanism != "" {
		params = append(params, "authMechanism="+url.QueryEscape(authMechanism))
	}
	if len(params) > 0 {
		connectionUri = fmt.Sprintf("%s?%s", connectionUri, strings.Join(params, "&"))
	}
	return connectionUri, nil
}

func ValidateFilter(filter map[string]any) error {
	return validateFilterWithDepth(filter, 0)
}

func validateFilterWithDepth(filter map[string]any, depth int) error {
	// Check depth limit to prevent stack overflow
	if depth > maxFilterDepth {
		return newFilterValidationError("Filter validation failed: maximum nesting depth exceeded")
	}

	for key, value := range filter {
		if strings.HasPrefix(key, "$") {
			if err := validateOperator(key, value, depth+1); err != nil {
				return err
			}
		} else if subFilter, ok := value.(map[string]any); ok {
			if err := validateFilterWithDepth(subFilter, depth+1); err != nil {
				return err
			}
		} else if !isValidValue(value) {
			return newFilterValidationError(fmt.Sprintf("Invalid value for field '%s' in filter.", key))
		}
	}
	return nil
}

func validateOperator(operator string, value any, depth int) error {
	if !isAllowedOperator(operator, allowedOperators) {
		return newFilterValidationError(fmt.Sprintf("Invalid operator in filter: '%s'.", operator))
	}
	if isLogicalOperator(operator) {
		return validateLogicalOperator(operator, value, depth)
	}
	if !isValidOperatorValue(operator, value) {
		return newFilterValidationError(fmt.Sprintf("Invalid value for operator '%s' in filter.", operator))
	}
	return nil
}

func validateLogicalOperator(operator string, value any, depth int) error {
	array, ok := value.([]any)
	if !ok || len(array) == 0 {
		return newFilterValidationError(fmt.Sprintf("Invalid value for logical operator '%s' in filter.", operator))
	}
	for _, item := range array {
		subFilter, ok := item.(map[string]any)
		if !ok {
			return newFilterValidationError(fmt.Sprintf("Invalid value for logical operator '%s' in filter.", operator))
		}
		if err := validateFilterWithDepth(subFilter, depth+1); err != nil {
			return err
		}
	}
	return nil
}

func newFilterValidationError(msg string) error {
	return fmt.Errorf("%s\n%s", msg, filterHelpMessage)
}

func isAllowedOperator(operator string, allowedOperators []string) bool {
	for _, op := range allowedOperators {
		if operator == op {
			return true
		}
	}
	return false
}

func isLogicalOperator(operator string) bool {
	return operator == "$and" || operator == "$or" || operator == "$nor"
}

func isValidOperatorValue(operator string, value any) bool {
	if operator == "$in" || operator == "$nin" {
		array, ok := value.([]any)
		if !ok || len(array) == 0 {
			return false
		}
		for _, item := range array {
			if !isValidPrimitiveValue(item) {
				return false
			}
		}
		return true
	}
	return isValidPrimitiveValue(value)
}

func isValidValue(value any) bool {
	switch value.(type) {
	case string, int, float64, bool, map[string]any:
		return true
	default:
		return false
	}
}

func isValidPrimitiveValue(value any) bool {
	switch value.(type) {
	case string, int, float64, bool:
		return true
	default:
		return false
	}
}
