// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package network

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

// GetDynamicTags converts a CertInfo struct into tags for the backend
func (ci *CertInfo) GetDynamicTags() map[string]struct{} {
	tags := make(map[string]struct{})

	if ci.SerialNumber != "" {
		tags["tls_cert_serial:"+ci.SerialNumber] = struct{}{}
	}
	if ci.Domain != "" {
		tags["tls_cert_domain:"+ci.Domain] = struct{}{}
	}

	if !ci.Validity.NotBefore.IsZero() {
		timestamp := ci.Validity.NotBefore.UTC().Format(time.RFC3339)
		tags["tls_cert_not_before:"+timestamp] = struct{}{}
	}
	if !ci.Validity.NotAfter.IsZero() {
		timestamp := ci.Validity.NotAfter.UTC().Format(time.RFC3339)
		tags["tls_cert_not_after:"+timestamp] = struct{}{}
	}

	return tags
}
