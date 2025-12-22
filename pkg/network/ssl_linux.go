// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux_bpf

package network

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
