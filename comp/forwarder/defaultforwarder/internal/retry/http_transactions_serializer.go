// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package retry

import (
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	"github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder/resolver"
	"github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder/transaction"
	"github.com/DataDog/datadog-agent/pkg/config/utils"
	"github.com/DataDog/datadog-agent/pkg/telemetry"

	"google.golang.org/protobuf/proto"
)

const transactionsSerializerVersion = 3

// Use a non US ASCII char as a separator (Should neither appear in an HTTP header value nor in a URL).
// Note: This is not valid UTF-8, but the proto fields using it are defined as `bytes` to avoid UTF-8 validation.
const squareChar = "\xfe"
const placeHolderPrefix = squareChar + "API_KEY" + squareChar
const placeHolderFormat = placeHolderPrefix + "%v" + squareChar

var tlmV1TransactionsDeserialized = telemetry.NewCounterWithOpts("transactions", "v1deserialized", []string{}, "", telemetry.Options{DefaultMetric: true})

// HTTPTransactionsSerializer serializes Transaction instances.
// To support a new Transaction implementation, add a new
// method `func (s *HTTPTransactionsSerializer) Add(transaction NEW_TYPE) error {`
type HTTPTransactionsSerializer struct {
	log        log.Component
	collection HttpTransactionProtoCollection

	currentKeyVersion     int
	placeholderMutex      sync.RWMutex
	apiKeyToPlaceholder   []*strings.Replacer
	placeholderToAPIKey   *strings.Replacer
	placeholderToAPIKeyV1 *strings.Replacer

	resolver resolver.DomainResolver
}

// NewHTTPTransactionsSerializer creates a new instance of HTTPTransactionsSerializer
func NewHTTPTransactionsSerializer(log log.Component, resolver resolver.DomainResolver) *HTTPTransactionsSerializer {
	keys, version := resolver.GetAPIKeysInfo()
	apiKeyToPlaceholder, placeholderToAPIKey := createReplacers(keys, []*strings.Replacer{})

	dedupedKeys := resolver.GetAPIKeys()
	placeholderToAPIKeyV1 := createReplacerV1(dedupedKeys)

	return &HTTPTransactionsSerializer{
		log: log,
		collection: HttpTransactionProtoCollection{
			Version: transactionsSerializerVersion,
		},
		apiKeyToPlaceholder:   apiKeyToPlaceholder,
		placeholderToAPIKey:   placeholderToAPIKey,
		placeholderToAPIKeyV1: placeholderToAPIKeyV1,
		currentKeyVersion:     version,
		resolver:              resolver,
	}
}

// Add adds a transaction to the serializer.
// This function uses references on HTTPTransaction.Payload and HTTPTransaction.Headers
// and so the transaction must not be updated until a call to `GetBytesAndReset`.
func (s *HTTPTransactionsSerializer) Add(transaction *transaction.HTTPTransaction) error {
	if d := s.resolver.Resolve(transaction.Endpoint); transaction.Domain != d {
		// This error is not supposed to happen (Sanity check).
		return fmt.Errorf("the domain of the transaction %v does not match the domain %v", transaction.Domain, d)
	}

	priority, err := toTransactionPriorityProto(transaction.Priority)
	if err != nil {
		return err
	}

	destination, err := toTransactionDestinationProto(transaction.Destination)
	if err != nil {
		return err
	}

	var payload []byte
	var pointCount int32
	if transaction.Payload != nil {
		payload = transaction.Payload.GetContent()
		pointCount = int32(transaction.Payload.GetPointCount())
	}

	s.checkAPIKeyUpdate()

	endpoint := transaction.Endpoint
	transactionProto := HttpTransactionProto{
		// The domain is not stored on the disk for security reasons.
		// If a user can update the domain for some serialized transactions, they can replace the domain
		// by a local address like http://127.0.0.1:1234. The Agent would send the HTTP transactions to the url
		// http://127.0.0.1:1234/intake/?api_key=API_KEY which contains the API_KEY.
		Domain:      "",
		Endpoint:    &EndpointProto{Route: []byte(endpoint.Route), Name: endpoint.Name},
		Headers:     s.toHeaderProto(transaction.Headers),
		Payload:     payload,
		ErrorCount:  int64(transaction.ErrorCount),
		CreatedAt:   transaction.CreatedAt.Unix(),
		Retryable:   transaction.Retryable,
		Priority:    priority,
		PointCount:  pointCount,
		Destination: destination,
		APIKeyIndex: int32(transaction.APIKeyIndex),
	}
	s.collection.Values = append(s.collection.Values, &transactionProto)
	return nil
}

// checkAPIKeyUpdate checks if the resolver has updated it's API keys - it does this by maintaining a version
// number that increments with every update. If they have been updated, we reload the new set of keys and
// update our replacers.
func (s *HTTPTransactionsSerializer) checkAPIKeyUpdate() {
	if s.resolver.GetAPIKeyVersion() != s.currentKeyVersion {
		// API keys have been updated so we need to rebuild.
		s.placeholderMutex.Lock()
		defer s.placeholderMutex.Unlock()

		keys, version := s.resolver.GetAPIKeysInfo()
		s.apiKeyToPlaceholder, s.placeholderToAPIKey = createReplacers(keys, s.apiKeyToPlaceholder)
		s.currentKeyVersion = version
	}
}

// GetBytesAndReset returns as bytes the serialized transactions and reset
// the transaction collection.
func (s *HTTPTransactionsSerializer) GetBytesAndReset() ([]byte, error) {
	out, err := proto.Marshal(&s.collection)
	s.collection.Values = nil
	return out, err
}

// Deserialize deserializes from bytes.
func (s *HTTPTransactionsSerializer) Deserialize(bytes []byte) ([]transaction.Transaction, int, error) {
	collection := HttpTransactionProtoCollection{}

	if err := proto.Unmarshal(bytes, &collection); err != nil {
		return nil, 0, err
	}

	s.checkAPIKeyUpdate()

	var httpTransactions []transaction.Transaction
	errorCount := 0
	for _, tr := range collection.Values {
		var destination transaction.Destination
		e := tr.Endpoint

		priority, err := fromTransactionPriorityProto(tr.Priority)

		var resolverAuth transaction.Authorizer
		var apiKeyIndex int

		if err == nil {
			destination, err = fromTransactionDestinationProto(tr.Destination)
		}

		if err == nil {
			if collection.Version >= 3 {
				// Version 3+ stores the API key index directly in the proto field.
				apiKeyIndex = int(tr.APIKeyIndex)
				if apiKeyIndex >= len(s.resolver.GetAuthorizers()) {
					err = fmt.Errorf("APIKeyIndex %d is out of range (have %d keys)", apiKeyIndex, len(s.resolver.GetAuthorizers()))
				} else {
					resolverAuth = s.resolver
				}
			} else {
				// Versions 1 and 2 embedded the API key as a placeholder token
				// (\xfeAPI_KEY\xfeN\xfe) in the route and/or header values. Extract the
				// index N instead of substituting back the actual key string.
				apiKeyIndex, err = s.apiKeyIndexFromProto(tr, collection.Version)
				if err == nil {
					resolverAuth = s.resolver
				}
			}
		}

		if err != nil {
			s.log.Errorf("Error when deserializing a transaction: %v", err)
			errorCount++
			continue
		}

		route := stripPlaceholders(string(e.Route))
		headers := s.fromHeaderProto(tr.Headers)
		endpoint := transaction.Endpoint{Route: route, Name: e.Name}
		domain := s.resolver.Resolve(endpoint)

		tr := transaction.HTTPTransaction{
			Domain:         domain,
			Endpoint:       endpoint,
			Headers:        headers,
			Payload:        transaction.NewBytesPayload(tr.Payload, int(tr.GetPointCount())),
			ErrorCount:     int(tr.ErrorCount),
			CreatedAt:      time.Unix(tr.CreatedAt, 0),
			Retryable:      tr.Retryable,
			StorableOnDisk: true,
			Priority:       priority,
			Destination:    destination,
			APIKeyIndex:    apiKeyIndex,
			Resolver:       resolverAuth,
		}
		tr.SetDefaultHandlers()
		httpTransactions = append(httpTransactions, &tr)
	}
	return httpTransactions, errorCount, nil
}

// extractPlaceholderIndex finds the first \xfeAPI_KEY\xfeN\xfe token in str and returns
// the integer index N. Returns (0, false) when no token is present.
func extractPlaceholderIndex(str string) (int, bool) {
	idx := strings.Index(str, placeHolderPrefix)
	if idx == -1 {
		return 0, false
	}
	rest := str[idx+len(placeHolderPrefix):]
	end := strings.Index(rest, squareChar)
	if end == -1 {
		return 0, false
	}
	n, err := strconv.Atoi(rest[:end])
	if err != nil {
		return 0, false
	}
	return n, true
}

// stripPlaceholders removes all \xfeAPI_KEY\xfeN\xfe tokens from str.
func stripPlaceholders(str string) string {
	for {
		idx := strings.Index(str, placeHolderPrefix)
		if idx == -1 {
			break
		}
		rest := str[idx+len(placeHolderPrefix):]
		end := strings.Index(rest, squareChar)
		if end == -1 {
			break
		}
		str = str[:idx] + rest[end+len(squareChar):]
	}
	return str
}

// apiKeyIndexFromProto extracts the APIKeyIndex for proto versions 1 and 2 by scanning the
// stored header values and route bytes for a \xfeAPI_KEY\xfeN\xfe placeholder and returning
// the resolved index N. For V1, N is a position in the alphabetically sorted key list and is
// converted to the current unsorted position. For V2, N is returned directly.
//
// Any header with the placeholder is removed.
func (s *HTTPTransactionsSerializer) apiKeyIndexFromProto(tr *HttpTransactionProto, protoVersion int32) (int, error) {
	index := -1
	for headerKey, headerValues := range tr.Headers {
		for _, v := range headerValues.Values {
			if placeholderIdx, found := extractPlaceholderIndex(string(v)); found {
				// The header should not exist in the transaction after we have
				// extracted the API key. We continue scanning through the headers
				// after finding a key in the (should not happen) case that there
				// are multiple API key headers.
				delete(tr.Headers, headerKey)
				if index == -1 {
					var err error
					index, err = s.resolvePlaceholderIndex(placeholderIdx, protoVersion)
					if err != nil {
						return -1, err
					}
				}
			}
		}
	}

	if index != -1 {
		return index, nil
	}

	// Fall back to the route.
	if placeholderIdx, found := extractPlaceholderIndex(string(tr.Endpoint.Route)); found {
		return s.resolvePlaceholderIndex(placeholderIdx, protoVersion)
	}

	// No placeholder found; the transaction had no API key embedded (e.g. a local-domain transaction).
	return 0, nil
}

// resolvePlaceholderIndex converts a raw placeholder index to the current APIKeyIndex,
// accounting for V1's sorted-key ordering.
func (s *HTTPTransactionsSerializer) resolvePlaceholderIndex(placeholderIdx int, protoVersion int32) (int, error) {
	if protoVersion == 1 {
		return s.v1SortedIndexToCurrent(placeholderIdx)
	}
	return placeholderIdx, nil
}

// v1SortedIndexToCurrent maps a V1 sorted-order placeholder index to the index of the
// corresponding key in the resolver's current (unsorted) deduped key list.
func (s *HTTPTransactionsSerializer) v1SortedIndexToCurrent(sortedIdx int) (int, error) {
	dedupedKeys := s.resolver.GetAPIKeys()
	keys := make([]string, len(dedupedKeys))
	copy(keys, dedupedKeys)
	sort.Strings(keys)

	if sortedIdx >= len(keys) {
		return 0, fmt.Errorf("V1 placeholder index %d out of range (have %d keys)", sortedIdx, len(keys))
	}
	targetKey := keys[sortedIdx]

	for i, k := range dedupedKeys {
		if k == targetKey {
			if sortedIdx != i {
				tlmV1TransactionsDeserialized.Inc()
			}
			return i, nil
		}
	}
	return 0, fmt.Errorf("V1 key at sorted index %d (%q) not found in current key list", sortedIdx, targetKey)
}

// fromHeaderProto converts serialized header proto to http.Header, stripping any API key
// placeholders that may be present in old (pre-V3) serialized data.
func (s *HTTPTransactionsSerializer) fromHeaderProto(headersProto map[string]*HeaderValuesProto) http.Header {
	headers := make(http.Header)
	for key, headerValuesProto := range headersProto {
		var headerValues []string
		for _, v := range headerValuesProto.Values {
			headerValues = append(headerValues, string(v))
		}
		headers[key] = headerValues
	}
	return headers
}

func fromTransactionPriorityProto(priority TransactionPriorityProto) (transaction.Priority, error) {
	switch priority {
	case TransactionPriorityProto_NORMAL:
		return transaction.TransactionPriorityNormal, nil
	case TransactionPriorityProto_HIGH:
		return transaction.TransactionPriorityHigh, nil
	default:
		return transaction.TransactionPriorityNormal, fmt.Errorf("Unsupported priority %v", priority)
	}
}

func fromTransactionDestinationProto(destination TransactionDestinationProto) (transaction.Destination, error) {
	switch destination {
	case TransactionDestinationProto_ALL_REGIONS:
		return transaction.AllRegions, nil
	case TransactionDestinationProto_PRIMARY_ONLY:
		return transaction.PrimaryOnly, nil
	case TransactionDestinationProto_SECONDARY_ONLY:
		return transaction.SecondaryOnly, nil
	default:
		return transaction.AllRegions, fmt.Errorf("Unsupported destination %v", destination)
	}
}

func (s *HTTPTransactionsSerializer) toHeaderProto(headers http.Header) map[string]*HeaderValuesProto {
	headersProto := make(map[string]*HeaderValuesProto)
	for key, headerValues := range headers {
		// (Safety) Don't include the api key header, it shouldn't be there, but just in case...
		if key != "DD-Api-Key" {
			// Convert strings to bytes after replacing API keys
			protoValues := make([][]byte, 0, len(headerValues))
			for _, header := range headerValues {
				protoValues = append(protoValues, []byte(header))
			}
			headersProto[key] = &HeaderValuesProto{Values: protoValues}
		}
	}
	return headersProto
}

func toTransactionPriorityProto(priority transaction.Priority) (TransactionPriorityProto, error) {
	switch priority {
	case transaction.TransactionPriorityNormal:
		return TransactionPriorityProto_NORMAL, nil
	case transaction.TransactionPriorityHigh:
		return TransactionPriorityProto_HIGH, nil
	default:
		return TransactionPriorityProto_NORMAL, fmt.Errorf("Unsupported priority %v", priority)
	}
}

func toTransactionDestinationProto(destination transaction.Destination) (TransactionDestinationProto, error) {
	switch destination {
	case transaction.AllRegions:
		return TransactionDestinationProto_ALL_REGIONS, nil
	case transaction.PrimaryOnly:
		return TransactionDestinationProto_PRIMARY_ONLY, nil
	case transaction.SecondaryOnly:
		return TransactionDestinationProto_SECONDARY_ONLY, nil
	default:
		return TransactionDestinationProto_ALL_REGIONS, fmt.Errorf("Unsupported destination %v", destination)
	}
}

// createReplacersV1 creates replacers for transactions created with V1 of the serializer.
// This version sorted the keys prior to serializing. The location in this list is used to
// unscrub the key.
func createReplacerV1(apiKeys []string) *strings.Replacer {
	// Copy to not modify apiKeys order
	keys := make([]string, len(apiKeys))
	copy(keys, apiKeys)
	// Sort to always have the same order
	sort.Strings(keys)
	var placeholderToAPIKey []string
	for i, k := range keys {
		placeholder := fmt.Sprintf(placeHolderFormat, i)
		placeholderToAPIKey = append(placeholderToAPIKey, placeholder, k)
	}

	return strings.NewReplacer(placeholderToAPIKey...)
}

// createReplacers will create the replacers from and to an API key.
// This relies on the position that the key is found in our list. If the position of the keys in the config
// are changed, then the transaction will be restored to a different, likely wrong, API key.
// We take the list of current APIKey->Placeholder replacers and append the new one to it because we need to
// maintain a history of API keys since a transaction being serialized may contain an old API key and we
// still need to make sure we can replace that one.
func createReplacers(apiKeys []utils.APIKeys, currentAPIKeyPlaceholder []*strings.Replacer) ([]*strings.Replacer, *strings.Replacer) {
	var apiKeyPlaceholder []string
	var placeholderToAPIKey []string
	index := 0
	for path := range apiKeys {
		for _, key := range apiKeys[path].Keys {
			placeholder := fmt.Sprintf(placeHolderFormat, index)
			apiKeyPlaceholder = append(apiKeyPlaceholder, key, placeholder)
			placeholderToAPIKey = append(placeholderToAPIKey, placeholder, key)
			index++
		}
	}
	return append(currentAPIKeyPlaceholder, strings.NewReplacer(apiKeyPlaceholder...)), strings.NewReplacer(placeholderToAPIKey...)
}
