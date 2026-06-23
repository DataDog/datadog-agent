// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package defaultforwarderimpl

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"go.uber.org/multierr"

	"github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	secrets "github.com/DataDog/datadog-agent/comp/core/secrets/def"
	defaultforwarderdef "github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder/def"
	"github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder/endpoints"
	"github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder/resolver"
	"github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder/transaction"
	"github.com/DataDog/datadog-agent/pkg/config/utils"
	"github.com/DataDog/datadog-agent/pkg/version"
)

// ErrPermanentHTTPError is the sentinel wrapped into errors returned when the
// intake responds with a permanent failure (400, 413, 403-drop). Callers at
// the OTel exporter boundary can detect this with errors.Is and convert it to
// consumererror.NewPermanent so the exporterhelper queue does not retry it.
var ErrPermanentHTTPError = errors.New("permanent intake error")

// errOTelSyncNotSupported is returned by Forwarder interface methods that are
// not used by the OTel serializer metrics path and therefore not implemented by
// OTelSyncForwarder. It replaces a delegation to the stopped embedded
// DefaultForwarder, which would return a less informative "forwarder is not
// started" error.
var errOTelSyncNotSupported = errors.New("not supported by OTelSyncForwarder: only the serializer metrics path is implemented")

// OTelSyncForwarder is a synchronous forwarder that aggregates and returns
// transaction errors instead of swallowing them. It is intended for use by the
// OTel serializer exporter so that failures surface back through ConsumeMetrics
// to the exporterhelper queue/retry layer (see OTAGENT-1024).
//
// Supported methods (the OTel serializer metrics path):
//   - SubmitTransaction, SubmitV1Series, SubmitV1Intake, SubmitV1IntakeDirect
//   - SubmitV1CheckRuns, SubmitHostMetadata, SubmitMetadata, SubmitAgentChecksMetadata
//   - GetDomainResolvers
//
// All other Forwarder interface methods (process, container, orchestrator checks)
// return errOTelSyncNotSupported; they exist only for interface compliance.
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
var _ defaultforwarderdef.Forwarder = &OTelSyncForwarder{}

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
// both multi_region_failover.enabled and multi_region_failover.failover_metrics
// are true, mirroring the steady-state guard in domainForwarder.shouldSendHTTPTransaction.
func (f *OTelSyncForwarder) sendHTTPTransactions(ctx context.Context, transactions []*transaction.HTTPTransaction) error {
	var errs error
	for _, txn := range transactions {
		if dr, ok := txn.Resolver.(mrfChecker); ok && dr.IsMRF() {
			if txn.Kind != transaction.Metadata && (!f.config.GetBool("multi_region_failover.enabled") || !f.config.GetBool("multi_region_failover.failover_metrics")) {
				continue
			}
		}
		var permanentErr error
		var responseReceived bool
		origHandler := txn.CompletionHandler
		txn.CompletionHandler = func(tx *transaction.HTTPTransaction, statusCode int, body []byte, err error) {
			if statusCode > 0 {
				responseReceived = true
			}
			if err == nil && (statusCode == 400 || statusCode == 413 || statusCode == 403) {
				permanentErr = fmt.Errorf("HTTP %d dropping transaction to %s%s: %w", statusCode, tx.Domain, tx.Endpoint.Route, ErrPermanentHTTPError)
			}
			origHandler(tx, statusCode, body, err)
		}
		if err := txn.Process(ctx, f.config, f.log, f.secrets, f.client, nil); err != nil {
			errs = multierr.Append(errs, err)
		} else if permanentErr != nil {
			errs = multierr.Append(errs, permanentErr)
		} else if !responseReceived {
			// internalProcess silently drops canceled requests (returns nil, statusCode 0).
			// Only propagate ctx.Err() when no real HTTP response was received; a post-success
			// cancellation must not turn an already-accepted payload into a reported failure.
			if ctxErr := ctx.Err(); ctxErr != nil {
				errs = multierr.Append(errs, ctxErr)
				break
			}
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

// SubmitProcessChecks is not supported; see errOTelSyncNotSupported.
func (f *OTelSyncForwarder) SubmitProcessChecks(_ transaction.BytesPayloads, _ http.Header) (chan defaultforwarderdef.Response, error) {
	return nil, errOTelSyncNotSupported
}

// SubmitProcessDiscoveryChecks is not supported; see errOTelSyncNotSupported.
func (f *OTelSyncForwarder) SubmitProcessDiscoveryChecks(_ transaction.BytesPayloads, _ http.Header) (chan defaultforwarderdef.Response, error) {
	return nil, errOTelSyncNotSupported
}

// SubmitRTProcessChecks is not supported; see errOTelSyncNotSupported.
func (f *OTelSyncForwarder) SubmitRTProcessChecks(_ transaction.BytesPayloads, _ http.Header) (chan defaultforwarderdef.Response, error) {
	return nil, errOTelSyncNotSupported
}

// SubmitContainerChecks is not supported; see errOTelSyncNotSupported.
func (f *OTelSyncForwarder) SubmitContainerChecks(_ transaction.BytesPayloads, _ http.Header) (chan defaultforwarderdef.Response, error) {
	return nil, errOTelSyncNotSupported
}

// SubmitRTContainerChecks is not supported; see errOTelSyncNotSupported.
func (f *OTelSyncForwarder) SubmitRTContainerChecks(_ transaction.BytesPayloads, _ http.Header) (chan defaultforwarderdef.Response, error) {
	return nil, errOTelSyncNotSupported
}

// SubmitConnectionChecks is not supported; see errOTelSyncNotSupported.
func (f *OTelSyncForwarder) SubmitConnectionChecks(_ transaction.BytesPayloads, _ http.Header) (chan defaultforwarderdef.Response, error) {
	return nil, errOTelSyncNotSupported
}

// SubmitOrchestratorChecks is not supported; see errOTelSyncNotSupported.
func (f *OTelSyncForwarder) SubmitOrchestratorChecks(_ transaction.BytesPayloads, _ http.Header, _ int) error {
	return errOTelSyncNotSupported
}

// SubmitOrchestratorManifests is not supported; see errOTelSyncNotSupported.
func (f *OTelSyncForwarder) SubmitOrchestratorManifests(_ transaction.BytesPayloads, _ http.Header) error {
	return errOTelSyncNotSupported
}

// GetDomainResolvers returns the list of resolvers used by this forwarder.
func (f *OTelSyncForwarder) GetDomainResolvers() []resolver.DomainResolver {
	return f.defaultForwarder.GetDomainResolvers()
}
