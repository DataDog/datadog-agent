// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package defaultforwarderimpl

import (
	"context"
	"net/http"
	"strings"
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
		Transport: handlerTransport(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}),
	}
	f := newTestOTelSyncForwarder(t, client)

	txn := transaction.NewHTTPTransaction()
	txn.Domain = "https://app.datadoghq.com"
	txn.Endpoint = transaction.Endpoint{Route: "/api/v1/series"}
	txn.Payload = transaction.NewBytesPayloadWithoutMetaData([]byte("{}"))

	err := f.sendHTTPTransactions(context.Background(), []*transaction.HTTPTransaction{txn})
	assert.Error(t, err, "5xx response should surface as an error")
}

func TestOTelSyncForwarder_sendHTTPTransactions_PermanentErrorOnBadRequest(t *testing.T) {
	for _, status := range []int{http.StatusBadRequest, http.StatusRequestEntityTooLarge, http.StatusForbidden} {
		status := status
		t.Run(http.StatusText(status), func(t *testing.T) {
			client := &http.Client{
				Transport: handlerTransport(func(w http.ResponseWriter, _ *http.Request) {
					w.WriteHeader(status)
				}),
			}
			f := newTestOTelSyncForwarder(t, client)

			txn := transaction.NewHTTPTransaction()
			txn.Domain = "https://app.datadoghq.com"
			txn.Endpoint = transaction.Endpoint{Route: "/api/v1/series"}
			txn.Payload = transaction.NewBytesPayloadWithoutMetaData([]byte("{}"))

			err := f.sendHTTPTransactions(context.Background(), []*transaction.HTTPTransaction{txn})
			require.Error(t, err, "permanent %d should surface as an error instead of being silently dropped", status)
			assert.ErrorIs(t, err, ErrPermanentHTTPError, "permanent %d must wrap ErrPermanentHTTPError so callers can mark it non-retryable", status)
		})
	}
}

func TestOTelSyncForwarder_sendHTTPTransactions_SuccessOnOK(t *testing.T) {
	client := &http.Client{
		Transport: handlerTransport(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		}),
	}
	f := newTestOTelSyncForwarder(t, client)

	txn := transaction.NewHTTPTransaction()
	txn.Domain = "https://app.datadoghq.com"
	txn.Endpoint = transaction.Endpoint{Route: "/api/v1/series"}
	txn.Payload = transaction.NewBytesPayloadWithoutMetaData([]byte("{}"))

	assert.NoError(t, f.sendHTTPTransactions(context.Background(), []*transaction.HTTPTransaction{txn}))
}

func TestOTelSyncForwarder_sendHTTPTransactions_CombinesMultipleErrors(t *testing.T) {
	client := &http.Client{
		Transport: handlerTransport(func(w http.ResponseWriter, _ *http.Request) {
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

	err := f.sendHTTPTransactions(context.Background(), []*transaction.HTTPTransaction{makeTxn(), makeTxn()})
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

// contextAwareTransport wraps an http.HandlerFunc but returns the request
// context's error immediately if the context is already done, so that
// internalProcess sees ctx.Err() == context.Canceled and takes the silent-drop
// path — which is exactly what our new propagation check must catch.
type contextAwareTransport http.HandlerFunc

func (tr contextAwareTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if err := req.Context().Err(); err != nil {
		return nil, err
	}
	return handlerTransport(tr).RoundTrip(req)
}

func TestOTelSyncForwarder_SubmitV1IntakeDirect_PropagatesCancellation(t *testing.T) {
	client := &http.Client{
		Transport: contextAwareTransport(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		}),
	}
	f := newTestOTelSyncForwarder(t, client)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // pre-cancel so internalProcess takes the silent-drop path

	payload := transaction.BytesPayloads{
		transaction.NewBytesPayloadWithoutMetaData([]byte(`{}`)),
	}
	err := f.SubmitV1IntakeDirect(ctx, payload, transaction.Metadata, http.Header{})
	require.Error(t, err, "canceled context must surface as an error through SubmitV1IntakeDirect")
	assert.ErrorIs(t, err, context.Canceled)
}

// newTestOTelSyncForwarderWithMRF builds an OTelSyncForwarder with both a primary
// and an MRF endpoint in the EDS so MRF gating behaviour can be tested.
func newTestOTelSyncForwarderWithMRF(t *testing.T, client *http.Client, mrfEnabled, failoverMetrics bool) *OTelSyncForwarder {
	t.Helper()
	cfg := configmock.New(t)
	cfg.Set("api_key", "testapikey0000000000000000000000000", pkgconfigmodel.SourceFile)
	cfg.Set("dd_url", "https://app.datadoghq.com", pkgconfigmodel.SourceDefault)
	cfg.Set("multi_region_failover.enabled", mrfEnabled, pkgconfigmodel.SourceFile)
	cfg.Set("multi_region_failover.failover_metrics", failoverMetrics, pkgconfigmodel.SourceFile)
	log := logmock.New(t)
	sec := secretsmock.New(t)
	eds := utils.EndpointDescriptorSet{
		"https://app.datadoghq.com": {
			BaseURL:   "https://app.datadoghq.com",
			APIKeySet: []utils.APIKeys{utils.NewAPIKeys("api_key", "testapikey0000000000000000000000000")},
		},
		"https://mrf.datadoghq.com": {
			BaseURL:   "https://mrf.datadoghq.com",
			APIKeySet: []utils.APIKeys{utils.NewAPIKeys("multi_region_failover.api_key", "mrfapikey0000000000000000000000000")},
			IsMRF:     true,
		},
	}
	f, err := NewOTelSyncForwarder(cfg, log, sec, eds, client)
	require.NoError(t, err)
	return f
}

func TestOTelSyncForwarder_sendHTTPTransactions_SkipsMRFWhenFailoverMetricsDisabled(t *testing.T) {
	var requestedHosts []string
	client := &http.Client{
		Transport: handlerTransport(func(w http.ResponseWriter, r *http.Request) {
			requestedHosts = append(requestedHosts, r.Host)
			w.WriteHeader(http.StatusOK)
		}),
	}
	f := newTestOTelSyncForwarderWithMRF(t, client, true /* enabled */, false /* failover_metrics disabled */)

	payload := transaction.BytesPayloads{
		transaction.NewBytesPayloadWithoutMetaData([]byte(`{}`)),
	}
	require.NoError(t, f.SubmitV1Series(payload, http.Header{}))

	// Only the primary domain should have been contacted; MRF must be skipped.
	// Note: NewDefaultForwarder prefixes known DD domains with the agent version
	// (e.g. app.datadoghq.com → 7-81-0-app.agent.datadoghq.com), so we check
	// for the absence of "mrf" rather than an exact host match.
	require.Len(t, requestedHosts, 1, "exactly one request expected when MRF disabled")
	assert.NotContains(t, requestedHosts[0], "mrf", "MRF domain must not receive traffic when failover_metrics=false")
}

func TestOTelSyncForwarder_sendHTTPTransactions_SkipsMRFWhenMRFNotEnabled(t *testing.T) {
	var requestedHosts []string
	client := &http.Client{
		Transport: handlerTransport(func(w http.ResponseWriter, r *http.Request) {
			requestedHosts = append(requestedHosts, r.Host)
			w.WriteHeader(http.StatusOK)
		}),
	}
	// enabled=false but failover_metrics=true: MRF must still be skipped.
	f := newTestOTelSyncForwarderWithMRF(t, client, false /* enabled */, true /* failover_metrics */)

	payload := transaction.BytesPayloads{
		transaction.NewBytesPayloadWithoutMetaData([]byte(`{}`)),
	}
	require.NoError(t, f.SubmitV1Series(payload, http.Header{}))

	require.Len(t, requestedHosts, 1, "exactly one request expected when multi_region_failover.enabled=false")
	assert.NotContains(t, requestedHosts[0], "mrf", "MRF domain must not receive traffic when enabled=false")
}

func TestOTelSyncForwarder_sendHTTPTransactions_SendsMRFWhenFailoverMetricsEnabled(t *testing.T) {
	var requestedHosts []string
	client := &http.Client{
		Transport: handlerTransport(func(w http.ResponseWriter, r *http.Request) {
			requestedHosts = append(requestedHosts, r.Host)
			w.WriteHeader(http.StatusOK)
		}),
	}
	f := newTestOTelSyncForwarderWithMRF(t, client, true /* enabled */, true /* failover_metrics enabled */)

	payload := transaction.BytesPayloads{
		transaction.NewBytesPayloadWithoutMetaData([]byte(`{}`)),
	}
	require.NoError(t, f.SubmitV1Series(payload, http.Header{}))

	// Both primary and MRF domains should receive traffic.
	require.Len(t, requestedHosts, 2, "two requests expected when MRF enabled: primary and MRF domain")
	mrfHits := 0
	for _, h := range requestedHosts {
		if strings.Contains(h, "mrf") {
			mrfHits++
		}
	}
	assert.Equal(t, 1, mrfHits, "MRF domain should receive exactly one request when both MRF flags are true")
}
