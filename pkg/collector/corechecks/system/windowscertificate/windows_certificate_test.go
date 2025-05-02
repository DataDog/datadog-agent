// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.
//go:build windows

//nolint:revive // TODO(WINA) Fix revive linter

package windowscertificate

import (
	"testing"

	"github.com/stretchr/testify/mock"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
	"github.com/DataDog/datadog-agent/pkg/metrics/servicecheck"
)

func TestWindowsCertificate(t *testing.T) {
	certCheck := new(WinCertChk)

	certCheck.config = Config{
		CertificateStores: []string{"ROOT", "CA"},
		CertSubject:       []string{"Microsoft", "Datadog"},
		DaysCritical:      5,
		DaysWarning:       10,
	}

	m := mocksender.NewMockSender(certCheck.ID())
	certCheck.Configure(m.GetSenderManager(), integration.FakeConfigHash, nil, nil, "test")

	m.On("Gauge", "windows_certificate.days_remaining", mock.AnythingOfType("float64"), "", mock.AnythingOfType("[]string"))
	m.On("ServiceCheck", "windows_certificate.cert_expiration", mock.AnythingOfType("servicecheck.ServiceCheckStatus"), "", mock.AnythingOfType("[]string"), mock.AnythingOfType("string"))
	m.On("Commit").Return().Times(1)

	certCheck.Run()

	m.AssertCalled(t, "Gauge", "windows_certificate.days_remaining", mock.AnythingOfType("float64"), "", mock.AnythingOfType("[]string"))
	m.AssertCalled(t, "ServiceCheck", "windows_certificate.cert_expiration", mock.AnythingOfType("servicecheck.ServiceCheckStatus"), "", mock.AnythingOfType("[]string"), mock.AnythingOfType("string"))
	m.AssertNumberOfCalls(t, "Commit", 1)

}

func TestWindowsCertificateWithNoCertificates(t *testing.T) {
	certCheck := new(WinCertChk)

	certCheck.config = Config{
		CertificateStores: []string{},
		CertSubject:       []string{},
		DaysCritical:      5,
		DaysWarning:       10,
	}

	m := mocksender.NewMockSender(certCheck.ID())
	certCheck.Configure(m.GetSenderManager(), integration.FakeConfigHash, nil, nil, "test")
	m.On("Commit").Return().Times(1)

	certCheck.Run()

	m.AssertExpectations(t)
	m.AssertNumberOfCalls(t, "Gauge", 0)
	m.AssertNumberOfCalls(t, "ServiceCheck", 0)
	m.AssertNumberOfCalls(t, "Commit", 1)
}

func TestWindowsCertificateWithInvalidStore(t *testing.T) {
	certCheck := new(WinCertChk)

	certCheck.config = Config{
		CertificateStores: []string{"INVALID"},
	}

	m := mocksender.NewMockSender(certCheck.ID())
	certCheck.Configure(m.GetSenderManager(), integration.FakeConfigHash, nil, nil, "test")
	m.On("Commit").Return().Times(1)

	certCheck.Run()

	m.AssertExpectations(t)
	m.AssertNumberOfCalls(t, "Gauge", 0)
	m.AssertNumberOfCalls(t, "ServiceCheck", 0)
	m.AssertNumberOfCalls(t, "Commit", 1)
}

func TestWindowsCertificateWithInvalidSubject(t *testing.T) {
	certCheck := new(WinCertChk)

	certCheck.config = Config{
		CertificateStores: []string{"ROOT"},
		CertSubject:       []string{"INVALID"},
		DaysCritical:      5,
		DaysWarning:       10,
	}

	m := mocksender.NewMockSender(certCheck.ID())
	certCheck.Configure(m.GetSenderManager(), integration.FakeConfigHash, nil, nil, "test")
	m.On("Commit").Return().Times(1)

	certCheck.Run()

	m.AssertExpectations(t)
	m.AssertNumberOfCalls(t, "Gauge", 0)
	m.AssertNumberOfCalls(t, "ServiceCheck", 0)
	m.AssertNumberOfCalls(t, "Commit", 1)
}

func TestWindowsCertificateServiceCheckCritical(t *testing.T) {
	certCheck := new(WinCertChk)

	certCheck.config = Config{
		CertificateStores: []string{"ROOT"},
		DaysCritical:      500000,
		DaysWarning:       10,
	}

	m := mocksender.NewMockSender(certCheck.ID())
	certCheck.Configure(m.GetSenderManager(), integration.FakeConfigHash, nil, nil, "test")
	m.On("Gauge", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
	m.On("ServiceCheck", "windows_certificate.cert_expiration", servicecheck.ServiceCheckCritical, "", mock.AnythingOfType("[]string"), mock.AnythingOfType("string"))
	m.On("Commit").Return().Times(1)

	certCheck.Run()

	m.AssertCalled(t, "ServiceCheck", "windows_certificate.cert_expiration", servicecheck.ServiceCheckCritical, "", mock.AnythingOfType("[]string"), mock.AnythingOfType("string"))
	m.AssertNumberOfCalls(t, "Commit", 1)
}

func TestWindowsCertificateServiceCheckWarning(t *testing.T) {
	certCheck := new(WinCertChk)

	certCheck.config = Config{
		CertificateStores: []string{"ROOT"},
		DaysCritical:      5,
		DaysWarning:       500000,
	}

	m := mocksender.NewMockSender(certCheck.ID())
	certCheck.Configure(m.GetSenderManager(), integration.FakeConfigHash, nil, nil, "test")
	m.On("Gauge", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
	m.On("ServiceCheck", "windows_certificate.cert_expiration", mock.AnythingOfType("servicecheck.ServiceCheckStatus"), "", mock.AnythingOfType("[]string"), mock.AnythingOfType("string"))
	m.On("Commit").Return().Times(1)

	certCheck.Run()

	m.AssertCalled(t, "ServiceCheck", "windows_certificate.cert_expiration", servicecheck.ServiceCheckWarning, "", mock.AnythingOfType("[]string"), mock.AnythingOfType("string"))
	// Certififcates that are expired will always be reported as critical
	m.AssertCalled(t, "ServiceCheck", "windows_certificate.cert_expiration", servicecheck.ServiceCheckCritical, "", mock.AnythingOfType("[]string"), "Certificate has expired")
	m.AssertNumberOfCalls(t, "Commit", 1)
}
