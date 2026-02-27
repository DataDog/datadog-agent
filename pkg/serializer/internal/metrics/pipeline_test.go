// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.Datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package metrics

import (
	"net/http"
	"slices"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder/endpoints"
	"github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder/resolver"
	"github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder/transaction"
	"github.com/DataDog/datadog-agent/pkg/config/utils"
	"github.com/DataDog/datadog-agent/pkg/util/testutil"
)

type mockForwarder struct {
	txn []*transaction.HTTPTransaction
}

func (mf *mockForwarder) SubmitTransaction(txn *transaction.HTTPTransaction) error {
	mf.txn = append(mf.txn, txn)
	return nil
}

func TestPipelineSendValidate(t *testing.T) {
	res1, err := resolver.NewSingleDomainResolver2(utils.EndpointDescriptor{
		BaseURL: "http://example.test",
		APIKeySet: []utils.APIKeys{
			utils.NewAPIKeys("api_key", "1"),
		},
	})
	require.NoError(t, err)
	res2, err := resolver.NewSingleDomainResolver2(utils.EndpointDescriptor{
		BaseURL: "http://another.test",
		APIKeySet: []utils.APIKeys{
			utils.NewAPIKeys("api_key", "2"),
		},
	})
	require.NoError(t, err)

	ctx := PipelineContext{
		Destinations: []PipelineDestination{{
			Resolver:          res1,
			Endpoint:          endpoints.SeriesEndpoint,
			ValidationBatchID: "123",
		}, {
			Resolver:          res2,
			Endpoint:          endpoints.SeriesEndpoint,
			ValidationBatchID: "",
		}}}

	ctx.addPayload(transaction.NewBytesPayload([]byte{1}, 1))
	ctx.addPayload(transaction.NewBytesPayload([]byte{2}, 1))

	fwd := &mockForwarder{}

	err = ctx.send(fwd, http.Header{})
	require.NoError(t, err)

	// out of two destinations and two payloads, we get 4 transactions
	testutil.ElementsMatchFn(t, slices.All(fwd.txn),
		func(t require.TestingT, _ int, txn *transaction.HTTPTransaction) {
			require.Equal(t, "http://example.test", txn.Domain)
			require.Equal(t, []byte{1}, txn.Payload.GetContent())
			require.Equal(t, "123", txn.Headers.Get("x-metrics-request-id"))
			require.Equal(t, "0", txn.Headers.Get("x-metrics-request-seq"))
			require.Equal(t, "2", txn.Headers.Get("x-metrics-request-len"))
		},
		func(t require.TestingT, _ int, txn *transaction.HTTPTransaction) {
			require.Equal(t, "http://example.test", txn.Domain)
			require.Equal(t, []byte{2}, txn.Payload.GetContent())
			require.Equal(t, "123", txn.Headers.Get("x-metrics-request-id"))
			require.Equal(t, "1", txn.Headers.Get("x-metrics-request-seq"))
			require.Equal(t, "2", txn.Headers.Get("x-metrics-request-len"))
		},
		func(t require.TestingT, _ int, txn *transaction.HTTPTransaction) {
			require.Equal(t, "http://another.test", txn.Domain)
			require.Equal(t, []byte{1}, txn.Payload.GetContent())
			require.Empty(t, txn.Headers.Get("x-metrics-request-id"))
			require.Empty(t, txn.Headers.Get("x-metrics-request-payload-seq"))
			require.Empty(t, txn.Headers.Get("x-metrics-request-payload-len"))
		},
		func(t require.TestingT, _ int, txn *transaction.HTTPTransaction) {
			require.Equal(t, "http://another.test", txn.Domain)
			require.Equal(t, []byte{2}, txn.Payload.GetContent())
			require.Empty(t, txn.Headers.Get("x-metrics-request-id"))
			require.Empty(t, txn.Headers.Get("x-metrics-request-payload-seq"))
			require.Empty(t, txn.Headers.Get("x-metrics-request-payload-len"))
		},
	)
}
