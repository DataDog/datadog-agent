// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package defaultforwarder

import (
	"context"
	"fmt"
	"net/http"

	"go.uber.org/multierr"

	"github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	secrets "github.com/DataDog/datadog-agent/comp/core/secrets/def"
	"github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder/endpoints"
	"github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder/resolver"
	"github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder/transaction"
	"github.com/DataDog/datadog-agent/pkg/config/utils"
	"github.com/DataDog/datadog-agent/pkg/version"
)

// OTelSyncForwarder is a synchronous forwarder that aggregates and returns
// transaction errors instead of swallowing them. It is intended for use by the
// OTel serializer exporter so that failures surface back through ConsumeMetrics
// to the exporterhelper queue/retry layer (see OTAGENT-1024).
//
// Differences vs SyncForwarder:
//   - Errors from HTTP transactions are returned (multierr-combined) rather than logged.
//   - Each transaction is attempted once; the OTel exporterhelper drives retries.
type OTelSyncForwarder struct {
	config           config.Component
	log              log.Component
	secrets          secrets.Component
	defaultForwarder *DefaultForwarder
	client           *http.Client
}

// Compile-time check that OTelSyncForwarder implements the Forwarder interface.
var _ Forwarder = &OTelSyncForwarder{}

// NewOTelSyncForwarder returns a new synchronous, error-propagating forwarder.
// The caller supplies the *http.Client (typically built from confighttp.ClientConfig
// via ToClient so OTel-native HTTP settings are honored).
func NewOTelSyncForwarder(config config.Component, log log.Component, secrets secrets.Component, endpoints utils.EndpointDescriptorSet, client *http.Client) (*OTelSyncForwarder, error) {
	options, err := NewOptionsWithOPW(config, log, endpoints)
	if err != nil {
		return nil, err
	}
	options.Secrets = secrets
	return &OTelSyncForwarder{
		config:           config,
		log:              log,
		secrets:          secrets,
		defaultForwarder: NewDefaultForwarder(config, log, options),
		client:           client,
	}, nil
}

// Start is a no-op; the OTel exporterhelper owns the lifecycle.
func (f *OTelSyncForwarder) Start() error { return nil }

// Stop is a no-op; the OTel exporterhelper owns the lifecycle.
func (f *OTelSyncForwarder) Stop() {}

// mrfChecker is satisfied by domainResolver so we can gate MRF transactions
// without importing the resolver package.
type mrfChecker interface{ IsMRF() bool }

// sendHTTPTransactions sends each transaction synchronously and returns a
// multierr-combined error covering all failures. Unlike SyncForwarder, no
// per-transaction retry is performed: retries are the OTel layer's job.
//
// HTTPTransaction.Process silently drops certain permanent failures (400, 413,
// and 403 without a secret refresh) by returning nil. We wrap the completion
// handler to intercept those status codes and surface them as errors so the OTel
// exporterhelper can count them and drive retries or permanent-error handling.
//
// MRF gating: non-metadata transactions for MRF resolvers are skipped unless
// multi_region_failover.failover_metrics is enabled, mirroring the guard in
// domainForwarder.shouldSendHTTPTransaction.
func (f *OTelSyncForwarder) sendHTTPTransactions(ctx context.Context, transactions []*transaction.HTTPTransaction) error {
	var errs error
	for _, txn := range transactions {
		if dr, ok := txn.Resolver.(mrfChecker); ok && dr.IsMRF() {
			if txn.Kind != transaction.Metadata && !f.config.GetBool("multi_region_failover.failover_metrics") {
				continue
			}
		}
		var permanentErr error
		origHandler := txn.CompletionHandler
		txn.CompletionHandler = func(tx *transaction.HTTPTransaction, statusCode int, body []byte, err error) {
			if err == nil && (statusCode == 400 || statusCode == 413 || statusCode == 403) {
				permanentErr = fmt.Errorf("permanent intake error %d: dropping transaction to %s%s", statusCode, tx.Domain, tx.Endpoint.Route)
			}
			origHandler(tx, statusCode, body, err)
		}
		if err := txn.Process(ctx, f.config, f.log, f.secrets, f.client, nil); err != nil {
			errs = multierr.Append(errs, err)
		} else if permanentErr != nil {
			errs = multierr.Append(errs, permanentErr)
		}
	}
	return errs
}

// SubmitTransaction sends a single transaction synchronously. This is the main
// path used by pkg/serializer's v2 pipelines.
// Mirrors DefaultForwarder.SubmitTransaction header injection so agent-version,
// user-agent, and allow-arbitrary-tag headers are present on v2 series payloads.
func (f *OTelSyncForwarder) SubmitTransaction(txn *transaction.HTTPTransaction) error {
	txn.Headers.Set(versionHTTPHeaderKey, version.AgentVersion)
	txn.Headers.Set(useragentHTTPHeaderKey, "datadog-agent/"+version.AgentVersion)
	if f.config.GetBool("allow_arbitrary_tags") {
		txn.Headers.Set(arbitraryTagHTTPHeaderKey, "true")
	}
	return f.sendHTTPTransactions(context.Background(), []*transaction.HTTPTransaction{txn})
}

// SubmitV1Series sends timeseries to the v1 endpoint.
func (f *OTelSyncForwarder) SubmitV1Series(payload transaction.BytesPayloads, extra http.Header) error {
	transactions := f.defaultForwarder.createHTTPTransactions(endpoints.V1SeriesEndpoint, payload, transaction.Series, extra)
	return f.sendHTTPTransactions(context.Background(), transactions)
}

// SubmitV1Intake sends payloads to the universal `/intake/` endpoint.
func (f *OTelSyncForwarder) SubmitV1Intake(payload transaction.BytesPayloads, kind transaction.Kind, extra http.Header) error {
	transactions := f.defaultForwarder.createHTTPTransactions(endpoints.V1IntakeEndpoint, payload, kind, extra)
	// the intake endpoint requires the Content-Type header to be set
	for _, t := range transactions {
		t.Headers.Set("Content-Type", "application/json")
	}
	return f.sendHTTPTransactions(context.Background(), transactions)
}

// SubmitV1IntakeDirect sends payloads synchronously to the universal `/intake/` endpoint.
// Unlike SubmitV1Intake, the caller's context is honored so cancellations and
// deadlines propagate to the underlying HTTP request.
func (f *OTelSyncForwarder) SubmitV1IntakeDirect(ctx context.Context, payload transaction.BytesPayloads, kind transaction.Kind, extra http.Header) error {
	transactions := f.defaultForwarder.createHTTPTransactions(endpoints.V1IntakeEndpoint, payload, kind, extra)
	for _, t := range transactions {
		t.Headers.Set("Content-Type", "application/json")
	}
	return f.sendHTTPTransactions(ctx, transactions)
}

// SubmitV1CheckRuns sends service checks to the v1 endpoint.
func (f *OTelSyncForwarder) SubmitV1CheckRuns(payload transaction.BytesPayloads, extra http.Header) error {
	transactions := f.defaultForwarder.createHTTPTransactions(endpoints.V1CheckRunsEndpoint, payload, transaction.CheckRuns, extra)
	return f.sendHTTPTransactions(context.Background(), transactions)
}

// SubmitHostMetadata sends a host_metadata payload.
func (f *OTelSyncForwarder) SubmitHostMetadata(payload transaction.BytesPayloads, extra http.Header) error {
	return f.SubmitV1Intake(payload, transaction.Metadata, extra)
}

// SubmitMetadata sends a metadata payload.
func (f *OTelSyncForwarder) SubmitMetadata(payload transaction.BytesPayloads, extra http.Header) error {
	return f.SubmitV1Intake(payload, transaction.Metadata, extra)
}

// SubmitAgentChecksMetadata sends an agentchecks_metadata payload.
func (f *OTelSyncForwarder) SubmitAgentChecksMetadata(payload transaction.BytesPayloads, extra http.Header) error {
	return f.SubmitV1Intake(payload, transaction.Metadata, extra)
}

// SubmitProcessChecks sends process checks.
func (f *OTelSyncForwarder) SubmitProcessChecks(payload transaction.BytesPayloads, extra http.Header) (chan Response, error) {
	return f.defaultForwarder.submitProcessLikePayload(endpoints.ProcessesEndpoint, payload, extra, true)
}

// SubmitProcessDiscoveryChecks sends process discovery checks.
func (f *OTelSyncForwarder) SubmitProcessDiscoveryChecks(payload transaction.BytesPayloads, extra http.Header) (chan Response, error) {
	return f.defaultForwarder.submitProcessLikePayload(endpoints.ProcessDiscoveryEndpoint, payload, extra, true)
}

// SubmitRTProcessChecks sends real-time process checks.
func (f *OTelSyncForwarder) SubmitRTProcessChecks(payload transaction.BytesPayloads, extra http.Header) (chan Response, error) {
	return f.defaultForwarder.submitProcessLikePayload(endpoints.RtProcessesEndpoint, payload, extra, false)
}

// SubmitContainerChecks sends container checks.
func (f *OTelSyncForwarder) SubmitContainerChecks(payload transaction.BytesPayloads, extra http.Header) (chan Response, error) {
	return f.defaultForwarder.submitProcessLikePayload(endpoints.ContainerEndpoint, payload, extra, true)
}

// SubmitRTContainerChecks sends real-time container checks.
func (f *OTelSyncForwarder) SubmitRTContainerChecks(payload transaction.BytesPayloads, extra http.Header) (chan Response, error) {
	return f.defaultForwarder.submitProcessLikePayload(endpoints.RtContainerEndpoint, payload, extra, false)
}

// SubmitConnectionChecks sends connection checks.
func (f *OTelSyncForwarder) SubmitConnectionChecks(payload transaction.BytesPayloads, extra http.Header) (chan Response, error) {
	return f.defaultForwarder.submitProcessLikePayload(endpoints.ConnectionsEndpoint, payload, extra, true)
}

// SubmitOrchestratorChecks sends orchestrator checks.
func (f *OTelSyncForwarder) SubmitOrchestratorChecks(payload transaction.BytesPayloads, extra http.Header, payloadType int) error {
	return f.defaultForwarder.SubmitOrchestratorChecks(payload, extra, payloadType)
}

// SubmitOrchestratorManifests sends orchestrator manifests.
func (f *OTelSyncForwarder) SubmitOrchestratorManifests(payload transaction.BytesPayloads, extra http.Header) error {
	return f.defaultForwarder.SubmitOrchestratorManifests(payload, extra)
}

// GetDomainResolvers returns the list of resolvers used by this forwarder.
func (f *OTelSyncForwarder) GetDomainResolvers() []resolver.DomainResolver {
	return f.defaultForwarder.GetDomainResolvers()
}
