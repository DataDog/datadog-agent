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
	"google.golang.org/protobuf/proto"

	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	"github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder/resolver"
	"github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder/transaction"
	"github.com/DataDog/datadog-agent/pkg/config/utils"
)

// resolveKey applies the transaction's resolver at its stored index and returns the DD-Api-Key value.
func resolveKey(txn *transaction.HTTPTransaction) string {
	h := make(http.Header)
	txn.Resolver.Authorize(txn.APIKeyIndex, h)
	return h.Get("DD-Api-Key")
}

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

// TestHTTPTransactionSerializerMissingAPIKey verifies that when a V3 transaction is
// deserialized by a resolver that has fewer keys than the stored APIKeyIndex, the
// transaction is dropped with an error rather than being returned with an invalid index.
func TestHTTPTransactionSerializerMissingAPIKey(t *testing.T) {
	r := require.New(t)
	log := logmock.New(t)
	res, err := resolver.NewSingleDomainResolver(domain, []utils.APIKeys{utils.NewAPIKeys("path", apiKey1, apiKey2)})
	require.NoError(t, err)
	serializer := NewHTTPTransactionsSerializer(log, res)

	tr := createHTTPTransactionTests(domain)
	tr.APIKeyIndex = 1 // points to apiKey2
	r.NoError(serializer.Add(tr))
	bytes, err := serializer.GetBytesAndReset()
	r.NoError(err)

	// Deserializing with the same resolver: no error, APIKeyIndex preserved.
	txns, errorCount, err := serializer.Deserialize(bytes)
	r.NoError(err)
	r.Equal(0, errorCount)
	r.Equal(1, txns[0].(*transaction.HTTPTransaction).APIKeyIndex)

	// Deserializing with a resolver that only has apiKey1: the transaction is dropped
	// because APIKeyIndex 1 is out of range for a single-key resolver.
	res2, err := resolver.NewSingleDomainResolver(domain, []utils.APIKeys{utils.NewAPIKeys("path", apiKey1)})
	require.NoError(t, err)
	serializerSmaller := NewHTTPTransactionsSerializer(log, res2)
	txns2, errorCount, err := serializerSmaller.Deserialize(bytes)
	r.NoError(err)
	r.Equal(1, errorCount)
	r.Empty(txns2)
}

// TestHTTPTransactionSerializerUpdateAPIKey verifies that after a key rotation the
// serialized bytes do not contain the new API key in plaintext and that the correct
// APIKeyIndex is preserved across a round-trip.
func TestHTTPTransactionSerializerUpdateAPIKey(t *testing.T) {
	r := require.New(t)
	log := logmock.New(t)
	res, err := resolver.NewSingleDomainResolver(domain, []utils.APIKeys{utils.NewAPIKeys("additional_endpoints", apiKey1, apiKey2)})
	r.NoError(err)

	serializer := NewHTTPTransactionsSerializer(log, res)

	tr := createHTTPTransactionTests(domain)
	tr.APIKeyIndex = 0 // apiKey1
	r.NoError(serializer.Add(tr))
	bytes, err := serializer.GetBytesAndReset()
	r.NoError(err)
	r.NotContains(string(bytes), apiKey1, "Serialized data should not contain %s", apiKey1)

	// Update the API keys so index 1 now resolves to apiKey2 (unchanged).
	res.UpdateAPIKeys("additional_endpoints", []utils.APIKeys{utils.NewAPIKeys("additional_endpoints", apiKey4, apiKey2, apiKey3)})

	tr2 := createHTTPTransactionTests(domain)
	tr2.APIKeyIndex = 2 // apiKey3
	r.NoError(serializer.Add(tr2))
	bytes, err = serializer.GetBytesAndReset()
	r.NoError(err)
	r.NotContains(string(bytes), apiKey3, "Serialized data should not contain %s", apiKey3)

	transactions, _, err := serializer.Deserialize(bytes)
	r.NoError(err)
	r.Equal(2, transactions[0].(*transaction.HTTPTransaction).APIKeyIndex)
}

// TestHTTPTransactionSerializerUpdateDedupedAPIKey verifies that the stored APIKeyIndex
// survives a key-rotation round-trip even when keys were previously deduplicated.
func TestHTTPTransactionSerializerUpdateDedupedAPIKey(t *testing.T) {
	r := require.New(t)
	log := logmock.New(t)

	// apiKey1 is duplicated across two config paths.
	res, err := resolver.NewSingleDomainResolver(domain, []utils.APIKeys{
		utils.NewAPIKeys("api_key", apiKey1),
		utils.NewAPIKeys("additional_endpoints", apiKey1, apiKey2),
	})
	r.NoError(err)

	serializer := NewHTTPTransactionsSerializer(log, res)

	tr := createHTTPTransactionTests(domain)
	tr.APIKeyIndex = 0 // points to the first deduped key (apiKey1)
	r.NoError(serializer.Add(tr))
	bytes, err := serializer.GetBytesAndReset()
	r.NoError(err)
	r.NotContains(string(bytes), apiKey1, "Serialized data should not contain %s", apiKey1)

	// Rotate keys so there are no longer any duplicates.
	res.UpdateAPIKeys("api_key", []utils.APIKeys{utils.NewAPIKeys("api_key", apiKey3)})
	res.UpdateAPIKeys("additional_endpoints", []utils.APIKeys{utils.NewAPIKeys("additional_endpoints", apiKey4, apiKey5)})

	// The stored index (0) is preserved; the actual key resolved at send time may differ.
	transactions, _, err := serializer.Deserialize(bytes)
	r.NoError(err)
	r.Equal(0, transactions[0].(*transaction.HTTPTransaction).APIKeyIndex)
}

// TestHTTPTransactionSerializerUpdateAPIKeyBeforeSerializing verifies that when API keys are
// rotated between transaction creation and serialization, the stored APIKeyIndex is preserved
// correctly across the round-trip.
func TestHTTPTransactionSerializerUpdateAPIKeyBeforeSerializing(t *testing.T) {
	r := require.New(t)
	log := logmock.New(t)
	res, err := resolver.NewSingleDomainResolver(domain, []utils.APIKeys{utils.NewAPIKeys("additional_endpoints", apiKey1, apiKey2)})
	r.NoError(err)

	serializer := NewHTTPTransactionsSerializer(log, res)

	txn := createHTTPTransactionTests(domain)
	txn.APIKeyIndex = 0 // originally points to apiKey1

	// Rotate keys before calling Add (apiKey4 replaces apiKey1 at index 0).
	res.UpdateAPIKeys("additional_endpoints", []utils.APIKeys{utils.NewAPIKeys("additional_endpoints", apiKey4, apiKey2)})

	r.NoError(serializer.Add(txn))
	bytes, err := serializer.GetBytesAndReset()
	r.NoError(err)
	r.NotContains(string(bytes), apiKey1, "Serialized data should not contain %s", apiKey1)

	transactions, _, err := serializer.Deserialize(bytes)
	r.NoError(err)
	r.Equal(0, transactions[0].(*transaction.HTTPTransaction).APIKeyIndex)
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
	return createHTTPTransactionWithHeaderTests(http.Header{"Key": []string{"value1"}}, domain)
}

func createHTTPTransactionWithHeaderTests(header http.Header, domain string) *transaction.HTTPTransaction {
	payload := []byte{1, 2, 3}
	tr := transaction.NewHTTPTransaction()
	tr.Domain = domain
	tr.Endpoint = transaction.Endpoint{Route: "route", Name: "name"}
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
		assert.Empty(t, deserialized.Headers.Get("DD-Api-Key"), "t.Headers must never hold the API key")
		assert.Equal(t, expectedKey, resolveKey(deserialized),
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
		assert.Empty(t, d.Headers.Get("DD-Api-Key"), "t.Headers must never hold the API key")
		gotKeys[d.APIKeyIndex] = resolveKey(d)
	}

	for idx, expectedKey := range expectedKeys {
		assert.Equal(t, expectedKey, gotKeys[idx],
			"transaction at index %d should authorize with key %s", idx, expectedKey)
	}
}

// TestDeserializeV2BackwardCompat verifies that transactions serialized in the old V2 format
// are correctly deserialized under the new design: the placeholder index is extracted from
// the stored route/headers and written to APIKeyIndex, the Resolver is set, and
// Authorize() applies the correct key without it being stored in Headers.
func TestDeserializeV2BackwardCompat(t *testing.T) {
	r := require.New(t)
	log := logmock.New(t)
	res, err := resolver.NewSingleDomainResolver(domain, []utils.APIKeys{utils.NewAPIKeys("additional_endpoints", apiKey1, apiKey2)})
	r.NoError(err)
	serializer := NewHTTPTransactionsSerializer(log, res)

	// Binary blob of a V2 collection (no APIKeyIndex proto field; placeholder index 0 embedded
	// in both the route bytes and the "Key" header value).
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

	// The placeholder index 0 is extracted and written to APIKeyIndex; Resolver is set so
	// Authorize() can apply the key at send time.
	r.NotNil(deserialized.Resolver, "V2 transactions should have Resolver set so Authorize() works")
	r.Equal(0, deserialized.APIKeyIndex, "placeholder index 0 maps to APIKeyIndex 0 for V2")

	// The placeholder has been stripped from the headers; the raw key is not present.
	r.Empty(deserialized.Headers.Get("Key"), "Headers must not contain the API key value")

	// Authorize() applies the key at index 0 (apiKey1).
	r.NotPanics(func() {
		r.Equal(apiKey1, resolveKey(deserialized))
	})
}

// TestExtractPlaceholderIndex verifies the low-level token parser.
func TestExtractPlaceholderIndex(t *testing.T) {
	cases := []struct {
		input     string
		wantIdx   int
		wantFound bool
	}{
		{fmt.Sprintf(placeHolderFormat, 0), 0, true},
		{fmt.Sprintf(placeHolderFormat, 7), 7, true},
		{"route" + fmt.Sprintf(placeHolderFormat, 3), 3, true},
		{"no placeholder here", 0, false},
		{"", 0, false},
	}
	for _, c := range cases {
		idx, found := extractPlaceholderIndex(c.input)
		assert.Equal(t, c.wantFound, found, "input: %q", c.input)
		if found {
			assert.Equal(t, c.wantIdx, idx, "input: %q", c.input)
		}
	}
}

// TestV2IndexExtraction verifies that a V2 transaction round-trips with the correct
// APIKeyIndex derived from the embedded placeholder rather than a restored key value.
func TestV2IndexExtraction(t *testing.T) {
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
			"V3 round-trip: APIKeyIndex %d must survive serialization", wantIdx)
		assert.NotNil(t, deserialized.Resolver)
	}
}

// TestV1IndexExtraction verifies that a V1-format transaction (sorted key order) is mapped
// to the correct APIKeyIndex in the current (unsorted) resolver key list.
func TestV1IndexExtraction(t *testing.T) {
	log := logmock.New(t)
	// Keys are deliberately out of alphabetical order so sorted order differs from config order.
	// Config order (deduped): [apiKey3, apiKey1, apiKey2]
	// Sorted order:           [apiKey1, apiKey2, apiKey3]
	res, err := resolver.NewSingleDomainResolver(domain, []utils.APIKeys{
		utils.NewAPIKeys("path", apiKey3, apiKey1, apiKey2),
	})
	require.NoError(t, err)

	serializer := NewHTTPTransactionsSerializer(log, res)

	// Build a minimal V1 proto blob by hand: version=1, one transaction whose route
	// contains placeholder index N (sorted order).
	buildV1Blob := func(sortedIdx int) []byte {
		ph := fmt.Sprintf(placeHolderFormat, sortedIdx)
		col := HttpTransactionProtoCollection{
			Version: 1,
			Values: []*HttpTransactionProto{
				{
					Endpoint:    &EndpointProto{Route: []byte("route" + ph), Name: "name"},
					Headers:     map[string]*HeaderValuesProto{},
					Payload:     []byte{1, 2, 3},
					PointCount:  10,
					Retryable:   true,
					Destination: TransactionDestinationProto_PRIMARY_ONLY,
				},
			},
		}
		b, _ := proto.Marshal(&col)
		return b
	}

	// sorted index 0 = apiKey1; apiKey1 is at position 1 in the config list.
	// sorted index 1 = apiKey2; apiKey2 is at position 2 in the config list.
	// sorted index 2 = apiKey3; apiKey3 is at position 0 in the config list.
	wantCurrentIdx := []int{1, 2, 0}

	for sortedIdx, wantIdx := range wantCurrentIdx {
		data := buildV1Blob(sortedIdx)
		txns, errorCount, err := serializer.Deserialize(data)
		require.NoError(t, err, "sorted index %d", sortedIdx)
		require.Equal(t, 0, errorCount)
		require.Len(t, txns, 1)

		deserialized := txns[0].(*transaction.HTTPTransaction)
		assert.Equal(t, wantIdx, deserialized.APIKeyIndex,
			"V1 sorted index %d should map to current index %d", sortedIdx, wantIdx)
		assert.NotNil(t, deserialized.Resolver)

		// Authorize() should apply the correct key.
		dedupedKeys := res.GetAPIKeys() // [apiKey3, apiKey1, apiKey2]
		assert.Equal(t, dedupedKeys[wantIdx], resolveKey(deserialized),
			"Authorize() key at current index %d", wantIdx)
	}
}

// TestV2IndexFromHeaderPlaceholder verifies that when the route has no placeholder but a
// header value does, the index is still extracted correctly (V2 format).
func TestV2IndexFromHeaderPlaceholder(t *testing.T) {
	log := logmock.New(t)
	res, err := resolver.NewSingleDomainResolver(domain, []utils.APIKeys{utils.NewAPIKeys("path", apiKey1, apiKey2)})
	require.NoError(t, err)

	serializer := NewHTTPTransactionsSerializer(log, res)

	ph1 := fmt.Sprintf(placeHolderFormat, 1) // points to apiKey2
	col := HttpTransactionProtoCollection{
		Version: 2,
		Values: []*HttpTransactionProto{
			{
				// Route has no placeholder; placeholder is only in a header value.
				Endpoint: &EndpointProto{Route: []byte("route"), Name: "name"},
				Headers: map[string]*HeaderValuesProto{
					"X-Custom": {Values: [][]byte{[]byte(ph1)}},
				},
				Payload:     []byte{1},
				Retryable:   true,
				Destination: TransactionDestinationProto_ALL_REGIONS,
			},
		},
	}
	data, err := proto.Marshal(&col)
	require.NoError(t, err)

	txns, errorCount, err := serializer.Deserialize(data)
	require.NoError(t, err)
	require.Equal(t, 0, errorCount)
	require.Len(t, txns, 1)

	deserialized := txns[0].(*transaction.HTTPTransaction)
	assert.Equal(t, 1, deserialized.APIKeyIndex)
	assert.NotNil(t, deserialized.Resolver)
	assert.Empty(t, deserialized.Headers.Get("X-Custom"), "placeholder must be stripped from header")
}

// TestDeserializeV2 ensures that newer agent versions can read files created by old agent
// versions (V2 format). The placeholder index is extracted from the stored data, written to
// APIKeyIndex, and Authorize() applies the correct key.
func TestDeserializeV2(t *testing.T) {
	r := require.New(t)
	log := logmock.New(t)
	res, err := resolver.NewSingleDomainResolver(domain, []utils.APIKeys{utils.NewAPIKeys("additional_endpoints", apiKey1, apiKey2)})
	r.NoError(err)
	serializer := NewHTTPTransactionsSerializer(log, res)

	// Two V2 transactions: first embeds placeholder index 0, second embeds index 1.
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

	expectedIndices := []int{0, 1}
	expectedKeys := []string{apiKey1, apiKey2}

	for i, txn := range txns {
		txn := txn.(*transaction.HTTPTransaction)
		t.Run(fmt.Sprintf("payload %d", i), func(t *testing.T) {
			r := require.New(t)
			r.Equal(expectedIndices[i], txn.APIKeyIndex)
			r.NotNil(txn.Resolver, "Resolver must be set for V2 transactions")
			// The placeholder has been stripped; the raw key is not in headers.
			r.Empty(txn.Headers.Get("Key"), "Headers must not contain the API key value")
			// Authorize() applies the correct key.
			r.Equal(expectedKeys[i], resolveKey(txn))
			r.Equal(domain, txn.Domain)
			// Placeholder is stripped from the route too.
			r.Equal("route", txn.Endpoint.Route)
			r.Equal([]byte{1, 2, 3}, txn.Payload.GetContent())
			r.Equal(10, txn.Payload.GetPointCount())
		})
	}
}
