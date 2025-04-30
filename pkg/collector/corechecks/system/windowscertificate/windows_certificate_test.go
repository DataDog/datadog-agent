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
)

func TestWindowsCertificate(t *testing.T) {
	certCheck := new(WinCertChk)

	certCheck.config = Config{
		CertificateStores: []string{"ROOT"},
		CertSubject:       []string{"AAA"},
		DaysCritical:      10,
		DaysWarning:       5,
	}

	m := mocksender.NewMockSender(certCheck.ID())
	certCheck.Configure(m.GetSenderManager(), integration.FakeConfigHash, nil, nil, "test")

	m.On("Gauge", "windows_certificate.days_remaining", mock.AnythingOfType("float64"), "", mock.AnythingOfType("[]string"))
	m.On("ServiceCheck", "windows_certificate.cert_expiration", mock.AnythingOfType("servicecheck.ServiceCheckStatus"), "", mock.AnythingOfType("[]string"), mock.AnythingOfType("string"))
	m.On("Commit").Return().Times(1)

	certCheck.Run()

	m.AssertExpectations(t)
	m.AssertNumberOfCalls(t, "Gauge", 1)
	m.AssertNumberOfCalls(t, "ServiceCheck", 1)
	m.AssertNumberOfCalls(t, "Commit", 1)

}
