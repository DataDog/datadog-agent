// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build windows

// Package windowscertificate implements a windows certificate check
package windowscertificate

import (
	"crypto/x509"
	"encoding/asn1"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf16"

	"golang.org/x/sys/windows"

	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/winutil"
)

// extKeyUsageOIDToName maps EKU OIDs to short names.
// RFC 5280 §4.2.1.12: https://datatracker.ietf.org/doc/html/rfc5280#section-4.2.1.12
// Microsoft: https://learn.microsoft.com/en-us/windows/win32/api/certenroll/nn-certenroll-ix509extensionenhancedkeyusage
var extKeyUsageOIDToName = map[string]string{
	"2.5.29.37.0":             "anyExtendedKeyUsage",
	"1.3.6.1.5.5.7.3.1":       "serverAuth",
	"1.3.6.1.5.5.7.3.2":       "clientAuth",
	"1.3.6.1.5.5.7.3.3":       "codeSigning",
	"1.3.6.1.5.5.7.3.4":       "emailProtection",
	"1.3.6.1.5.5.7.3.5":       "ipsecEndSystem",
	"1.3.6.1.5.5.7.3.6":       "ipsecTunnel",
	"1.3.6.1.5.5.7.3.7":       "ipsecUser",
	"1.3.6.1.5.5.7.3.8":       "timeStamping",
	"1.3.6.1.5.5.7.3.9":       "ocspSigning",
	"1.3.6.1.4.1.311.10.3.3":  "microsoftServerGatedCrypto",
	"2.16.840.1.113730.4.1":   "netscapeServerGatedCrypto",
	"1.3.6.1.4.1.311.10.3.1":  "microsoftCertTrustListSigning",
	"1.3.6.1.4.1.311.10.3.4":  "microsoftEncryptedFileSystem",
	"1.3.6.1.4.1.311.10.3.12": "microsoftDocumentSigning",
	"1.3.6.1.4.1.311.20.2.1":  "microsoftCertificateRequestAgent",
	"1.3.6.1.4.1.311.20.2.2":  "microsoftSmartcardLogon",
	"1.3.6.1.4.1.311.21.5":    "microsoftCAEncryption",
	"1.3.6.1.4.1.311.21.6":    "microsoftKeyRecovery",
	"1.3.6.1.4.1.311.54.1.2":  "microsoftRootListSigner",
	"1.3.6.1.4.1.311.61.1.1":  "microsoftKernelCodeSigning",
	"1.3.6.1.4.1.311.61.4.1":  "microsoftEarlyLaunchAntimalware",
}

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

// getCertContextProperty reads an extended property of a certificate context
// using the standard two-phase Windows pattern: first call to learn the
// buffer size, second call to copy the bytes. Returns a nil slice (no error)
// when the property is not set on the certificate.
func getCertContextProperty(certContext *windows.CertContext, propID uint32) ([]byte, error) {
	var pcbData uint32

	err := winutil.CertGetCertificateContextProperty(certContext, propID, nil, &pcbData)
	if err != nil {
		if err == cryptENotFound {
			return nil, nil
		}
		return nil, err
	}
	if pcbData == 0 {
		return nil, nil
	}

	pvData := make([]byte, pcbData)
	err = winutil.CertGetCertificateContextProperty(certContext, propID, &pvData[0], &pcbData)
	if err != nil {
		return nil, err
	}
	return pvData, nil
}

// getCertThumbprint returns the thumbprint of a certificate
// In Windows the thumbprint is the sha1 hash of the certificate's raw bytes
//
// https://learn.microsoft.com/en-us/windows/win32/seccrypto/certificate-thumbprint
func getCertThumbprint(certContext *windows.CertContext) (string, error) {
	pvData, err := getCertContextProperty(certContext, certHashPropID)
	if err != nil {
		return "", err
	}
	if len(pvData) == 0 {
		return "", errors.New("certificate has no SHA-1 Thumbprint")
	}
	return hex.EncodeToString(pvData), nil
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

// appendOptionalTags appends each optional tag group onto the base tag slice,
// gated by the corresponding config flag.
func appendOptionalTags(tags []string, cert *x509.Certificate, friendlyName string, cfg Config) []string {
	if cfg.CertificateTemplateTag {
		tags = append(tags, getTemplateTags(cert)...)
	}
	if cfg.EnhancedKeyUsageTag {
		tags = append(tags, getEnhancedKeyUsageTags(cert)...)
	}
	if cfg.FriendlyNameTag && friendlyName != "" {
		tags = append(tags, "friendly_name:"+friendlyName)
	}
	if cfg.SubjectAlternativeNamesTag {
		tags = append(tags, getSANTags(cert)...)
	}
	if cfg.IssuerTag {
		tags = append(tags, getIssuerTags(cert)...)
	}
	if cfg.SignatureAlgorithmTag {
		tags = append(tags, getSignatureHashTags(cert)...)
	}
	return tags
}

// getTemplateTags extracts the Microsoft certificate-template extension. V1
// (OID 1.3.6.1.4.1.311.20.2) is preferred when both are present because it
// carries the human-readable template name; V2 carries the template OID and
// version numbers.
func getTemplateTags(cert *x509.Certificate) []string {
	var v2Value []byte
	for i := range cert.Extensions {
		ext := &cert.Extensions[i]
		if ext.Id.Equal(oidCertTemplateV1) {
			var name asn1.RawValue
			if _, err := asn1.Unmarshal(ext.Value, &name); err != nil {
				log.Debugf("Error parsing certificate template V1 extension: %v", err)
				continue
			}
			decoded := decodeBMPString(name.Bytes)
			if decoded == "" {
				continue
			}
			return []string{"certificate_template:" + decoded}
		}
		if ext.Id.Equal(oidCertTemplateV2) {
			v2Value = ext.Value
		}
	}
	if v2Value != nil {
		var t struct {
			TemplateID   asn1.ObjectIdentifier
			MajorVersion int `asn1:"optional"`
			MinorVersion int `asn1:"optional"`
		}
		if _, err := asn1.Unmarshal(v2Value, &t); err != nil {
			log.Debugf("Error parsing certificate template V2 extension: %v", err)
			return []string{}
		}
		return []string{
			"certificate_template:" + t.TemplateID.String(),
			fmt.Sprintf("certificate_template_major_version:%d", t.MajorVersion),
			fmt.Sprintf("certificate_template_minor_version:%d", t.MinorVersion),
		}
	}
	return []string{}
}

// getEnhancedKeyUsageTags returns tags for each OID in the EKU extension. Known
// OIDs are rendered with their short names, unknown OIDs as dotted strings.
func getEnhancedKeyUsageTags(cert *x509.Certificate) []string {
	tags := []string{}
	for _, oid := range cert.UnknownExtKeyUsage {
		tags = append(tags, "enhanced_key_usage:"+ekuName(oid.String()))
	}
	for _, eku := range cert.ExtKeyUsage {
		if oid, ok := ekuOIDFromStdlib(eku); ok {
			tags = append(tags, "enhanced_key_usage:"+ekuName(oid))
		}
	}
	return tags
}

func ekuName(oid string) string {
	if name, ok := extKeyUsageOIDToName[oid]; ok {
		return name
	}
	return oid
}

// ekuOIDFromStdlib maps x509.ExtKeyUsage constants back to OID strings. Keep
// in sync with Go's unexported extKeyUsageOIDs table.
// https://cs.opensource.google/go/go/+/refs/tags/go1.24.0:src/crypto/x509/x509.go;l=565
func ekuOIDFromStdlib(eku x509.ExtKeyUsage) (string, bool) {
	switch eku {
	case x509.ExtKeyUsageAny:
		return "2.5.29.37.0", true
	case x509.ExtKeyUsageServerAuth:
		return "1.3.6.1.5.5.7.3.1", true
	case x509.ExtKeyUsageClientAuth:
		return "1.3.6.1.5.5.7.3.2", true
	case x509.ExtKeyUsageCodeSigning:
		return "1.3.6.1.5.5.7.3.3", true
	case x509.ExtKeyUsageEmailProtection:
		return "1.3.6.1.5.5.7.3.4", true
	case x509.ExtKeyUsageIPSECEndSystem:
		return "1.3.6.1.5.5.7.3.5", true
	case x509.ExtKeyUsageIPSECTunnel:
		return "1.3.6.1.5.5.7.3.6", true
	case x509.ExtKeyUsageIPSECUser:
		return "1.3.6.1.5.5.7.3.7", true
	case x509.ExtKeyUsageTimeStamping:
		return "1.3.6.1.5.5.7.3.8", true
	case x509.ExtKeyUsageOCSPSigning:
		return "1.3.6.1.5.5.7.3.9", true
	case x509.ExtKeyUsageMicrosoftServerGatedCrypto:
		return "1.3.6.1.4.1.311.10.3.3", true
	case x509.ExtKeyUsageNetscapeServerGatedCrypto:
		return "2.16.840.1.113730.4.1", true
	case x509.ExtKeyUsageMicrosoftCommercialCodeSigning:
		return "1.3.6.1.4.1.311.2.1.22", true
	case x509.ExtKeyUsageMicrosoftKernelCodeSigning:
		return "1.3.6.1.4.1.311.61.1.1", true
	}
	return "", false
}

// getSANTags returns tags for each Subject Alternative Name entry.
func getSANTags(cert *x509.Certificate) []string {
	tags := []string{}
	for _, dns := range cert.DNSNames {
		tags = append(tags, "san_dns:"+dns)
	}
	for _, ip := range cert.IPAddresses {
		tags = append(tags, "san_ip:"+ip.String())
	}
	for _, email := range cert.EmailAddresses {
		tags = append(tags, "san_email:"+email)
	}
	for _, uri := range cert.URIs {
		tags = append(tags, "san_uri:"+uri.String())
	}
	return tags
}

// getIssuerTags mirrors getSubjectTags for the certificate's issuer.
func getIssuerTags(cert *x509.Certificate) []string {
	tags := []string{}
	for _, attr := range cert.Issuer.Names {
		tags = append(tags, fmt.Sprintf("issuer_%s:%s", getAttributeTypeName(attr.Type.String()), attr.Value))
	}
	return tags
}

// getSignatureHashTags splits a SignatureAlgorithm string like "SHA256-RSA"
// into separate algorithm and hash tags. Falls back to a single tag on
// unexpected formats.
func getSignatureHashTags(cert *x509.Certificate) []string {
	sig := cert.SignatureAlgorithm.String()
	if before, after, ok := strings.Cut(sig, "-"); ok && before != "" && after != "" {
		return []string{
			"signature_algorithm:" + after,
			"signature_hash_algorithm:" + before,
		}
	}
	return []string{"signature_algorithm:" + sig}
}

// getFriendlyName reads the friendly name property from a certificate context.
// Windows stores friendly names as UTF-16LE, NUL-terminated.
func getFriendlyName(certContext *windows.CertContext) (string, error) {
	pvData, err := getCertContextProperty(certContext, certFriendlyNamePropID)
	if err != nil {
		return "", err
	}
	if len(pvData) == 0 {
		return "", nil
	}
	return decodeUTF16LE(pvData), nil
}

// decodeBMPString decodes a BMPString (UTF-16 big-endian, UCS-2).
func decodeBMPString(b []byte) string {
	if len(b)%2 != 0 {
		return ""
	}
	u16 := make([]uint16, len(b)/2)
	for i := 0; i < len(u16); i++ {
		u16[i] = uint16(b[2*i])<<8 | uint16(b[2*i+1])
	}
	return strings.TrimRight(string(utf16.Decode(u16)), "\x00")
}

// decodeUTF16LE decodes UTF-16 little-endian bytes (Windows-native string
// representation) and strips any trailing NUL terminator.
func decodeUTF16LE(b []byte) string {
	if len(b)%2 != 0 {
		b = b[:len(b)-1]
	}
	u16 := make([]uint16, len(b)/2)
	for i := 0; i < len(u16); i++ {
		u16[i] = uint16(b[2*i]) | uint16(b[2*i+1])<<8
	}
	return strings.TrimRight(string(utf16.Decode(u16)), "\x00")
}
