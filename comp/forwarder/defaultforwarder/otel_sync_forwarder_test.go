// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package defaultforwarder

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	secretsmock "github.com/DataDog/datadog-agent/comp/core/secrets/mock"
	"github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder/transaction"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/config/utils"
	"github.com/DataDog/datadog-agent/pkg/version"
)

// newTestOTelSyncForwarder builds an OTelSyncForwarder wired to the given HTTP
// client. The config is pointed at a dummy URL; callers are expected to
// intercept transport-level via a custom http.Client.
func newTestOTelSyncForwarder(t *testing.T, client *http.Client, extraSetup ...func(pkgconfigmodel.BuildableConfig)) *OTelSyncForwarder {
	t.Helper()
	cfg := configmock.New(t)
	cfg.Set("api_key", "testapikey0000000000000000000000000", pkgconfigmodel.SourceFile)
	cfg.Set("dd_url", "https://app.datadoghq.com", pkgconfigmodel.SourceDefault)
	for _, fn := range extraSetup {
		fn(cfg)
	}
	log := logmock.New(t)
	sec := secretsmock.New(t)
	eds := utils.EndpointDescriptorSet{
		"https://app.datadoghq.com": {
			BaseURL:   "https://app.datadoghq.com",
			APIKeySet: []utils.APIKeys{utils.NewAPIKeys("api_key", "testapikey0000000000000000000000000")},
		},
	}
	f, err := NewOTelSyncForwarder(cfg, log, sec, eds, client)
	require.NoError(t, err)
	return f
}

func TestOTelSyncForwarder_StartStop(t *testing.T) {
	f := newTestOTelSyncForwarder(t, &http.Client{})
	assert.NoError(t, f.Start(), "Start should be a no-op returning nil")
	f.Stop() // must not panic
}

func TestOTelSyncForwarder_SubmitTransaction_VersionHeader(t *testing.T) {
	var capturedReq *http.Request
	client := &http.Client{
		Transport: handlerTransport(func(w http.ResponseWriter, r *http.Request) {
			capturedReq = r.Clone(r.Context())
			w.WriteHeader(http.StatusOK)
		}),
	}
	f := newTestOTelSyncForwarder(t, client)

	txn := transaction.NewHTTPTransaction()
	txn.Domain = "https://app.datadoghq.com"
	txn.Endpoint = transaction.Endpoint{Route: "/api/v1/series"}
	txn.Payload = transaction.NewBytesPayloadWithoutMetaData([]byte("{}"))

	require.NoError(t, f.SubmitTransaction(txn))
	require.NotNil(t, capturedReq)
	assert.Equal(t, version.AgentVersion, capturedReq.Header.Get("DD-Agent-Version"))
	assert.Equal(t, "datadog-agent/"+version.AgentVersion, capturedReq.Header.Get("User-Agent"))
}

func TestOTelSyncForwarder_SubmitTransaction_NoArbitraryTagsHeaderByDefault(t *testing.T) {
	var capturedReq *http.Request
	client := &http.Client{
		Transport: handlerTransport(func(w http.ResponseWriter, r *http.Request) {
			capturedReq = r.Clone(r.Context())
			w.WriteHeader(http.StatusOK)
		}),
	}
	f := newTestOTelSyncForwarder(t, client) // allow_arbitrary_tags defaults to false

	txn := transaction.NewHTTPTransaction()
	txn.Domain = "https://app.datadoghq.com"
	txn.Endpoint = transaction.Endpoint{Route: "/api/v1/series"}
	txn.Payload = transaction.NewBytesPayloadWithoutMetaData([]byte("{}"))

	require.NoError(t, f.SubmitTransaction(txn))
	require.NotNil(t, capturedReq)
	assert.Empty(t, capturedReq.Header.Get("Allow-Arbitrary-Tag-Value"))
}

func TestOTelSyncForwarder_SubmitTransaction_ArbitraryTagsHeader(t *testing.T) {
	var capturedReq *http.Request
	client := &http.Client{
		Transport: handlerTransport(func(w http.ResponseWriter, r *http.Request) {
			capturedReq = r.Clone(r.Context())
			w.WriteHeader(http.StatusOK)
		}),
	}
	f := newTestOTelSyncForwarder(t, client, func(cfg pkgconfigmodel.BuildableConfig) {
		cfg.Set("allow_arbitrary_tags", true, pkgconfigmodel.SourceFile)
	})

	txn := transaction.NewHTTPTransaction()
	txn.Domain = "https://app.datadoghq.com"
	txn.Endpoint = transaction.Endpoint{Route: "/api/v1/series"}
	txn.Payload = transaction.NewBytesPayloadWithoutMetaData([]byte("{}"))

	require.NoError(t, f.SubmitTransaction(txn))
	require.NotNil(t, capturedReq)
	assert.Equal(t, "true", capturedReq.Header.Get("Allow-Arbitrary-Tag-Value"))
}

func TestOTelSyncForwarder_sendHTTPTransactions_PropagatesError(t *testing.T) {
	client := &http.Client{
		Transport: handlerTransport(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}),
	}
	f := newTestOTelSyncForwarder(t, client)

	txn := transaction.NewHTTPTransaction()
	txn.Domain = "https://app.datadoghq.com"
	txn.Endpoint = transaction.Endpoint{Route: "/api/v1/series"}
	txn.Payload = transaction.NewBytesPayloadWithoutMetaData([]byte("{}"))

	err := f.sendHTTPTransactions([]*transaction.HTTPTransaction{txn})
	assert.Error(t, err, "5xx response should surface as an error")
}

func TestOTelSyncForwarder_sendHTTPTransactions_SuccessOnOK(t *testing.T) {
	client := &http.Client{
		Transport: handlerTransport(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}),
	}
	f := newTestOTelSyncForwarder(t, client)

	txn := transaction.NewHTTPTransaction()
	txn.Domain = "https://app.datadoghq.com"
	txn.Endpoint = transaction.Endpoint{Route: "/api/v1/series"}
	txn.Payload = transaction.NewBytesPayloadWithoutMetaData([]byte("{}"))

	assert.NoError(t, f.sendHTTPTransactions([]*transaction.HTTPTransaction{txn}))
}

func TestOTelSyncForwarder_sendHTTPTransactions_CombinesMultipleErrors(t *testing.T) {
	client := &http.Client{
		Transport: handlerTransport(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusBadGateway)
		}),
	}
	f := newTestOTelSyncForwarder(t, client)

	makeTxn := func() *transaction.HTTPTransaction {
		txn := transaction.NewHTTPTransaction()
		txn.Domain = "https://app.datadoghq.com"
		txn.Endpoint = transaction.Endpoint{Route: "/api/v1/series"}
		txn.Payload = transaction.NewBytesPayloadWithoutMetaData([]byte("{}"))
		return txn
	}

	err := f.sendHTTPTransactions([]*transaction.HTTPTransaction{makeTxn(), makeTxn()})
	require.Error(t, err)
	// multierr wraps both individual errors; the combined message should
	// reference both failures.
	assert.Contains(t, err.Error(), "502")
}

func TestOTelSyncForwarder_SubmitV1Intake_SetsContentType(t *testing.T) {
	var capturedReq *http.Request
	client := &http.Client{
		Transport: handlerTransport(func(w http.ResponseWriter, r *http.Request) {
			capturedReq = r.Clone(r.Context())
			w.WriteHeader(http.StatusOK)
		}),
	}
	f := newTestOTelSyncForwarder(t, client)

	payload := transaction.BytesPayloads{
		transaction.NewBytesPayloadWithoutMetaData([]byte(`{}`)),
	}
	require.NoError(t, f.SubmitV1Intake(payload, transaction.Metadata, http.Header{}))
	require.NotNil(t, capturedReq)
	assert.Equal(t, "application/json", capturedReq.Header.Get("Content-Type"))
}

func TestOTelSyncForwarder_GetDomainResolvers_NonEmpty(t *testing.T) {
	f := newTestOTelSyncForwarder(t, &http.Client{})
	resolvers := f.GetDomainResolvers()
	assert.NotEmpty(t, resolvers, "GetDomainResolvers should return at least one resolver")
}
