// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build windows

package windowscertificate

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha1"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"math/big"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/windows"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
	"github.com/DataDog/datadog-agent/pkg/metrics/servicecheck"
)

func TestWindowsCertificate(t *testing.T) {
	certCheck := new(WinCertChk)

	instanceConfig := []byte(`
certificate_store: ROOT
certificate_subjects:
  - Microsoft
  - Datadog
days_warning: 10
days_critical: 5`)

	certCheck.BuildID(integration.FakeConfigHash, instanceConfig, nil)
	m := mocksender.NewMockSender(certCheck.ID())
	certCheck.Configure(m.GetSenderManager(), integration.FakeConfigHash, instanceConfig, nil, "test")

	m.On("Gauge", "windows_certificate.days_remaining", mock.AnythingOfType("float64"), "", mock.AnythingOfType("[]string"))
	m.On("ServiceCheck", "windows_certificate.cert_expiration", mock.AnythingOfType("servicecheck.ServiceCheckStatus"), "", mock.AnythingOfType("[]string"), mock.AnythingOfType("string"))
	m.On("Commit").Return()

	certCheck.Run()

	m.AssertExpectations(t)
	m.AssertCalled(t, "Gauge", "windows_certificate.days_remaining", mock.AnythingOfType("float64"), "", mock.AnythingOfType("[]string"))
	m.AssertCalled(t, "ServiceCheck", "windows_certificate.cert_expiration", mock.AnythingOfType("servicecheck.ServiceCheckStatus"), "", mock.AnythingOfType("[]string"), mock.AnythingOfType("string"))
	m.AssertNumberOfCalls(t, "Commit", 1)

}

func TestWindowsCertificateWithNoCertificates(t *testing.T) {
	certCheck := new(WinCertChk)

	instanceConfig := []byte(`
certificate_store:
certificate_subjects:
days_warning: 10
days_critical: 5`)

	certCheck.BuildID(integration.FakeConfigHash, instanceConfig, nil)
	m := mocksender.NewMockSender(certCheck.ID())
	err := certCheck.Configure(m.GetSenderManager(), integration.FakeConfigHash, instanceConfig, nil, "test")
	require.Error(t, err)

	m.AssertNotCalled(t, "Run")
	m.AssertNumberOfCalls(t, "Gauge", 0)
	m.AssertNumberOfCalls(t, "ServiceCheck", 0)
	m.AssertNumberOfCalls(t, "Commit", 0)
}

func TestWindowsCertificateWithInvalidStore(t *testing.T) {
	certCheck := new(WinCertChk)

	instanceConfig := []byte(`
certificate_store: INVALID`)

	certCheck.BuildID(integration.FakeConfigHash, instanceConfig, nil)
	m := mocksender.NewMockSender(certCheck.ID())
	certCheck.Configure(m.GetSenderManager(), integration.FakeConfigHash, instanceConfig, nil, "test")
	m.On("Commit").Return()

	err := certCheck.Run()
	require.Error(t, err)

	m.AssertExpectations(t)
	m.AssertNumberOfCalls(t, "Gauge", 0)
	m.AssertNumberOfCalls(t, "ServiceCheck", 0)
	m.AssertNumberOfCalls(t, "Commit", 1)
}

func TestWindowsCertificateWithInvalidSubject(t *testing.T) {
	certCheck := new(WinCertChk)

	instanceConfig := []byte(`
certificate_store: ROOT
certificate_subjects:
  - INVALID
days_warning: 10
days_critical: 5`)

	certCheck.BuildID(integration.FakeConfigHash, instanceConfig, nil)
	m := mocksender.NewMockSender(certCheck.ID())
	certCheck.Configure(m.GetSenderManager(), integration.FakeConfigHash, instanceConfig, nil, "test")
	m.On("Commit").Return()

	certCheck.Run()

	m.AssertExpectations(t)
	m.AssertNumberOfCalls(t, "Gauge", 0)
	m.AssertNumberOfCalls(t, "ServiceCheck", 0)
	m.AssertNumberOfCalls(t, "Commit", 1)
}

func TestWindowsCertificateServiceCheckCritical(t *testing.T) {
	certCheck := new(WinCertChk)

	instanceConfig := []byte(`
certificate_store: ROOT
days_warning: 10
days_critical: 500000`)

	certCheck.BuildID(integration.FakeConfigHash, instanceConfig, nil)
	m := mocksender.NewMockSender(certCheck.ID())
	certCheck.Configure(m.GetSenderManager(), integration.FakeConfigHash, instanceConfig, nil, "test")
	m.On("Gauge", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
	m.On("ServiceCheck", "windows_certificate.cert_expiration", servicecheck.ServiceCheckCritical, "", mock.AnythingOfType("[]string"), mock.AnythingOfType("string"))
	m.On("Commit").Return()

	certCheck.Run()

	m.AssertExpectations(t)
	m.AssertCalled(t, "ServiceCheck", "windows_certificate.cert_expiration", servicecheck.ServiceCheckCritical, "", mock.AnythingOfType("[]string"), mock.AnythingOfType("string"))
	m.AssertNotCalled(t, "ServiceCheck", "windows_certificate.cert_expiration", servicecheck.ServiceCheckWarning, "", mock.AnythingOfType("[]string"), mock.AnythingOfType("string"))
	m.AssertNumberOfCalls(t, "Commit", 1)
}

func TestWindowsCertificateServiceCheckWarning(t *testing.T) {
	certCheck := new(WinCertChk)

	instanceConfig := []byte(`
certificate_store: ROOT
certificate_subjects:
  - Microsoft
days_warning: 500000
days_critical: 5`)

	certCheck.BuildID(integration.FakeConfigHash, instanceConfig, nil)
	m := mocksender.NewMockSender(certCheck.ID())
	certCheck.Configure(m.GetSenderManager(), integration.FakeConfigHash, instanceConfig, nil, "test")
	m.On("Gauge", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
	m.On("ServiceCheck", "windows_certificate.cert_expiration", mock.AnythingOfType("servicecheck.ServiceCheckStatus"), "", mock.AnythingOfType("[]string"), mock.AnythingOfType("string"))
	m.On("Commit").Return()

	certCheck.Run()

	m.AssertExpectations(t)
	m.AssertCalled(t, "ServiceCheck", "windows_certificate.cert_expiration", servicecheck.ServiceCheckWarning, "", mock.AnythingOfType("[]string"), mock.AnythingOfType("string"))
	// Certificates that are expired will always be reported as critical
	m.AssertCalled(t, "ServiceCheck", "windows_certificate.cert_expiration", servicecheck.ServiceCheckCritical, "", mock.AnythingOfType("[]string"), mock.AnythingOfType("string"))
	m.AssertNumberOfCalls(t, "Commit", 1)
}

func TestWindowsCertificateNegativeDaysThresholds(t *testing.T) {
	certCheck := new(WinCertChk)

	instanceConfig := []byte(`
certificate_store: ROOT
days_warning: -1
days_critical: -1`)

	certCheck.BuildID(integration.FakeConfigHash, instanceConfig, nil)
	m := mocksender.NewMockSender(certCheck.ID())
	err := certCheck.Configure(m.GetSenderManager(), integration.FakeConfigHash, instanceConfig, nil, "test")
	require.Error(t, err)

	m.AssertNotCalled(t, "Run")
	m.AssertNumberOfCalls(t, "Gauge", 0)
	m.AssertNumberOfCalls(t, "ServiceCheck", 0)
	m.AssertNumberOfCalls(t, "Commit", 0)
}

func TestWindowsCertificateWithCrl(t *testing.T) {
	certCheck := new(WinCertChk)

	instanceConfig := []byte(`
certificate_store: CA
certificate_subjects:
  - Microsoft
enable_crl_monitoring: true`)

	certCheck.BuildID(integration.FakeConfigHash, instanceConfig, nil)
	m := mocksender.NewMockSender(certCheck.ID())
	certCheck.Configure(m.GetSenderManager(), integration.FakeConfigHash, instanceConfig, nil, "test")

	m.On("Gauge", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
	m.On("ServiceCheck", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything)
	m.On("Commit").Return()

	certCheck.Run()

	m.AssertExpectations(t)
	m.AssertCalled(t, "Gauge", "windows_certificate.days_remaining", mock.AnythingOfType("float64"), "", mock.AnythingOfType("[]string"))
	m.AssertCalled(t, "ServiceCheck", "windows_certificate.cert_expiration", mock.AnythingOfType("servicecheck.ServiceCheckStatus"), "", mock.AnythingOfType("[]string"), mock.AnythingOfType("string"))
	m.AssertCalled(t, "Gauge", "windows_certificate.crl_days_remaining", mock.AnythingOfType("float64"), "", mock.AnythingOfType("[]string"))
	m.AssertCalled(t, "ServiceCheck", "windows_certificate.crl_expiration", mock.AnythingOfType("servicecheck.ServiceCheckStatus"), "", mock.AnythingOfType("[]string"), mock.AnythingOfType("string"))
	m.AssertNumberOfCalls(t, "Commit", 1)
}

func TestWindowsCertificateWithCrlNegativeDaysThresholds(t *testing.T) {
	certCheck := new(WinCertChk)

	instanceConfig := []byte(`
certificate_store: CA
certificate_subjects:
  - INVALID
enable_crl_monitoring: true
crl_days_warning: -1`)

	certCheck.BuildID(integration.FakeConfigHash, instanceConfig, nil)
	m := mocksender.NewMockSender(certCheck.ID())
	err := certCheck.Configure(m.GetSenderManager(), integration.FakeConfigHash, instanceConfig, nil, "test")
	require.Error(t, err)

	m.AssertNotCalled(t, "Run")
	m.AssertNumberOfCalls(t, "Gauge", 0)
	m.AssertNumberOfCalls(t, "ServiceCheck", 0)
	m.AssertNumberOfCalls(t, "Commit", 0)
}

func TestWindowsCertificateWithCrlNoCrlFound(t *testing.T) {
	certCheck := new(WinCertChk)

	instanceConfig := []byte(`
certificate_store: MY
certificate_subjects:
  - INVALID
enable_crl_monitoring: true`)

	certCheck.BuildID(integration.FakeConfigHash, instanceConfig, nil)
	m := mocksender.NewMockSender(certCheck.ID())
	certCheck.Configure(m.GetSenderManager(), integration.FakeConfigHash, instanceConfig, nil, "test")
	m.On("Commit").Return()

	certCheck.Run()

	m.AssertExpectations(t)
	m.AssertNumberOfCalls(t, "Gauge", 0)
	m.AssertNumberOfCalls(t, "ServiceCheck", 0)
	m.AssertNumberOfCalls(t, "Commit", 1)
}

func TestWindowsCertificateCertChainVerification(t *testing.T) {
	certCheck := new(WinCertChk)

	instanceConfig := []byte(`
certificate_store: ROOT
certificate_subjects:
  - Microsoft
cert_chain_validation:
  enabled: true
`)

	certCheck.BuildID(integration.FakeConfigHash, instanceConfig, nil)
	m := mocksender.NewMockSender(certCheck.ID())
	certCheck.Configure(m.GetSenderManager(), integration.FakeConfigHash, instanceConfig, nil, "test")

	m.On("Gauge", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
	m.On("ServiceCheck", "windows_certificate.cert_expiration", mock.AnythingOfType("servicecheck.ServiceCheckStatus"), "", mock.AnythingOfType("[]string"), mock.AnythingOfType("string"))
	m.On("ServiceCheck", "windows_certificate.cert_chain_validation", mock.AnythingOfType("servicecheck.ServiceCheckStatus"), "", mock.AnythingOfType("[]string"), mock.AnythingOfType("string"))
	m.On("Commit").Return()

	certCheck.Run()

	m.AssertExpectations(t)
	m.AssertCalled(t, "Gauge", "windows_certificate.days_remaining", mock.AnythingOfType("float64"), "", mock.AnythingOfType("[]string"))
	m.AssertCalled(t, "ServiceCheck", "windows_certificate.cert_expiration", mock.AnythingOfType("servicecheck.ServiceCheckStatus"), "", mock.AnythingOfType("[]string"), mock.AnythingOfType("string"))
	m.AssertCalled(t, "ServiceCheck", "windows_certificate.cert_chain_validation", servicecheck.ServiceCheckOK, "", mock.AnythingOfType("[]string"), mock.AnythingOfType("string"))
	m.AssertNumberOfCalls(t, "Commit", 1)
}

func TestWindowsCertificateCertChainVerificationWithFlags(t *testing.T) {
	certCheck := new(WinCertChk)

	instanceConfig := []byte(`
certificate_store: ROOT
cert_chain_validation:
  enabled: true
  policy_validation_flags:
    - "CERT_CHAIN_POLICY_IGNORE_NOT_TIME_VALID_FLAG"
    - "CERT_CHAIN_POLICY_IGNORE_CTL_NOT_TIME_VALID_FLAG"
    - "CERT_CHAIN_POLICY_IGNORE_NOT_TIME_NESTED_FLAG"
    - "CERT_CHAIN_POLICY_IGNORE_INVALID_BASIC_CONSTRAINTS_FLAG"
    - "CERT_CHAIN_POLICY_IGNORE_INVALID_NAME_FLAG"
    - "CERT_CHAIN_POLICY_IGNORE_INVALID_POLICY_FLAG"
    - "CERT_CHAIN_POLICY_IGNORE_END_REV_UNKNOWN_FLAG"
    - "CERT_CHAIN_POLICY_IGNORE_CTL_SIGNER_REV_UNKNOWN_FLAG"
    - "CERT_CHAIN_POLICY_IGNORE_CA_REV_UNKNOWN_FLAG"
    - "CERT_CHAIN_POLICY_IGNORE_ROOT_REV_UNKNOWN_FLAG"
`)

	certCheck.BuildID(integration.FakeConfigHash, instanceConfig, nil)
	m := mocksender.NewMockSender(certCheck.ID())
	certCheck.Configure(m.GetSenderManager(), integration.FakeConfigHash, instanceConfig, nil, "test")

	m.On("Gauge", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
	m.On("ServiceCheck", "windows_certificate.cert_expiration", mock.AnythingOfType("servicecheck.ServiceCheckStatus"), "", mock.AnythingOfType("[]string"), mock.AnythingOfType("string"))
	m.On("ServiceCheck", "windows_certificate.cert_chain_validation", mock.AnythingOfType("servicecheck.ServiceCheckStatus"), "", mock.AnythingOfType("[]string"), mock.AnythingOfType("string"))
	m.On("Commit").Return()

	certCheck.Run()

	m.AssertExpectations(t)
	m.AssertCalled(t, "Gauge", "windows_certificate.days_remaining", mock.AnythingOfType("float64"), "", mock.AnythingOfType("[]string"))
	m.AssertCalled(t, "ServiceCheck", "windows_certificate.cert_expiration", mock.AnythingOfType("servicecheck.ServiceCheckStatus"), "", mock.AnythingOfType("[]string"), mock.AnythingOfType("string"))
	m.AssertCalled(t, "ServiceCheck", "windows_certificate.cert_chain_validation", servicecheck.ServiceCheckOK, "", mock.AnythingOfType("[]string"), mock.AnythingOfType("string"))
	m.AssertNumberOfCalls(t, "Commit", 1)
}

func TestWindowsCertificateCertChainVerificationWithEmptyPolicyValidationFlags(t *testing.T) {
	certCheck := new(WinCertChk)

	instanceConfig := []byte(`
certificate_store: ROOT
certificate_subjects:
  - Microsoft
cert_chain_validation:
  enabled: true
  policy_validation_flags:
`)

	certCheck.BuildID(integration.FakeConfigHash, instanceConfig, nil)
	m := mocksender.NewMockSender(certCheck.ID())
	certCheck.Configure(m.GetSenderManager(), integration.FakeConfigHash, instanceConfig, nil, "test")

	m.On("Gauge", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
	m.On("ServiceCheck", "windows_certificate.cert_expiration", mock.AnythingOfType("servicecheck.ServiceCheckStatus"), "", mock.AnythingOfType("[]string"), mock.AnythingOfType("string"))
	m.On("ServiceCheck", "windows_certificate.cert_chain_validation", mock.AnythingOfType("servicecheck.ServiceCheckStatus"), "", mock.AnythingOfType("[]string"), mock.AnythingOfType("string"))
	m.On("Commit").Return()

	certCheck.Run()

	m.AssertExpectations(t)
	m.AssertCalled(t, "Gauge", "windows_certificate.days_remaining", mock.AnythingOfType("float64"), "", mock.AnythingOfType("[]string"))
	m.AssertCalled(t, "ServiceCheck", "windows_certificate.cert_expiration", mock.AnythingOfType("servicecheck.ServiceCheckStatus"), "", mock.AnythingOfType("[]string"), mock.AnythingOfType("string"))
	m.AssertCalled(t, "ServiceCheck", "windows_certificate.cert_chain_validation", servicecheck.ServiceCheckOK, "", mock.AnythingOfType("[]string"), mock.AnythingOfType("string"))
	m.AssertNumberOfCalls(t, "Commit", 1)
}

func TestWindowsCertificateCertChainVerificationWithNoCertificates(t *testing.T) {
	certCheck := new(WinCertChk)

	instanceConfig := []byte(`
certificate_store: ROOT
certificate_subjects:
  - INVALID
cert_chain_validation:
  enabled: true
`)

	certCheck.BuildID(integration.FakeConfigHash, instanceConfig, nil)
	m := mocksender.NewMockSender(certCheck.ID())
	certCheck.Configure(m.GetSenderManager(), integration.FakeConfigHash, instanceConfig, nil, "test")
	m.On("Commit").Return()

	certCheck.Run()

	m.AssertExpectations(t)
	m.AssertNumberOfCalls(t, "Gauge", 0)
	m.AssertNumberOfCalls(t, "ServiceCheck", 0)
	m.AssertNumberOfCalls(t, "Commit", 1)
}

func TestCRLIssuerTags(t *testing.T) {
	issuer := "L=Internet\r\n O=\"VeriSign, Inc.\"\r\n OU=VeriSign Commercial Software Publishers CA"
	tags := getCrlIssuerTags(issuer)
	require.Equal(t, []string{"crl_issuer_L:Internet", "crl_issuer_O:VeriSign, Inc.", "crl_issuer_OU:VeriSign Commercial Software Publishers CA"}, tags)

	issuer = "L=Internet\r\n O=\"VeriSign, Inc.\"\r\n OU=VeriSign Commercial Software Publishers CA\r\n CN=VeriSign Class 3 Public Primary Certification Authority - G5"
	tags = getCrlIssuerTags(issuer)
	require.Equal(t, []string{"crl_issuer_L:Internet", "crl_issuer_O:VeriSign, Inc.", "crl_issuer_OU:VeriSign Commercial Software Publishers CA", "crl_issuer_CN:VeriSign Class 3 Public Primary Certification Authority - G5"}, tags)

	issuer = "CN=GlobalSign Root CA\r\n OU=GlobalSign\r\n O=GlobalSign nv-sa\r\n C=BE"
	tags = getCrlIssuerTags(issuer)
	require.Equal(t, []string{"crl_issuer_CN:GlobalSign Root CA", "crl_issuer_OU:GlobalSign", "crl_issuer_O:GlobalSign nv-sa", "crl_issuer_C:BE"}, tags)

	issuer = "C=US\r\n S=Arizona\r\n L=Scottsdale\r\n O=\"GoDaddy.com, Inc.\"\r\n OU=http://certificates.godaddy.com/repository\r\n CN=Go Daddy Secure Certification Authority\r\n SERIALNUMBER=07969287"
	tags = getCrlIssuerTags(issuer)
	require.Equal(t, []string{"crl_issuer_C:US", "crl_issuer_S:Arizona",
		"crl_issuer_L:Scottsdale",
		"crl_issuer_O:GoDaddy.com, Inc.",
		"crl_issuer_OU:http://certificates.godaddy.com/repository",
		"crl_issuer_CN:Go Daddy Secure Certification Authority",
		"crl_issuer_SERIALNUMBER:07969287"}, tags)
}

func TestThumbprintSerialNumberTags(t *testing.T) {
	certCheck := new(WinCertChk)

	instanceConfig := []byte(`
certificate_store: CA
enable_crl_monitoring: true
`)

	certCheck.BuildID(integration.FakeConfigHash, instanceConfig, nil)
	m := mocksender.NewMockSender(certCheck.ID())
	certCheck.Configure(m.GetSenderManager(), integration.FakeConfigHash, instanceConfig, nil, "test")

	m.On("Gauge", "windows_certificate.days_remaining", mock.AnythingOfType("float64"), "", mock.MatchedBy(func(tags []string) bool {
		for _, tag := range tags {
			if strings.HasPrefix(tag, "certificate_thumbprint:") {
				return true
			} else if strings.HasPrefix(tag, "certificate_serial_number:") {
				return true
			}
		}
		return false
	}))
	m.On("ServiceCheck", "windows_certificate.cert_expiration", mock.AnythingOfType("servicecheck.ServiceCheckStatus"), "", mock.MatchedBy(func(tags []string) bool {
		for _, tag := range tags {
			if strings.HasPrefix(tag, "certificate_thumbprint:") {
				return true
			} else if strings.HasPrefix(tag, "certificate_serial_number:") {
				return true
			}
		}
		return false
	}), mock.AnythingOfType("string"))
	m.On("Gauge", "windows_certificate.crl_days_remaining", mock.AnythingOfType("float64"), "", mock.MatchedBy(func(tags []string) bool {
		for _, tag := range tags {
			if strings.HasPrefix(tag, "crl_thumbprint:") {
				return true
			}
		}
		return false
	}))
	m.On("ServiceCheck", "windows_certificate.crl_expiration", mock.AnythingOfType("servicecheck.ServiceCheckStatus"), "", mock.MatchedBy(func(tags []string) bool {
		for _, tag := range tags {
			if strings.HasPrefix(tag, "crl_thumbprint:") {
				return true
			}
		}
		return false
	}), mock.AnythingOfType("string"))
	m.On("Commit").Return()

	certCheck.Run()

	m.AssertExpectations(t)
	m.AssertCalled(t, "Gauge", "windows_certificate.days_remaining", mock.AnythingOfType("float64"), "", mock.AnythingOfType("[]string"))
	m.AssertCalled(t, "ServiceCheck", "windows_certificate.cert_expiration", mock.AnythingOfType("servicecheck.ServiceCheckStatus"), "", mock.AnythingOfType("[]string"), mock.AnythingOfType("string"))
	m.AssertCalled(t, "Gauge", "windows_certificate.crl_days_remaining", mock.AnythingOfType("float64"), "", mock.AnythingOfType("[]string"))
	m.AssertCalled(t, "ServiceCheck", "windows_certificate.crl_expiration", mock.AnythingOfType("servicecheck.ServiceCheckStatus"), "", mock.AnythingOfType("[]string"), mock.AnythingOfType("string"))
	m.AssertNumberOfCalls(t, "Commit", 1)
}

func TestGetCertThumbprint(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	now := time.Now()
	tpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "Go Test Certificate"},
		NotBefore:    now.Add(-5 * time.Minute),
		NotAfter:     now.AddDate(1, 0, 0),
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		IsCA:         true, // self-signed
	}
	der, err := x509.CreateCertificate(rand.Reader, tpl, tpl, &key.PublicKey, key)
	require.NoError(t, err)

	derThumbprint := sha1.Sum(der)

	certContext, err := windows.CertCreateCertificateContext(windows.X509_ASN_ENCODING, &der[0], uint32(len(der)))
	require.NoError(t, err)

	thumbprint, err := getCertThumbprint(certContext)
	require.NoError(t, err)
	require.Equal(t, hex.EncodeToString(derThumbprint[:]), thumbprint)
}
