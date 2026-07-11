// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

// Package defaultforwardernoop provides a no-op implementation of the defaultforwarder component.
package defaultforwardernoop

import (
	"context"
	"net/http"

	defaultforwarderdef "github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder/def"
	"github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder/resolver"
	"github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder/transaction"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

type noopForwarder struct{}

// NewComponent returns a no-op forwarder component.
func NewComponent() defaultforwarderdef.Component {
	return &noopForwarder{}
}

// Module provides a no-op forwarder component.
func Module() fxutil.Module {
	return fxutil.Component(
		fxutil.ProvideComponentConstructor(NewComponent),
	)
}

func (f *noopForwarder) Start() error { return nil }
func (f *noopForwarder) Stop()        {}

func (f *noopForwarder) SubmitV1Series(_ transaction.BytesPayloads, _ http.Header) error {
	return nil
}

func (f *noopForwarder) SubmitV1Intake(_ transaction.BytesPayloads, _ transaction.Kind, _ http.Header) error {
	return nil
}

func (f *noopForwarder) SubmitV1CheckRuns(_ transaction.BytesPayloads, _ http.Header) error {
	return nil
}

func (f *noopForwarder) SubmitHostMetadata(_ transaction.BytesPayloads, _ http.Header) error {
	return nil
}

func (f *noopForwarder) SubmitAgentChecksMetadata(_ transaction.BytesPayloads, _ http.Header) error {
	return nil
}

func (f *noopForwarder) SubmitMetadata(_ transaction.BytesPayloads, _ http.Header) error {
	return nil
}

func (f *noopForwarder) SubmitProcessChecks(_ transaction.BytesPayloads, _ http.Header) (chan defaultforwarderdef.Response, error) {
	return nil, nil
}

func (f *noopForwarder) SubmitProcessDiscoveryChecks(_ transaction.BytesPayloads, _ http.Header) (chan defaultforwarderdef.Response, error) {
	return nil, nil
}

func (f *noopForwarder) SubmitRTProcessChecks(_ transaction.BytesPayloads, _ http.Header) (chan defaultforwarderdef.Response, error) {
	return nil, nil
}

func (f *noopForwarder) SubmitContainerChecks(_ transaction.BytesPayloads, _ http.Header) (chan defaultforwarderdef.Response, error) {
	return nil, nil
}

func (f *noopForwarder) SubmitRTContainerChecks(_ transaction.BytesPayloads, _ http.Header) (chan defaultforwarderdef.Response, error) {
	return nil, nil
}

func (f *noopForwarder) SubmitConnectionChecks(_ transaction.BytesPayloads, _ http.Header) (chan defaultforwarderdef.Response, error) {
	return nil, nil
}

func (f *noopForwarder) SubmitOrchestratorChecks(_ transaction.BytesPayloads, _ http.Header, _ int) error {
	return nil
}

func (f *noopForwarder) SubmitOrchestratorManifests(_ transaction.BytesPayloads, _ http.Header) error {
	return nil
}

func (f *noopForwarder) SubmitV1IntakeDirect(_ context.Context, _ transaction.BytesPayloads, _ transaction.Kind, _ http.Header) error {
	return nil
}

func (f *noopForwarder) GetDomainResolvers() []resolver.DomainResolver {
	return nil
}

func (f *noopForwarder) SubmitTransaction(_ *transaction.HTTPTransaction) error {
	return nil
}

var _ defaultforwarderdef.Forwarder = (*noopForwarder)(nil)
