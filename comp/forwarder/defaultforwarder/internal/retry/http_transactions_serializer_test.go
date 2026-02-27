// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package retry

import (
	"fmt"
	"net/http"
	"reflect"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	"github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder/resolver"
	"github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder/transaction"
	"github.com/DataDog/datadog-agent/pkg/config/utils"
)

const apiKey1 = "apiKey1"
const apiKey2 = "apiKey2"
const apiKey3 = "apiKey3"
const apiKey4 = "apiKey4"
const apiKey5 = "apiKey5"
const domain = "domain"
const vectorDomain = "vectorDomain"

func TestHTTPSerializeDeserialize(t *testing.T) {
	r, err := resolver.NewSingleDomainResolver(domain, []utils.APIKeys{utils.NewAPIKeys("path", apiKey1, apiKey2)})
	require.NoError(t, err)
	runTestHTTPSerializeDeserializeWithResolver(t, domain, r)
}
func TestHTTPSerializeDeserializeWithResolverOverride(t *testing.T) {
	r, err := resolver.NewMultiDomainResolver(domain, []utils.APIKeys{utils.NewAPIKeys("path", apiKey1, apiKey2)})
	require.NoError(t, err)
	r.RegisterAlternateDestination(vectorDomain, "name", resolver.Vector)
	runTestHTTPSerializeDeserializeWithResolver(t, vectorDomain, r)
}

func runTestHTTPSerializeDeserializeWithResolver(t *testing.T, d string, r resolver.DomainResolver) {
	a := assert.New(t)
	tr := createHTTPTransactionTests(d)
	log := logmock.New(t)
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
	log := logmock.New(t)
	r, err := resolver.NewSingleDomainResolver(domain, nil)
	require.NoError(t, err)
	serializer := NewHTTPTransactionsSerializer(log, r)

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
	log := logmock.New(t)
	res, err := resolver.NewSingleDomainResolver(domain, []utils.APIKeys{utils.NewAPIKeys("path", apiKey1, apiKey2)})
	require.NoError(t, err)
	serializer := NewHTTPTransactionsSerializer(log, res)

	r.NoError(serializer.Add(createHTTPTransactionWithHeaderTests(http.Header{"Key": []string{apiKey1}}, domain)))
	r.NoError(serializer.Add(createHTTPTransactionWithHeaderTests(http.Header{"Key": []string{apiKey2}}, domain)))
	bytes, err := serializer.GetBytesAndReset()
	r.NoError(err)

	_, errorCount, err := serializer.Deserialize(bytes)
	r.NoError(err)
	r.Equal(0, errorCount)

	res, err = resolver.NewSingleDomainResolver(domain, []utils.APIKeys{utils.NewAPIKeys("path", apiKey1)})
	require.NoError(t, err)
	serializerMissingAPIKey := NewHTTPTransactionsSerializer(log, res)
	_, errorCount, err = serializerMissingAPIKey.Deserialize(bytes)
	r.NoError(err)
	r.Equal(1, errorCount)
}

func TestHTTPTransactionSerializerUpdateAPIKey(t *testing.T) {
	r := require.New(t)
	log := logmock.New(t)
	res, err := resolver.NewSingleDomainResolver(domain, []utils.APIKeys{utils.NewAPIKeys("additional_endpoints", apiKey1, apiKey2)})
	r.NoError(err)

	serializer := NewHTTPTransactionsSerializer(log, res)

	r.NoError(serializer.Add(createHTTPTransactionWithHeaderTests(http.Header{"Key": []string{apiKey1}}, domain)))
	bytes, err := serializer.GetBytesAndReset()
	r.NoError(err)

	r.NotContains(string(bytes), apiKey1, "Serialized data should not contain %s", apiKey1)

	// Update the API keys.
	res.UpdateAPIKeys("additional_endpoints", []utils.APIKeys{utils.NewAPIKeys("additional_endpoints", apiKey4, apiKey2, apiKey3)})

	r.NoError(serializer.Add(createHTTPTransactionWithHeaderTests(http.Header{"Key": []string{apiKey3}}, domain)))
	bytes, err = serializer.GetBytesAndReset()
	r.NoError(err)

	// New API key should be scrubbed.
	r.NotContains(string(bytes), apiKey3, "Serialized data should not contain %s", apiKey3)

	// Ensure it can be restored
	transactions, _, err := serializer.Deserialize(bytes)
	r.NoError(err)
	r.Contains(transactions[0].(*transaction.HTTPTransaction).Headers["Key"], apiKey3)
}

func TestHTTPTransactionSerializerUpdateDedupedAPIKey(t *testing.T) {
	r := require.New(t)
	log := logmock.New(t)

	// apiKey1 is duplicated.
	res, err := resolver.NewSingleDomainResolver(domain, []utils.APIKeys{
		utils.NewAPIKeys("api_key", apiKey1),
		utils.NewAPIKeys("additional_endpoints", apiKey1, apiKey2),
	})
	r.NoError(err)

	serializer := NewHTTPTransactionsSerializer(log, res)

	r.NoError(serializer.Add(createHTTPTransactionWithHeaderTests(http.Header{"Key": []string{apiKey1}}, domain)))
	bytes, err := serializer.GetBytesAndReset()
	r.NoError(err)

	r.NotContains(string(bytes), apiKey1, "Serialized data should not contain %s", apiKey1)

	// Update the API keys, there are now no duplicates.
	res.UpdateAPIKeys("api_key", []utils.APIKeys{utils.NewAPIKeys("api_key", apiKey3)})
	res.UpdateAPIKeys("additional_endpoints", []utils.APIKeys{utils.NewAPIKeys("additional_endpoints", apiKey4, apiKey5)})

	// When this transaction is restored, what was apiKey1 could now be either apiKey3 or apiKey4
	transactions, _, err := serializer.Deserialize(bytes)
	r.NoError(err)
	r.Contains(transactions[0].(*transaction.HTTPTransaction).Headers["Key"], apiKey3)
}

func TestHTTPTransactionSerializerUpdateAPIKeyBeforeSerializing(t *testing.T) {
	r := require.New(t)
	log := logmock.New(t)
	res, err := resolver.NewSingleDomainResolver(domain, []utils.APIKeys{utils.NewAPIKeys("additional_endpoints", apiKey1, apiKey2)})
	r.NoError(err)

	serializer := NewHTTPTransactionsSerializer(log, res)
	txn := createHTTPTransactionWithHeaderTests(http.Header{"Key": []string{apiKey1}}, domain)

	// Update the API keys.
	res.UpdateAPIKeys("additional_endpoints", []utils.APIKeys{utils.NewAPIKeys("additional_endpoints", apiKey4, apiKey2)})

	r.NoError(serializer.Add(txn))

	bytes, err := serializer.GetBytesAndReset()
	r.NoError(err)

	r.NotContains(string(bytes), apiKey1, "Serialized data should not contain %s", apiKey1)

	transactions, _, err := serializer.Deserialize(bytes)
	r.NoError(err)
	r.Contains(transactions[0].(*transaction.HTTPTransaction).Headers["Key"], apiKey4)
}

func TestHTTPTransactionFieldsCount(t *testing.T) {
	tr := transaction.HTTPTransaction{}
	transactionType := reflect.TypeOf(tr)
	assert.Equalf(t, 15, transactionType.NumField(),
		"A field was added or removed from HTTPTransaction. "+
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
	tr.Destination = transaction.PrimaryOnly
	return tr
}

func assertTransactionEqual(a *assert.Assertions, tr1 *transaction.HTTPTransaction, tr2 *transaction.HTTPTransaction) {
	a.Equal(tr1.Domain, tr2.Domain)
	a.Equal(tr1.Endpoint, tr2.Endpoint)
	a.EqualValues(tr1.Headers, tr2.Headers)
	a.Equal(tr1.Retryable, tr2.Retryable)
	a.Equal(tr1.Priority, tr2.Priority)
	a.Equal(tr1.ErrorCount, tr2.ErrorCount)
	a.Equal(tr1.Destination, tr2.Destination)
	a.Equal(tr1.APIKeyIndex, tr2.APIKeyIndex)

	a.NotNil(tr1.Payload)
	a.NotNil(tr2.Payload)
	a.Equal(*tr1.Payload, *tr2.Payload)

	// Ignore monotonic clock
	a.Equal(tr1.CreatedAt.Format(time.RFC3339), tr2.CreatedAt.Format(time.RFC3339))
}

// TestDeserializedTransactionHasResolver verifies that deserialized transactions have a Resolver set,
// so Authorize() can be called without panicking.
func TestDeserializedTransactionHasResolver(t *testing.T) {
	log := logmock.New(t)
	res, err := resolver.NewSingleDomainResolver(domain, []utils.APIKeys{utils.NewAPIKeys("path", apiKey1, apiKey2)})
	require.NoError(t, err)
	serializer := NewHTTPTransactionsSerializer(log, res)

	tr := createHTTPTransactionTests(domain)
	require.NoError(t, serializer.Add(tr))
	bytes, err := serializer.GetBytesAndReset()
	require.NoError(t, err)

	txns, errorCount, err := serializer.Deserialize(bytes)
	require.NoError(t, err)
	require.Equal(t, 0, errorCount)
	require.Len(t, txns, 1)

	deserialized := txns[0].(*transaction.HTTPTransaction)
	assert.NotNil(t, deserialized.Resolver, "Resolver must be set on deserialized transactions so Authorize() can be called")
}

// TestDeserializedTransactionAPIKeyIndexPreserved verifies that the APIKeyIndex stored during
// serialization is correctly restored so the transaction uses the same API key after restart.
func TestDeserializedTransactionAPIKeyIndexPreserved(t *testing.T) {
	log := logmock.New(t)
	res, err := resolver.NewSingleDomainResolver(domain, []utils.APIKeys{utils.NewAPIKeys("path", apiKey1, apiKey2, apiKey3)})
	require.NoError(t, err)
	serializer := NewHTTPTransactionsSerializer(log, res)

	for wantIdx := 0; wantIdx < 3; wantIdx++ {
		tr := createHTTPTransactionTests(domain)
		tr.APIKeyIndex = wantIdx

		require.NoError(t, serializer.Add(tr))
		data, err := serializer.GetBytesAndReset()
		require.NoError(t, err)

		txns, errorCount, err := serializer.Deserialize(data)
		require.NoError(t, err)
		require.Equal(t, 0, errorCount)
		require.Len(t, txns, 1)

		deserialized := txns[0].(*transaction.HTTPTransaction)
		assert.Equal(t, wantIdx, deserialized.APIKeyIndex,
			"APIKeyIndex %d should survive serialization round-trip", wantIdx)
	}
}

// TestDeserializedTransactionAuthorize verifies that calling Authorize() on a deserialized
// transaction correctly sets the DD-Api-Key header based on the stored APIKeyIndex.
func TestDeserializedTransactionAuthorize(t *testing.T) {
	log := logmock.New(t)
	res, err := resolver.NewSingleDomainResolver(domain, []utils.APIKeys{utils.NewAPIKeys("path", apiKey1, apiKey2)})
	require.NoError(t, err)
	serializer := NewHTTPTransactionsSerializer(log, res)

	for wantIdx, expectedKey := range []string{apiKey1, apiKey2} {
		tr := createHTTPTransactionTests(domain)
		tr.APIKeyIndex = wantIdx

		require.NoError(t, serializer.Add(tr))
		data, err := serializer.GetBytesAndReset()
		require.NoError(t, err)

		txns, errorCount, err := serializer.Deserialize(data)
		require.NoError(t, err)
		require.Equal(t, 0, errorCount)
		require.Len(t, txns, 1)

		deserialized := txns[0].(*transaction.HTTPTransaction)
		// Authorize() is called by internalProcess before sending; simulate that here.
		assert.NotPanics(t, deserialized.Authorize)
		assert.Equal(t, expectedKey, deserialized.Headers.Get("DD-Api-Key"),
			"Authorize() should set DD-Api-Key to the key at index %d", wantIdx)
	}
}

// TestDeserializedTransactionAuthorizeMultipleKeys verifies that a batch of transactions
// with different APIKeyIndex values each gets the correct key after deserialization.
func TestDeserializedTransactionAuthorizeMultipleKeys(t *testing.T) {
	log := logmock.New(t)
	res, err := resolver.NewSingleDomainResolver(domain, []utils.APIKeys{utils.NewAPIKeys("path", apiKey1, apiKey2, apiKey3)})
	require.NoError(t, err)
	serializer := NewHTTPTransactionsSerializer(log, res)

	expectedKeys := []string{apiKey1, apiKey2, apiKey3}
	for idx := range expectedKeys {
		tr := createHTTPTransactionTests(domain)
		tr.APIKeyIndex = idx
		require.NoError(t, serializer.Add(tr))
	}

	data, err := serializer.GetBytesAndReset()
	require.NoError(t, err)

	txns, errorCount, err := serializer.Deserialize(data)
	require.NoError(t, err)
	require.Equal(t, 0, errorCount)
	require.Len(t, txns, 3)

	// Build a map of index -> key from the deserialized transactions.
	gotKeys := make(map[int]string)
	for _, txn := range txns {
		d := txn.(*transaction.HTTPTransaction)
		d.Authorize()
		gotKeys[d.APIKeyIndex] = d.Headers.Get("DD-Api-Key")
	}

	for idx, expectedKey := range expectedKeys {
		assert.Equal(t, expectedKey, gotKeys[idx],
			"transaction at index %d should authorize with key %s", idx, expectedKey)
	}
}

// TestDeserializeV2BackwardCompat verifies that transactions serialized in the old V2 format
// are correctly deserialized: the API key was stored directly in the headers (old design),
// Resolver is nil (not needed), and Authorize() is a safe no-op.
func TestDeserializeV2BackwardCompat(t *testing.T) {
	r := require.New(t)
	log := logmock.New(t)
	res, err := resolver.NewSingleDomainResolver(domain, []utils.APIKeys{utils.NewAPIKeys("additional_endpoints", apiKey1, apiKey2)})
	r.NoError(err)
	serializer := NewHTTPTransactionsSerializer(log, res)

	// Binary blob of a V2 collection (no APIKeyIndex field).
	var bytes = []byte{
		0x8, 0x2, 0x12, 0x45, 0x12, 0x18, 0xa, 0x10, 0x72, 0x6f, 0x75, 0x74, 0x65, 0xfe, 0x41, 0x50, 0x49, 0x5f, 0x4b,
		0x45, 0x59, 0xfe, 0x30, 0xfe, 0x12, 0x4, 0x6e, 0x61, 0x6d, 0x65, 0x1a, 0x14, 0xa, 0x3, 0x4b, 0x65, 0x79, 0x12,
		0xd, 0xa, 0xb, 0xfe, 0x41, 0x50, 0x49, 0x5f, 0x4b, 0x45, 0x59, 0xfe, 0x30, 0xfe, 0x22, 0x3, 0x1, 0x2, 0x3, 0x28,
		0x1, 0x30, 0xae, 0xcc, 0xc3, 0xcb, 0x6, 0x38, 0x1, 0x40, 0x1, 0x48, 0xa, 0x50, 0x1,
	}

	txns, errorCount, err := serializer.Deserialize(bytes)
	r.NoError(err)
	r.Equal(0, errorCount)
	r.Len(txns, 1)

	deserialized := txns[0].(*transaction.HTTPTransaction)

	// V2 transactions do not use the Resolver/APIKeyIndex mechanism: the API key was
	// stored directly in the headers at serialization time and is restored on deserialization.
	r.Nil(deserialized.Resolver, "V2 transactions should not have a Resolver; key is already in headers")
	r.Equal(0, deserialized.APIKeyIndex, "APIKeyIndex defaults to 0 for V2 format")

	// The API key placeholder was restored into the application header "Key".
	r.Equal(apiKey1, deserialized.Headers.Get("Key"))

	// Authorize() must be a safe no-op when Resolver is nil.
	r.NotPanics(deserialized.Authorize)
}

// TestDeserializeV2 ensures that newer agent versions can sufficiently read files created by the old agent versions.
func TestDeserializeV2(t *testing.T) {
	r := require.New(t)
	log := logmock.New(t)
	res, err := resolver.NewSingleDomainResolver(domain, []utils.APIKeys{utils.NewAPIKeys("additional_endpoints", apiKey1, apiKey2)})
	r.NoError(err)
	serializer := NewHTTPTransactionsSerializer(log, res)

	var bytes = []byte{
		0x8, 0x2, 0x12, 0x45, 0x12, 0x18, 0xa, 0x10, 0x72, 0x6f, 0x75, 0x74, 0x65, 0xfe, 0x41, 0x50, 0x49, 0x5f, 0x4b,
		0x45, 0x59, 0xfe, 0x30, 0xfe, 0x12, 0x4, 0x6e, 0x61, 0x6d, 0x65, 0x1a, 0x14, 0xa, 0x3, 0x4b, 0x65, 0x79, 0x12,
		0xd, 0xa, 0xb, 0xfe, 0x41, 0x50, 0x49, 0x5f, 0x4b, 0x45, 0x59, 0xfe, 0x30, 0xfe, 0x22, 0x3, 0x1, 0x2, 0x3, 0x28,
		0x1, 0x30, 0xae, 0xcc, 0xc3, 0xcb, 0x6, 0x38, 0x1, 0x40, 0x1, 0x48, 0xa, 0x50, 0x1, 0x12, 0x45, 0x12, 0x18, 0xa,
		0x10, 0x72, 0x6f, 0x75, 0x74, 0x65, 0xfe, 0x41, 0x50, 0x49, 0x5f, 0x4b, 0x45, 0x59, 0xfe, 0x30, 0xfe, 0x12, 0x4,
		0x6e, 0x61, 0x6d, 0x65, 0x1a, 0x14, 0xa, 0x3, 0x4b, 0x65, 0x79, 0x12, 0xd, 0xa, 0xb, 0xfe, 0x41, 0x50, 0x49,
		0x5f, 0x4b, 0x45, 0x59, 0xfe, 0x31, 0xfe, 0x22, 0x3, 0x1, 0x2, 0x3, 0x28, 0x1, 0x30, 0xae, 0xcc, 0xc3, 0xcb,
		0x6, 0x38, 0x1, 0x40, 0x1, 0x48, 0xa, 0x50, 0x1,
	}

	txns, errorCount, err := serializer.Deserialize(bytes)
	r.NoError(err)
	r.Equal(0, errorCount)
	r.Len(txns, 2)
	r.Equal(apiKey2, txns[1].(*transaction.HTTPTransaction).Headers.Get("Key"))

	apiKeys := []string{apiKey1, apiKey2}

	for i, txn := range txns {
		txn := txn.(*transaction.HTTPTransaction)
		t.Run(fmt.Sprintf("payload %d", i), func(t *testing.T) {
			r := require.New(t)
			r.Equal(apiKeys[i], txn.Headers.Get("Key"))
			r.Equal(domain, txn.Domain)
			r.Equal("route"+apiKey1, txn.Endpoint.Route)
			r.Equal([]byte{1, 2, 3}, txn.Payload.GetContent())
			r.Equal(10, txn.Payload.GetPointCount())
		})
	}
}
