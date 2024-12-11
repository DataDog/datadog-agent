// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package common

import (
	"encoding/json"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/components"
)

// AuthenticodeCertificate represents the certificate used to sign the file
type AuthenticodeCertificate struct {
	Subject    string `json:"Subject"`
	Issuer     string `json:"Issuer"`
	Thumbprint string `json:"Thumbprint"`
}

// AuthenticodeSignature is the result of GetAuthenticodeSignature()
type AuthenticodeSignature struct {
	SignerCertificate AuthenticodeCertificate `json:"SignerCertificate"`
	Status            int                     `json:"Status"`
	StatusMessage     string                  `json:"StatusMessage"`
}

// Valid returns true if the signature is valid.
func (s *AuthenticodeSignature) Valid() bool {
	return s.Status == 0
}

// GetAuthenticodeSignature returns the Authenticode signature of the file
// https://learn.microsoft.com/en-us/powershell/module/microsoft.powershell.security/get-authenticodesignature
func GetAuthenticodeSignature(host *components.RemoteHost, path string) (*AuthenticodeSignature, error) {
	cmd := "(Get-AuthenticodeSignature '" + path + "') | ConvertTo-Json"
	out, err := host.Execute(cmd)
	if err != nil {
		return nil, err
	}

	// Convert the JSON output to a struct
	signature := &AuthenticodeSignature{}
	err = json.Unmarshal([]byte(out), signature)
	if err != nil {
		return nil, err
	}

	return signature, nil
}
