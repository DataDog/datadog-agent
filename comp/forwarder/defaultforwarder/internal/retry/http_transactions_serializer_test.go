// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package retry

import (
	"net/http"
	"reflect"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/core/log"
	"github.com/DataDog/datadog-agent/comp/core/log/logimpl"
	"github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder/resolver"
	"github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder/transaction"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

const apiKey1 = "apiKey1"
const apiKey2 = "apiKey2"
const domain = "domain"
const vectorDomain = "vectorDomain"

func TestHTTPSerializeDeserialize(t *testing.T) {
	r := resolver.NewSingleDomainResolver(domain, []string{apiKey1, apiKey2})
	runTestHTTPSerializeDeserializeWithResolver(t, domain, r)
}
func TestHTTPSerializeDeserializeWithResolverOverride(t *testing.T) {
	r := resolver.NewMultiDomainResolver(domain, []string{apiKey1, apiKey2})
	r.RegisterAlternateDestination(vectorDomain, "name", resolver.Vector)
	runTestHTTPSerializeDeserializeWithResolver(t, vectorDomain, r)
}

func runTestHTTPSerializeDeserializeWithResolver(t *testing.T, d string, r resolver.DomainResolver) {
	a := assert.New(t)
	tr := createHTTPTransactionTests(d)
	log := fxutil.Test[log.Component](t, logimpl.MockModule())
	serializer := NewHTTPTransactionsSerializer(log, r)

	a.NoError(serializer.Add(tr))
	bytes, err := serializer.GetBytesAndReset()
	a.NoError(err)

	transactions, errorCount, err := serializer.Deserialize(bytes)
	a.NoError(err)
	a.Equal(0, errorCount)
	a.Len(transactions, 1)
	transactionDeserialized := transactions[0].(*transaction.HTTPTransaction)

	assertTransactionEqual(a, tr, transactionDeserialized)

	bytes, err = serializer.GetBytesAndReset()
	a.NoError(err)
	transactions, errorCount, err = serializer.Deserialize(bytes)
	a.Equal(0, errorCount)
	a.NoError(err)
	a.Len(transactions, 0)
}

func TestPartialDeserialize(t *testing.T) {
	a := assert.New(t)
	initialTransaction := createHTTPTransactionTests(domain)
	log := fxutil.Test[log.Component](t, logimpl.MockModule())
	serializer := NewHTTPTransactionsSerializer(log, resolver.NewSingleDomainResolver(domain, nil))

	a.NoError(serializer.Add(initialTransaction))
	a.NoError(serializer.Add(initialTransaction))
	bytes, err := serializer.GetBytesAndReset()
	a.NoError(err)

	for end := len(bytes); end >= 0; end-- {
		trs, _, err := serializer.Deserialize(bytes[:end])

		// If there is no error, transactions should be valid.
		if err == nil {
			for _, tr := range trs {
				assertTransactionEqual(a, tr.(*transaction.HTTPTransaction), initialTransaction)
			}
		}
	}
}

func TestHTTPTransactionSerializerMissingAPIKey(t *testing.T) {
	r := require.New(t)
	log := fxutil.Test[log.Component](t, logimpl.MockModule())
	serializer := NewHTTPTransactionsSerializer(log, resolver.NewSingleDomainResolver(domain, []string{apiKey1, apiKey2}))

	r.NoError(serializer.Add(createHTTPTransactionWithHeaderTests(http.Header{"Key": []string{apiKey1}}, domain)))
	r.NoError(serializer.Add(createHTTPTransactionWithHeaderTests(http.Header{"Key": []string{apiKey2}}, domain)))
	bytes, err := serializer.GetBytesAndReset()
	r.NoError(err)

	_, errorCount, err := serializer.Deserialize(bytes)
	r.NoError(err)
	r.Equal(0, errorCount)

	serializerMissingAPIKey := NewHTTPTransactionsSerializer(log, resolver.NewSingleDomainResolver(domain, []string{apiKey1}))
	_, errorCount, err = serializerMissingAPIKey.Deserialize(bytes)
	r.NoError(err)
	r.Equal(1, errorCount)
}

func TestHTTPTransactionFieldsCount(t *testing.T) {
	tr := transaction.HTTPTransaction{}
	transactionType := reflect.TypeOf(tr)
	assert.Equalf(t, 12, transactionType.NumField(),
		"A field was added or remove from HTTPTransaction. "+
			"You probably need to update the implementation of "+
			"HTTPTransactionsSerializer and then adjust this unit test.")
}

func createHTTPTransactionTests(domain string) *transaction.HTTPTransaction {
	return createHTTPTransactionWithHeaderTests(http.Header{"Key": []string{"value1", apiKey1, apiKey2}}, domain)
}

func createHTTPTransactionWithHeaderTests(header http.Header, domain string) *transaction.HTTPTransaction {
	payload := []byte{1, 2, 3}
	tr := transaction.NewHTTPTransaction()
	tr.Domain = domain
	tr.Endpoint = transaction.Endpoint{Route: "route" + apiKey1, Name: "name"}
	tr.Headers = header
	tr.Payload = transaction.NewBytesPayload(payload, 10)
	tr.ErrorCount = 1
	tr.CreatedAt = time.Now()
	tr.Retryable = true
	tr.Priority = transaction.TransactionPriorityHigh
	return tr
}

func assertTransactionEqual(a *assert.Assertions, tr1 *transaction.HTTPTransaction, tr2 *transaction.HTTPTransaction) {
	a.Equal(tr1.Domain, tr2.Domain)
	a.Equal(tr1.Endpoint, tr2.Endpoint)
	a.EqualValues(tr1.Headers, tr2.Headers)
	a.Equal(tr1.Retryable, tr2.Retryable)
	a.Equal(tr1.Priority, tr2.Priority)
	a.Equal(tr1.ErrorCount, tr2.ErrorCount)

	a.NotNil(tr1.Payload)
	a.NotNil(tr2.Payload)
	a.Equal(*tr1.Payload, *tr2.Payload)

	// Ignore monotonic clock
	a.Equal(tr1.CreatedAt.Format(time.RFC3339), tr2.CreatedAt.Format(time.RFC3339))
}
