// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build windows

// Package windowscertificate implements a windows certificate check
package windowscertificate

import (
	"crypto/x509"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"golang.org/x/sys/windows"

	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/winutil"
)

func getExpiration(expirationDate time.Time) float64 {
	daysRemaining := time.Until(expirationDate).Hours() / 24
	return float64(daysRemaining)
}

func getSubjectTags(cert *x509.Certificate) []string {
	subjectTags := []string{}
	subjectAttributes := cert.Subject.Names

	for _, subject := range subjectAttributes {
		subjectTags = append(subjectTags, fmt.Sprintf("subject_%s:%s", getAttributeTypeName(subject.Type.String()), subject.Value))
	}

	log.Debugf("Subject tags: %v", subjectTags)
	return subjectTags
}

// getCrlIssuerTags returns the tags for the CRL issuer
// For example: [L=Internet, O="VeriSign, Inc.", OU=VeriSign Commercial Software Publishers CA]
// will become: [crl_issuer_L:Internet, crl_issuer_O:VeriSign, Inc., crl_issuer_OU:VeriSign Commercial Software Publishers CA]
func getCrlIssuerTags(issuer string) []string {
	issuerTags := []string{}

	if issuer == "" {
		return issuerTags
	}

	issuerRDNs := strings.Split(issuer, "\r\n")
	for _, rdn := range issuerRDNs {
		rdn = strings.TrimSpace(rdn)
		if rdn == "" {
			continue
		}

		// Split by "=" to get key and value
		keyValue := strings.SplitN(rdn, "=", 2)
		if len(keyValue) == 2 {
			key := strings.TrimSpace(keyValue[0])
			value := strings.TrimSpace(keyValue[1])

			// Remove quotes if present
			if strings.HasPrefix(value, "\"") && strings.HasSuffix(value, "\"") {
				value = strings.Trim(value, "\"")
			}

			// Format: "crl_issuer_<key>:<value>"
			component := fmt.Sprintf("crl_issuer_%s:%s", key, value)
			issuerTags = append(issuerTags, component)
		}
	}
	log.Debugf("CRL issuer tags: %v", issuerTags)

	return issuerTags
}

// getCertThumbprint returns the thumbprint of a certificate
// In Windows the thumbprint is the sha1 hash of the certificate's raw bytes
//
// https://learn.microsoft.com/en-us/windows/win32/seccrypto/certificate-thumbprint
func getCertThumbprint(certContext *windows.CertContext) (string, error) {
	var pcbData uint32

	err := winutil.CertGetCertificateContextProperty(certContext, certHashPropID, nil, &pcbData)
	if err != nil {
		return "", err
	}
	if pcbData == 0 {
		return "", errors.New("certificate has no SHA-1 Thumbprint")
	}

	pvData := make([]byte, pcbData)
	err = winutil.CertGetCertificateContextProperty(certContext, certHashPropID, &pvData[0], &pcbData)
	if err != nil {
		return "", err
	}

	return hex.EncodeToString(pvData[:]), nil
}

// getCrlThumbprint returns the thumbprint of a CRL
// In Windows the thumbprint is the sha1 hash of the CRL's raw bytes
func getCrlThumbprint(pCrlContext *winutil.CRLContext) (string, error) {
	var pcbData uint32

	err := winutil.CertGetCRLContextProperty(pCrlContext, certHashPropID, nil, &pcbData)
	if err != nil {
		return "", err
	}
	if pcbData == 0 {
		return "", errors.New("CRL has no SHA-1 Thumbprint")
	}

	pvData := make([]byte, pcbData)
	err = winutil.CertGetCRLContextProperty(pCrlContext, certHashPropID, &pvData[0], &pcbData)
	if err != nil {
		return "", err
	}

	return hex.EncodeToString(pvData[:]), nil
}

func convertCertNameBlobToString(nameBlob *windows.CertNameBlob) (string, error) {
	if nameBlob == nil || nameBlob.Size == 0 {
		return "", nil
	}
	dwStrType := uint32(certX500NameStr | certNameStrCRLF)

	var strBuffer []uint16
	_, bufferSize, err := winutil.CertNameToStrW(windows.X509_ASN_ENCODING, nameBlob, dwStrType, nil, 0)

	if err != nil {
		return "", err
	}

	strBuffer = make([]uint16, bufferSize)
	certNameStr, _, err := winutil.CertNameToStrW(windows.X509_ASN_ENCODING, nameBlob, dwStrType, strBuffer, bufferSize)
	if err != nil {
		return "", err
	}
	return certNameStr, nil
}

// Returns the human-readable name for a given OID string
func getAttributeTypeName(oid string) string {
	switch oid {
	case "2.5.4.6":
		return "C"
	case "2.5.4.10":
		return "O"
	case "2.5.4.11":
		return "OU"
	case "2.5.4.3":
		return "CN"
	case "2.5.4.5":
		return "SERIALNUMBER"
	case "2.5.4.7":
		return "L"
	case "2.5.4.8":
		return "ST"
	case "2.5.4.9":
		return "STREET"
	case "2.5.4.17":
		return "POSTALCODE"
	case "2.5.4.12":
		return "T"
	case "2.5.4.42":
		return "GN"
	case "2.5.4.4":
		return "SN"
	case "2.5.4.46":
		return "DNQ"
	case "0.9.2342.19200300.100.1.1":
		return "UID"
	case "1.2.840.113549.1.9.1":
		return "E"
	case "0.9.2342.19200300.100.1.25":
		return "DC"
	default:
		return oid
	}
}
