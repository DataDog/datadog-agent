// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux_bpf

package ssluprobes

import (
	"encoding/hex"
	"time"

	netebpf "github.com/DataDog/datadog-agent/pkg/network/ebpf"
)

// FromCertItem converts the ebpf CertItem struct into go CertInfo
func (ci *CertInfo) FromCertItem(certItem *netebpf.CertItem) {
	ci.SerialNumber = hex.EncodeToString(certItem.Serial.Data[:certItem.Serial.Len])
	ci.Domain = string(certItem.Domain.Data[:certItem.Domain.Len])

	ci.Validity.FromCertValidity(&certItem.Validity)
}

const derDateFormat = "060102150405"

// FromCertValidity converts the ebpf CertValidity struct into go CertValidity
func (cv *CertValidity) FromCertValidity(validity *netebpf.CertValidity) {
	cv.NotBefore, _ = time.Parse(derDateFormat, string(validity.Before[:]))
	cv.NotAfter, _ = time.Parse(derDateFormat, string(validity.After[:]))
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
		tags["tls_cert_not_before:"+timestamp] = struct{}{}
	}

	return tags
}
