// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package registration

import (
	"fmt"
	"net/http"
)

// ID is the extension ID within the AWS Extension environment.
type ID string

// String returns the string value for this ID.
func (i ID) String() string {
	return string(i)
}

// HTTPClient represents an Http Client
type HTTPClient interface {
	Do(req *http.Request) (*http.Response, error)
}

// BuildURL builds and URL with a prefix and a route
func BuildURL(prefix string, route string) string {
	if len(prefix) == 0 {
		return fmt.Sprintf("http://localhost:9001%s", route)
	}
	return fmt.Sprintf("http://%s%s", prefix, route)
}
