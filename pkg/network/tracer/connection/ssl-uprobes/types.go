// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package ssluprobes contains the attachment logic for TLS cert collecting uprobes
package ssluprobes

import (
	"time"
)

// CertValidity describes the time range that a certificate is valid for
type CertValidity struct {
	NotBefore time.Time
	NotAfter  time.Time
}

// CertInfo describes metadata about a TLS certificate
type CertInfo struct {
	SerialNumber string
	Domain       string

	Validity CertValidity
}
