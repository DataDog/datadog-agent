// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build windows

package windowscertificate

import (
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

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
	// Certififcates that are expired will always be reported as critical
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
