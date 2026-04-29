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
		subjectTags = append(subjectTags, fmt.Sprintf("subject_%s:%s", getAttributeTypeName(subject.Type.String()), attrValueToString(subject.Value)))
	}

	log.Debugf("Subject tags: %v", subjectTags)
	return subjectTags
}

// attrValueToString renders an x509 DN attribute value as a string.
// pkix.AttributeTypeAndValue.Value is typed `any` because RFC 5280 permits
// several ASN.1 string encodings (PrintableString, UTF8String, BMPString,
// etc.).
func attrValueToString(v any) string {
	switch x := v.(type) {
	case string:
		return x
	case []byte:
		return string(x)
	case fmt.Stringer:
		return x.String()
	}
	if der, err := asn1.Marshal(v); err == nil {
		return "#" + hex.EncodeToString(der)
	}
	return fmt.Sprintf("%v", v)
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
		tags = append(tags, getSignatureAlgorithmTags(cert)...)
	}
	return tags
}

// resolveTemplateOIDName resolves a V2 template OID to its display name via
// the local CryptoAPI OID cache.
var resolveTemplateOIDName = func(oid string) string {
	name, err := winutil.CryptFindOIDInfo(winutil.CryptOIDInfoOIDKey, oid, winutil.CryptTemplateOIDGroupID)
	if err != nil {
		log.Debugf("CryptFindOIDInfo failed for template OID %s: %v", oid, err)
		return ""
	}
	return name
}

// getTemplateTags extracts the Microsoft certificate-template extensions and
// emits them under a split namespace so V1 and V2 never collide on the same
// tag key:
//
//   - certificate_template_name:<name>         V1 in-band name when present;
//     for V2-only certs, best-effort resolved via the local CryptoAPI OID
//     cache. Absent when V2's OID is not locally known (non-domain-joined
//     hosts, cross-forest certs, etc.).
//   - certificate_template_oid:<oid>           V2 only. Stable identifier
//     that does not change if a template is renamed in AD.
//   - certificate_template_major_version:<n>   V2 only.
//   - certificate_template_minor_version:<n>   V2 only.
func getTemplateTags(cert *x509.Certificate) []string {
	var tags []string
	var name string // value for certificate_template_name
	var v2Value []byte

	for i := range cert.Extensions {
		ext := &cert.Extensions[i]
		switch {
		case ext.Id.Equal(oidCertTemplateV1):
			var raw asn1.RawValue
			if _, err := asn1.Unmarshal(ext.Value, &raw); err != nil {
				log.Errorf("Error parsing certificate template V1 extension: %v", err)
				continue
			}
			if decoded := decodeBMPString(raw.Bytes); decoded != "" {
				name = decoded
			}
		case ext.Id.Equal(oidCertTemplateV2):
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
		} else {
			oid := t.TemplateID.String()
			tags = append(tags,
				"certificate_template_oid:"+oid,
				fmt.Sprintf("certificate_template_major_version:%d", t.MajorVersion),
				fmt.Sprintf("certificate_template_minor_version:%d", t.MinorVersion),
			)
			// Best-effort V2 name resolution; V1's in-band name takes
			// precedence when both extensions are present.
			if name == "" {
				if resolved := resolveTemplateOIDName(oid); resolved != "" {
					name = resolved
				}
			}
		}
	}

	if name != "" {
		tags = append(tags, "certificate_template_name:"+name)
	}
	return tags
}

// getEnhancedKeyUsageTags walks the Extended Key Usage extension (OID
// 2.5.29.37) and emits one tag per OID. Known OIDs are rendered with short
// names from extKeyUsageOIDToName; unknown OIDs are emitted as dotted strings.
func getEnhancedKeyUsageTags(cert *x509.Certificate) []string {
	tags := []string{}
	for i := range cert.Extensions {
		ext := &cert.Extensions[i]
		if !ext.Id.Equal(oidExtKeyUsage) {
			continue
		}
		var oids []asn1.ObjectIdentifier
		if _, err := asn1.Unmarshal(ext.Value, &oids); err != nil {
			log.Debugf("Error parsing Extended Key Usage extension: %v", err)
			return tags
		}
		for _, oid := range oids {
			tags = append(tags, "enhanced_key_usage:"+ekuName(oid.String()))
		}
		return tags
	}
	return tags
}

func ekuName(oid string) string {
	if name, ok := extKeyUsageOIDToName[oid]; ok {
		return name
	}
	return oid
}

// getSANTags returns tags for each Subject Alternative Name entry.
func getSANTags(cert *x509.Certificate) []string {
	tags := []string{}
	for _, dns := range cert.DNSNames {
		tags = append(tags, "subject_alt_name_dns:"+dns)
	}
	for _, ip := range cert.IPAddresses {
		tags = append(tags, "subject_alt_name_ip:"+ip.String())
	}
	for _, email := range cert.EmailAddresses {
		tags = append(tags, "subject_alt_name_email:"+email)
	}
	for _, uri := range cert.URIs {
		tags = append(tags, "subject_alt_name_uri:"+uri.String())
	}
	return tags
}

// getIssuerTags mirrors getSubjectTags for the certificate's issuer.
func getIssuerTags(cert *x509.Certificate) []string {
	tags := []string{}
	for _, attr := range cert.Issuer.Names {
		tags = append(tags, fmt.Sprintf("issuer_%s:%s", getAttributeTypeName(attr.Type.String()), attrValueToString(attr.Value)))
	}
	return tags
}

// getSignatureAlgorithmTags returns the certificate's signature algorithm as a
// single tag, e.g. "signature_algorithm:SHA256-RSA".
func getSignatureAlgorithmTags(cert *x509.Certificate) []string {
	return []string{"signature_algorithm:" + cert.SignatureAlgorithm.String()}
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
	return winutil.ConvertWindowsString(pvData), nil
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
