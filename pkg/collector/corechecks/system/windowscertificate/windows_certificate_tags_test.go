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

func TestDecodeUTF16LE(t *testing.T) {
	// "Hi" encoded as UTF-16LE (Windows native): 'H' 0x00 'i' 0x00
	b := []byte{'H', 0x00, 'i', 0x00}
	require.Equal(t, "Hi", decodeUTF16LE(b))

	// Trailing NUL terminator should be stripped
	require.Equal(t, "Hi", decodeUTF16LE([]byte{'H', 0x00, 'i', 0x00, 0x00, 0x00}))
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

func TestGetTemplateTagsV1(t *testing.T) {
	cert := &x509.Certificate{
		Extensions: []pkix.Extension{
			{Id: oidCertTemplateV1, Value: encodeBMPString("WebServer")},
		},
	}
	require.Equal(t, []string{"certificate_template:WebServer"}, getTemplateTags(cert))
}

func TestGetTemplateTagsV2(t *testing.T) {
	templateID := asn1.ObjectIdentifier{1, 3, 6, 1, 4, 1, 311, 21, 8, 1, 2, 3}
	cert := &x509.Certificate{
		Extensions: []pkix.Extension{
			{Id: oidCertTemplateV2, Value: encodeTemplateV2(templateID, 100, 0)},
		},
	}
	tags := getTemplateTags(cert)
	require.Contains(t, tags, "certificate_template:"+templateID.String())
	require.Contains(t, tags, "certificate_template_major_version:100")
	require.Contains(t, tags, "certificate_template_minor_version:0")
}

func TestGetTemplateTagsV1PreferredWhenBothPresent(t *testing.T) {
	templateID := asn1.ObjectIdentifier{1, 3, 6, 1, 4, 1, 311, 21, 8, 1, 2, 3}
	cert := &x509.Certificate{
		Extensions: []pkix.Extension{
			{Id: oidCertTemplateV2, Value: encodeTemplateV2(templateID, 1, 0)},
			{Id: oidCertTemplateV1, Value: encodeBMPString("WebServer")},
		},
	}
	require.Equal(t, []string{"certificate_template:WebServer"}, getTemplateTags(cert))
}

func TestGetTemplateTagsAbsent(t *testing.T) {
	require.Empty(t, getTemplateTags(&x509.Certificate{}))
}

func TestGetEnhancedKeyUsageTags(t *testing.T) {
	cert := &x509.Certificate{
		ExtKeyUsage: []x509.ExtKeyUsage{
			x509.ExtKeyUsageServerAuth,
			x509.ExtKeyUsageClientAuth,
		},
		UnknownExtKeyUsage: []asn1.ObjectIdentifier{
			{1, 2, 3, 4, 5}, // unknown — should emit as dotted string
			{1, 3, 6, 1, 4, 1, 311, 20, 2, 2}, // microsoftSmartcardLogon
		},
	}
	tags := getEnhancedKeyUsageTags(cert)
	require.Contains(t, tags, "enhanced_key_usage:serverAuth")
	require.Contains(t, tags, "enhanced_key_usage:clientAuth")
	require.Contains(t, tags, "enhanced_key_usage:1.2.3.4.5")
	require.Contains(t, tags, "enhanced_key_usage:microsoftSmartcardLogon")
}

func TestGetEnhancedKeyUsageTagsAbsent(t *testing.T) {
	require.Empty(t, getEnhancedKeyUsageTags(&x509.Certificate{}))
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
	require.Contains(t, tags, "san_dns:example.com")
	require.Contains(t, tags, "san_dns:www.example.com")
	require.Contains(t, tags, "san_ip:192.0.2.1")
	require.Contains(t, tags, "san_ip:2001:db8::1")
	require.Contains(t, tags, "san_email:admin@example.com")
	require.Contains(t, tags, "san_uri:https://example.com/path")
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

func TestGetSignatureHashTags(t *testing.T) {
	cert := &x509.Certificate{SignatureAlgorithm: x509.SHA256WithRSA}
	tags := getSignatureHashTags(cert)
	// x509.SHA256WithRSA.String() == "SHA256-RSA"
	require.ElementsMatch(t, []string{
		"signature_algorithm:RSA",
		"signature_hash_algorithm:SHA256",
	}, tags)
}

func TestGetSignatureHashTagsUnknown(t *testing.T) {
	cert := &x509.Certificate{SignatureAlgorithm: x509.UnknownSignatureAlgorithm}
	tags := getSignatureHashTags(cert)
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
	require.Contains(t, tags, "san_dns:example.com")
	require.Contains(t, tags, "issuer_CN:Issuer")
	require.Contains(t, tags, "signature_algorithm:RSA")
	require.Contains(t, tags, "signature_hash_algorithm:SHA256")
}

func TestAppendOptionalTagsFriendlyNameSkippedWhenEmpty(t *testing.T) {
	cert := &x509.Certificate{}
	cfg := Config{FriendlyNameTag: true}
	tags := appendOptionalTags(nil, cert, "", cfg)
	for _, tag := range tags {
		require.NotContains(t, tag, "friendly_name:")
	}
}
