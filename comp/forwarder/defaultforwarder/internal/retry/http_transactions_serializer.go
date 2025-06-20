// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package retry

import (
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	"github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder/resolver"
	"github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder/transaction"
	"github.com/DataDog/datadog-agent/pkg/config/utils"
	"github.com/DataDog/datadog-agent/pkg/telemetry"
	"github.com/DataDog/datadog-agent/pkg/util/common"

	proto "github.com/golang/protobuf/proto"
)

const transactionsSerializerVersion = 2

// Use an non US ASCII char as a separator (Should neither appear in an HTTP header value nor in a URL).
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
	if d, _ := s.resolver.Resolve(transaction.Endpoint); transaction.Domain != d {
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
		Endpoint:    &EndpointProto{Route: s.replaceAPIKeys(endpoint.Route), Name: endpoint.Name},
		Headers:     s.toHeaderProto(transaction.Headers),
		Payload:     payload,
		ErrorCount:  int64(transaction.ErrorCount),
		CreatedAt:   transaction.CreatedAt.Unix(),
		Retryable:   transaction.Retryable,
		Priority:    priority,
		PointCount:  pointCount,
		Destination: destination,
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
		var route string
		var proto http.Header
		var destination transaction.Destination
		e := tr.Endpoint

		priority, err := fromTransactionPriorityProto(tr.Priority)
		if err == nil {
			route, err = s.restoreAPIKeys(e.Route, collection.Version)
			if err == nil {
				proto, err = s.fromHeaderProto(tr.Headers, collection.Version)
				if err == nil { // TODO: the reason for this nesting pattern is unclear to me
					destination, err = fromTransactionDestinationProto(tr.Destination)
				}
			}
		}

		if err != nil {
			s.log.Errorf("Error when deserializing a transaction: %v", err)
			errorCount++
			continue
		}

		endpoint := transaction.Endpoint{Route: route, Name: e.Name}
		domain, _ := s.resolver.Resolve(endpoint)
		tr := transaction.HTTPTransaction{
			Domain:         domain,
			Endpoint:       endpoint,
			Headers:        proto,
			Payload:        transaction.NewBytesPayload(tr.Payload, int(tr.GetPointCount())),
			ErrorCount:     int(tr.ErrorCount),
			CreatedAt:      time.Unix(tr.CreatedAt, 0),
			Retryable:      tr.Retryable,
			StorableOnDisk: true,
			Priority:       priority,
			Destination:    destination,
		}
		tr.SetDefaultHandlers()
		httpTransactions = append(httpTransactions, &tr)
	}
	return httpTransactions, errorCount, nil
}

// replaceAPIKeys iterates backward through our list of replacer, returns when
// one of them is able to replace the key.
// Needs to iterate backwards to ensure the latest version of the replacers takes
// priority.
func (s *HTTPTransactionsSerializer) replaceAPIKeys(str string) string {
	s.placeholderMutex.RLock()
	defer s.placeholderMutex.RUnlock()

	for replacer := len(s.apiKeyToPlaceholder) - 1; replacer >= 0; replacer-- {
		replaced := s.apiKeyToPlaceholder[replacer].Replace(str)
		if str != replaced {
			return replaced
		}
	}

	return str

}

func (s *HTTPTransactionsSerializer) restoreAPIKeys(str string, protoVersion int32) (string, error) {
	var newStr string
	if protoVersion == 1 {
		// Handle transactions serialized in a prior version
		newStr = s.placeholderToAPIKeyV1.Replace(str)
		if newStr != str {
			tlmV1TransactionsDeserialized.Inc()
		}
	} else {
		s.placeholderMutex.RLock()
		newStr = s.placeholderToAPIKey.Replace(str)
		s.placeholderMutex.RUnlock()
	}

	if strings.Contains(newStr, placeHolderPrefix) {
		return "", errors.New("cannot restore the transaction as an API Key is missing")
	}
	return newStr, nil
}

func (s *HTTPTransactionsSerializer) fromHeaderProto(headersProto map[string]*HeaderValuesProto, protoVersion int32) (http.Header, error) {
	headers := make(http.Header)
	for key, headerValuesProto := range headersProto {
		var headerValues []string
		for _, v := range headerValuesProto.Values {
			value, err := s.restoreAPIKeys(v, protoVersion)
			if err != nil {
				return nil, err
			}
			headerValues = append(headerValues, value)
		}
		headers[key] = headerValues
	}
	return headers, nil
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
		headerValuesProto := HeaderValuesProto{Values: common.StringSliceTransform(headerValues, s.replaceAPIKeys)}
		headersProto[key] = &headerValuesProto
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
