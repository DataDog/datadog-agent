// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package auth provides an interface and implementations for signing and verifying requests send over HTTP (REST or GRPC).
package auth

import "io"

type header = map[string][]string

// type AuthError interface {
// 	StatusCode() int
// 	Error() string
// }

// type authError struct {
// 	statusCode int
// 	err        string
// }

// func (a *authError) StatusCode() int {
// 	return a.statusCode
// }

// func (a *authError) Error() string {
// 	return a.err
// }

// func newAuthError(err string, statusCode int) AuthError {
// 	return &authError{
// 		err:        err,
// 		statusCode: statusCode,
// 	}
// }

type statusCode = int

// The Authorizer interface is used to sign outgoing requests and verify incoming ones.
// It can be used for both pure HTTP requests and gRPC unary RPCs.
type Authorizer interface {
	// SignREST updates the provided reqHeaders in place using the provided values.
	SignREST(method string, reqHeaders header, body io.Reader, bodyLen int64) error

	// VerifyREST reads the content of the set authorization header and checks its authenticity against the provided values.
	VerifyREST(method string, reqHeaders header, body io.Reader, bodyLen int64) (statusCode, error)

	// SignGRPC updates the provided reqMetadata in place for gRPC unary RPCs.
	SignGRPC(fullMethod string, reqMetadata header) error

	// VerifyGRPC reads the content of the set authorization metadata and checks its authenticity for gRPC unary RPCs.
	VerifyGRPC(fullMethod string, reqMetadata header) error
}
