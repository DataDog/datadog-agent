// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package transaction

// Method is the type used to represent HTTP request methods
type Method int

const (
	// MethodUnknown represents an unknown request method
	MethodUnknown Method = iota
	// MethodGet represents the GET request method
	MethodGet
	// MethodPost represents the POST request method
	MethodPost
	// MethodPut represents the PUT request method
	MethodPut
	// MethodDelete represents the DELETE request method
	MethodDelete
	// MethodHead represents the HEAD request method
	MethodHead
	// MethodOptions represents the OPTIONS request method
	MethodOptions
	// MethodPatch represents the PATCH request method
	MethodPatch
	// MethodMaximum represents the end of the list of methods
	MethodMaximum
)

// Method returns a string representing the HTTP method of the request
func (m Method) String() string {
	switch m {
	case MethodGet:
		return "GET"
	case MethodPost:
		return "POST"
	case MethodPut:
		return "PUT"
	case MethodHead:
		return "HEAD"
	case MethodDelete:
		return "DELETE"
	case MethodOptions:
		return "OPTIONS"
	case MethodPatch:
		return "PATCH"
	default:
		return "UNKNOWN"
	}
}
