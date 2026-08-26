// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package defaultforwarderimpl

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	secretsmock "github.com/DataDog/datadog-agent/comp/core/secrets/mock"
	"github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder/endpoints"
	"github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder/transaction"
	mock "github.com/DataDog/datadog-agent/pkg/config/mock"
)

// notReadyResolver stands in for an authorization slot whose credential has not arrived.
type notReadyResolver struct{}

func (notReadyResolver) Authorize(_ uint, _ http.Header, _ log.Component) error {
	return transaction.ErrCredentialNotReady
}

func newWorkerForBreakerTest(t *testing.T) (*Worker, chan transaction.Transaction) {
	t.Helper()
	cfg := mock.New(t)
	logger := logmock.New(t)
	requeue := make(chan transaction.Transaction, 1)
	w := NewWorker(
		cfg, logger, secretsmock.New(t),
		make(chan transaction.Transaction), make(chan transaction.Transaction), requeue,
		newBlockedEndpoints(cfg, logger), &PointSuccessfullySentMock{},
		NewSharedConnection(logger, false, 1, cfg, nil),
	)
	return w, requeue
}

func newTransactionTo(domain string) *transaction.HTTPTransaction {
	txn := transaction.NewHTTPTransaction()
	txn.Domain = domain
	txn.Endpoint = endpoints.SeriesEndpoint
	txn.Payload = transaction.NewBytesPayloadWithoutMetaData([]byte("payload"))
	return txn
}

// The circuit breaker keys on domain+route, which every authorization slot on that domain shares.
// A slot still waiting for its delegated-auth credential must not close it: the endpoint is
// unprovisioned, not unhealthy, and blocking it throttles the slots that do have a credential. In
// practice that means one org still resolving would degrade metrics delivery for the primary org.
func TestMissingCredentialDoesNotBlockTheDomainForOtherSlots(t *testing.T) {
	w, requeue := newWorkerForBreakerTest(t)

	txn := newTransactionTo("https://app.datadoghq.com")
	txn.Resolver = notReadyResolver{}

	w.process(context.Background(), txn)

	assert.False(t, w.blockedList.isBlockForSend(txn.GetTarget(), time.Now()),
		"waiting on a credential must leave the domain open for the slots that have one")

	select {
	case <-requeue:
	default:
		t.Fatal("the transaction must be requeued so it is still there when the credential lands")
	}
}

// The guard above must not swallow real errors on the way past: a genuine delivery failure still
// has to trip the breaker, or the forwarder loses its backoff entirely.
func TestRealFailureStillBlocksTheDomain(t *testing.T) {
	w, _ := newWorkerForBreakerTest(t)

	// Nothing is listening on this port, so Process fails to connect.
	txn := newTransactionTo("http://127.0.0.1:1")

	w.process(context.Background(), txn)

	assert.True(t, w.blockedList.isBlockForSend(txn.GetTarget(), time.Now()),
		"a connection failure is an endpoint failure and must still close the breaker")
}

// Process must preserve the sentinel through its error wrapping, since that is the only thing the
// two circuit-breaker call sites can key on. Wrapping with %s instead of %w would silently restore
// the old behaviour with every test above still passing at the unit level.
func TestProcessPreservesTheCredentialSentinel(t *testing.T) {
	txn := newTransactionTo("https://app.datadoghq.com")
	txn.Resolver = notReadyResolver{}

	err := txn.Process(context.Background(), mock.New(t), logmock.New(t), secretsmock.New(t), &http.Client{}, &PointSuccessfullySentMock{})

	require.Error(t, err)
	assert.ErrorIs(t, err, transaction.ErrCredentialNotReady)
}
