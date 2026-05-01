// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build windows

package windowscertificate

import (
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"net"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// encodeEKUExtension ASN.1-marshals a SEQUENCE OF OID as the Extended Key Usage
// extension carries (RFC 5280 §4.2.1.12).
func encodeEKUExtension(oids ...asn1.ObjectIdentifier) []byte {
	b, err := asn1.Marshal(oids)
	if err != nil {
		panic(err)
	}
	return b
}

func TestDecodeBMPString(t *testing.T) {
	// "Hi" encoded as BMPString (UTF-16BE, UCS-2): 0x00 'H' 0x00 'i'
	b := []byte{0x00, 'H', 0x00, 'i'}
	require.Equal(t, "Hi", decodeBMPString(b))

	// Empty input
	require.Equal(t, "", decodeBMPString(nil))

	// Odd-length input is invalid BMPString
	require.Equal(t, "", decodeBMPString([]byte{0x00}))

	// Trailing NUL should be trimmed
	require.Equal(t, "Hi", decodeBMPString([]byte{0x00, 'H', 0x00, 'i', 0x00, 0x00}))
}

func encodeBMPString(s string) []byte {
	out := make([]byte, 0, len(s)*2)
	for _, r := range s {
		out = append(out, byte(r>>8), byte(r))
	}
	// Wrap as OCTET STRING-ish ASN.1 RawValue so unmarshal in getTemplateTags works.
	raw := asn1.RawValue{
		Class: asn1.ClassUniversal,
		Tag:   asn1.TagBMPString,
		Bytes: out,
	}
	bytes, err := asn1.Marshal(raw)
	if err != nil {
		panic(err)
	}
	return bytes
}

func encodeTemplateV2(oid asn1.ObjectIdentifier, major, minor int) []byte {
	bytes, err := asn1.Marshal(struct {
		TemplateID   asn1.ObjectIdentifier
		MajorVersion int `asn1:"optional"`
		MinorVersion int `asn1:"optional"`
	}{oid, major, minor})
	if err != nil {
		panic(err)
	}
	return bytes
}

// stubTemplateOIDResolver swaps resolveTemplateOIDName for the duration of a
// test. Returns a restore func the test should defer.
func stubTemplateOIDResolver(fn func(oid string) string) func() {
	original := resolveTemplateOIDName
	resolveTemplateOIDName = fn
	return func() { resolveTemplateOIDName = original }
}

func TestGetTemplateTagsV1(t *testing.T) {
	defer stubTemplateOIDResolver(func(string) string { return "" })()
	cert := &x509.Certificate{
		Extensions: []pkix.Extension{
			{Id: oidCertTemplateV1, Value: encodeBMPString("WebServer")},
		},
	}
	require.Equal(t, []string{"certificate_template_name:WebServer"}, getTemplateTags(cert))
}

func TestGetTemplateTagsV2(t *testing.T) {
	// Resolver miss: V2 emits OID + versions but no name tag.
	defer stubTemplateOIDResolver(func(string) string { return "" })()
	templateID := asn1.ObjectIdentifier{1, 3, 6, 1, 4, 1, 311, 21, 8, 1, 2, 3}
	cert := &x509.Certificate{
		Extensions: []pkix.Extension{
			{Id: oidCertTemplateV2, Value: encodeTemplateV2(templateID, 100, 0)},
		},
	}
	tags := getTemplateTags(cert)
	require.ElementsMatch(t, []string{
		"certificate_template_oid:" + templateID.String(),
		"certificate_template_major_version:100",
		"certificate_template_minor_version:0",
	}, tags)
}

func TestGetTemplateTagsV2WithResolverHit(t *testing.T) {
	templateID := asn1.ObjectIdentifier{1, 3, 6, 1, 4, 1, 311, 21, 8, 1, 2, 3}
	var resolverCalledWith string
	defer stubTemplateOIDResolver(func(oid string) string {
		resolverCalledWith = oid
		return "DatadogTestTemplate"
	})()
	cert := &x509.Certificate{
		Extensions: []pkix.Extension{
			{Id: oidCertTemplateV2, Value: encodeTemplateV2(templateID, 100, 0)},
		},
	}
	tags := getTemplateTags(cert)
	require.Equal(t, templateID.String(), resolverCalledWith)
	require.ElementsMatch(t, []string{
		"certificate_template_oid:" + templateID.String(),
		"certificate_template_major_version:100",
		"certificate_template_minor_version:0",
		"certificate_template_name:DatadogTestTemplate",
	}, tags)
}

func TestGetTemplateTagsBothExtensionsV1NameWins(t *testing.T) {
	// When both V1 and V2 are present, V1's in-band name is authoritative
	// for the name tag; V2 still contributes OID and versions. The resolver
	// is not consulted because the name is already known.
	resolverCalled := false
	defer stubTemplateOIDResolver(func(string) string {
		resolverCalled = true
		return "ResolverShouldNotRun"
	})()
	templateID := asn1.ObjectIdentifier{1, 3, 6, 1, 4, 1, 311, 21, 8, 1, 2, 3}
	cert := &x509.Certificate{
		Extensions: []pkix.Extension{
			{Id: oidCertTemplateV2, Value: encodeTemplateV2(templateID, 1, 0)},
			{Id: oidCertTemplateV1, Value: encodeBMPString("WebServer")},
		},
	}
	tags := getTemplateTags(cert)
	require.False(t, resolverCalled, "resolver must not be called when V1 name is present")
	require.ElementsMatch(t, []string{
		"certificate_template_oid:" + templateID.String(),
		"certificate_template_major_version:1",
		"certificate_template_minor_version:0",
		"certificate_template_name:WebServer",
	}, tags)
}

func TestGetTemplateTagsAbsent(t *testing.T) {
	defer stubTemplateOIDResolver(func(string) string { return "" })()
	require.Empty(t, getTemplateTags(&x509.Certificate{}))
}

func TestGetEnhancedKeyUsageTags(t *testing.T) {
	cert := &x509.Certificate{
		Extensions: []pkix.Extension{
			{
				Id: oidExtKeyUsage,
				Value: encodeEKUExtension(
					asn1.ObjectIdentifier{1, 3, 6, 1, 5, 5, 7, 3, 1},       // serverAuth
					asn1.ObjectIdentifier{1, 3, 6, 1, 5, 5, 7, 3, 2},       // clientAuth
					asn1.ObjectIdentifier{1, 2, 3, 4, 5},                   // unknown
					asn1.ObjectIdentifier{1, 3, 6, 1, 4, 1, 311, 20, 2, 2}, // microsoftSmartcardLogon
				),
			},
		},
	}
	tags := getEnhancedKeyUsageTags(cert)
	require.ElementsMatch(t, []string{
		"enhanced_key_usage:serverAuth",
		"enhanced_key_usage:clientAuth",
		"enhanced_key_usage:1.2.3.4.5",
		"enhanced_key_usage:microsoftSmartcardLogon",
	}, tags)
}

func TestGetEnhancedKeyUsageTagsAbsent(t *testing.T) {
	require.Empty(t, getEnhancedKeyUsageTags(&x509.Certificate{}))
}

func TestGetEnhancedKeyUsageTagsMalformed(t *testing.T) {
	cert := &x509.Certificate{
		Extensions: []pkix.Extension{
			{Id: oidExtKeyUsage, Value: []byte{0xff, 0xff}},
		},
	}
	require.Empty(t, getEnhancedKeyUsageTags(cert))
}

func TestGetSANTags(t *testing.T) {
	uri, _ := url.Parse("https://example.com/path")
	cert := &x509.Certificate{
		DNSNames:       []string{"example.com", "www.example.com"},
		IPAddresses:    []net.IP{net.ParseIP("192.0.2.1"), net.ParseIP("2001:db8::1")},
		EmailAddresses: []string{"admin@example.com"},
		URIs:           []*url.URL{uri},
	}
	tags := getSANTags(cert)
	require.Contains(t, tags, "subject_alt_name_dns:example.com")
	require.Contains(t, tags, "subject_alt_name_dns:www.example.com")
	require.Contains(t, tags, "subject_alt_name_ip:192.0.2.1")
	require.Contains(t, tags, "subject_alt_name_ip:2001:db8::1")
	require.Contains(t, tags, "subject_alt_name_email:admin@example.com")
	require.Contains(t, tags, "subject_alt_name_uri:https://example.com/path")
}

func TestGetSANTagsEmpty(t *testing.T) {
	require.Empty(t, getSANTags(&x509.Certificate{}))
}

func TestGetIssuerTags(t *testing.T) {
	cert := &x509.Certificate{
		Issuer: pkix.Name{
			CommonName:         "Test Root CA",
			Organization:       []string{"Datadog"},
			OrganizationalUnit: []string{"Security"},
			Country:            []string{"US"},
		},
	}
	// pkix.Name.Names is populated by ParseCertificate — for this fixture
	// we must populate it manually via Name.FillFromRDNSequence or by
	// constructing ToRDNSequence().Names. Easier: copy from Name.ToRDNSequence.
	cert.Issuer.ExtraNames = nil
	// Force Names via the ToRDNSequence round-trip:
	var name pkix.Name
	rdn := cert.Issuer.ToRDNSequence()
	name.FillFromRDNSequence(&rdn)
	cert.Issuer = name

	tags := getIssuerTags(cert)
	require.Contains(t, tags, "issuer_CN:Test Root CA")
	require.Contains(t, tags, "issuer_O:Datadog")
	require.Contains(t, tags, "issuer_OU:Security")
	require.Contains(t, tags, "issuer_C:US")
}

func TestGetIssuerTagsEmpty(t *testing.T) {
	require.Empty(t, getIssuerTags(&x509.Certificate{}))
}

func TestGetSignatureAlgorithmTags(t *testing.T) {
	cert := &x509.Certificate{SignatureAlgorithm: x509.SHA256WithRSA}
	tags := getSignatureAlgorithmTags(cert)
	// x509.SHA256WithRSA.String() == "SHA256-RSA"
	require.Equal(t, []string{"signature_algorithm:SHA256-RSA"}, tags)
}

func TestGetSignatureAlgorithmTagsUnknown(t *testing.T) {
	cert := &x509.Certificate{SignatureAlgorithm: x509.UnknownSignatureAlgorithm}
	tags := getSignatureAlgorithmTags(cert)
	require.Equal(t, []string{"signature_algorithm:" + x509.UnknownSignatureAlgorithm.String()}, tags)
}

func TestAppendOptionalTagsAllOff(t *testing.T) {
	cert := &x509.Certificate{
		DNSNames:           []string{"example.com"},
		SignatureAlgorithm: x509.SHA256WithRSA,
	}
	base := []string{"subject_CN:foo"}
	result := appendOptionalTags(base, cert, "My Cert", Config{})
	assert.Equal(t, base, result)
}

func TestAppendOptionalTagsAllOn(t *testing.T) {
	cert := &x509.Certificate{
		DNSNames:           []string{"example.com"},
		SignatureAlgorithm: x509.SHA256WithRSA,
		Issuer:             pkix.Name{CommonName: "Issuer"},
	}
	var name pkix.Name
	rdn := cert.Issuer.ToRDNSequence()
	name.FillFromRDNSequence(&rdn)
	cert.Issuer = name

	cfg := Config{
		CertificateTemplateTag:     true,
		EnhancedKeyUsageTag:        true,
		FriendlyNameTag:            true,
		SubjectAlternativeNamesTag: true,
		IssuerTag:                  true,
		SignatureAlgorithmTag:      true,
	}
	tags := appendOptionalTags(nil, cert, "My Cert", cfg)
	require.Contains(t, tags, "friendly_name:My Cert")
	require.Contains(t, tags, "subject_alt_name_dns:example.com")
	require.Contains(t, tags, "issuer_CN:Issuer")
	require.Contains(t, tags, "signature_algorithm:SHA256-RSA")
}

func TestAppendOptionalTagsFriendlyNameSkippedWhenEmpty(t *testing.T) {
	cert := &x509.Certificate{}
	cfg := Config{FriendlyNameTag: true}
	tags := appendOptionalTags(nil, cert, "", cfg)
	for _, tag := range tags {
		require.NotContains(t, tag, "friendly_name:")
	}
}
