// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package payload

// TracerouteErrorCode is a classifiable error code for traceroute failures,
// aligned with the codes emitted by the system-probe traceroute module. The
// wire values must match github.com/DataDog/datadog-traceroute/traceroute
// ErrorCode constants so responses can be decoded on the agent side without
// importing the heavy traceroute library.
type TracerouteErrorCode string

const (
	// TracerouteErrCodeDNS indicates a DNS resolution failure.
	TracerouteErrCodeDNS TracerouteErrorCode = "DNS"
	// TracerouteErrCodeTimeout indicates the operation timed out.
	TracerouteErrCodeTimeout TracerouteErrorCode = "TIMEOUT"
	// TracerouteErrCodeConnRefused indicates the target actively refused the connection.
	TracerouteErrCodeConnRefused TracerouteErrorCode = "CONNREFUSED"
	// TracerouteErrCodeHostUnreach indicates the target host is unreachable.
	TracerouteErrCodeHostUnreach TracerouteErrorCode = "HOSTUNREACH"
	// TracerouteErrCodeNetUnreach indicates the target network is unreachable.
	TracerouteErrCodeNetUnreach TracerouteErrorCode = "NETUNREACH"
	// TracerouteErrCodeDenied indicates a permission error or unsupported configuration.
	TracerouteErrCodeDenied TracerouteErrorCode = "DENIED"
	// TracerouteErrCodeInvalidRequest indicates bad parameters from the caller.
	TracerouteErrCodeInvalidRequest TracerouteErrorCode = "INVALID_REQUEST"
	// TracerouteErrCodeFailedEncoding indicates a failure to encode the response.
	TracerouteErrCodeFailedEncoding TracerouteErrorCode = "FAILED_ENCODING"
	// TracerouteErrCodeUnknown is the catch-all for unclassified errors.
	TracerouteErrCodeUnknown TracerouteErrorCode = "UNKNOWN"
)

// TracerouteError is a classified error returned from the system-probe
// traceroute module. It is the agent-side mirror of the upstream library's
// TracerouteError type, kept local to avoid pulling the whole traceroute
// library into the main agent binary.
type TracerouteError struct {
	Code    TracerouteErrorCode
	Message string
}

// Error implements the error interface.
func (e *TracerouteError) Error() string {
	return e.Message
}

// TracerouteErrorResponse is the JSON body returned on error from the
// system-probe traceroute HTTP endpoint.
type TracerouteErrorResponse struct {
	Code    TracerouteErrorCode `json:"code"`
	Message string              `json:"message"`
}
