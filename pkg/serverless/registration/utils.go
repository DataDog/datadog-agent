// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package registration

import (
	"fmt"
	"net/http"
	"os"
)

// ID is the extension ID within the AWS Lambda environment.
type ID string

// FunctionARN is the ARN of the Lambda function
type FunctionARN string

// String returns the string value for this ID.
func (i ID) String() string {
	return string(i)
}

// HTTPClient represents an Http Client
type HTTPClient interface {
	Do(req *http.Request) (*http.Response, error)
}

// BuildURL returns the full url based on the route
func BuildURL(route string) string {
	prefix := os.Getenv("AWS_LAMBDA_RUNTIME_API")
	if len(prefix) == 0 {
		return fmt.Sprintf("http://localhost:9001%s", route)
	}
	return fmt.Sprintf("http://%s%s", prefix, route)
}
