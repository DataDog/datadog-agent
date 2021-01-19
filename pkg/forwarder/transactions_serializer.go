// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package forwarder

import (
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/common"
	"github.com/DataDog/datadog-agent/pkg/util/log"

	proto "github.com/golang/protobuf/proto"
)

const transactionsSerializerVersion = 1

// Use an non US ASCII char as a separator (Should neither appear in an HTTP header value nor in a URL).
const squareChar = "\xfe"
const placeHolderPrefix = squareChar + "API_KEY" + squareChar
const placeHolderFormat = placeHolderPrefix + "%v" + squareChar

// TransactionsSerializer serializes Transaction instances.
// To support a new Transaction implementation, add a new
// method `func (s *TransactionsSerializer) Add(transaction NEW_TYPE) error {`
type TransactionsSerializer struct {
	collection          HttpTransactionProtoCollection
	apiKeyToPlaceholder *strings.Replacer
	placeholderToAPIKey *strings.Replacer
	domain              string
}

// NewTransactionsSerializer creates a new instance of TransactionsSerializer
func NewTransactionsSerializer(domain string, apiKeys []string) *TransactionsSerializer {
	apiKeyToPlaceholder, placeholderToAPIKey := createReplacers(apiKeys)

	return &TransactionsSerializer{
		collection: HttpTransactionProtoCollection{
			Version: transactionsSerializerVersion,
		},
		apiKeyToPlaceholder: apiKeyToPlaceholder,
		placeholderToAPIKey: placeholderToAPIKey,
		domain:              domain,
	}
}

// Add adds a transaction to the serializer.
// This function uses references on HTTPTransaction.Payload and HTTPTransaction.Headers
// and so the transaction must not be updated until a call to `GetBytesAndReset`.
func (s *TransactionsSerializer) Add(transaction *HTTPTransaction) error {
	if transaction.Domain != s.domain {
		// This error is not supposed to happen (Sanity check).
		return fmt.Errorf("The domain of the transaction %v does not match the domain %v", transaction.Domain, s.domain)
	}

	priority, err := toTransactionPriorityProto(transaction.priority)
	if err != nil {
		return err
	}

	var payload []byte
	if transaction.Payload != nil {
		payload = *transaction.Payload
	}

	endpoint := transaction.Endpoint
	transactionProto := HttpTransactionProto{
		Domain:     "", // Domain is always the same
		Endpoint:   &EndpointProto{Route: s.replaceAPIKeys(endpoint.route), Name: endpoint.name},
		Headers:    s.toHeaderProto(transaction.Headers),
		Payload:    payload,
		ErrorCount: int64(transaction.ErrorCount),
		CreatedAt:  transaction.createdAt.Unix(),
		Retryable:  transaction.retryable,
		Priority:   priority,
	}
	s.collection.Values = append(s.collection.Values, &transactionProto)
	return nil
}

// GetBytesAndReset returns as bytes the serialized transactions and reset
// the transaction collection.
func (s *TransactionsSerializer) GetBytesAndReset() ([]byte, error) {
	out, err := proto.Marshal(&s.collection)
	s.collection.Values = nil
	return out, err
}

// Deserialize deserializes from bytes.
func (s *TransactionsSerializer) Deserialize(bytes []byte) ([]Transaction, int, error) {
	collection := HttpTransactionProtoCollection{}

	if err := proto.Unmarshal(bytes, &collection); err != nil {
		return nil, 0, err
	}

	var httpTransactions []Transaction
	errorCount := 0
	for _, transaction := range collection.Values {
		var route string
		var proto http.Header
		e := transaction.Endpoint

		priority, err := fromTransactionPriorityProto(transaction.Priority)
		if err == nil {
			route, err = s.restoreAPIKeys(e.Route)
			if err == nil {
				proto, err = s.fromHeaderProto(transaction.Headers)
			}
		}

		if err != nil {
			log.Errorf("Error when deserializing a transaction: %v", err)
			errorCount++
			continue
		}
		tr := HTTPTransaction{
			Domain:         s.domain,
			Endpoint:       endpoint{route: route, name: e.Name},
			Headers:        proto,
			Payload:        &transaction.Payload,
			ErrorCount:     int(transaction.ErrorCount),
			createdAt:      time.Unix(transaction.CreatedAt, 0),
			retryable:      transaction.Retryable,
			storableOnDisk: true,
			priority:       priority,
		}
		tr.setDefaultHandlers()
		httpTransactions = append(httpTransactions, &tr)
	}
	return httpTransactions, errorCount, nil
}

func (s *TransactionsSerializer) replaceAPIKeys(str string) string {
	return s.apiKeyToPlaceholder.Replace(str)
}

func (s *TransactionsSerializer) restoreAPIKeys(str string) (string, error) {
	newStr := s.placeholderToAPIKey.Replace(str)

	if strings.Contains(newStr, placeHolderPrefix) {
		return "", errors.New("cannot restore the transaction as an API Key is missing")
	}
	return newStr, nil
}

func (s *TransactionsSerializer) fromHeaderProto(headersProto map[string]*HeaderValuesProto) (http.Header, error) {
	headers := make(http.Header)
	for key, headerValuesProto := range headersProto {
		var headerValues []string
		for _, v := range headerValuesProto.Values {
			value, err := s.restoreAPIKeys(v)
			if err != nil {
				return nil, err
			}
			headerValues = append(headerValues, value)
		}
		headers[key] = headerValues
	}
	return headers, nil
}

func fromTransactionPriorityProto(priority TransactionPriorityProto) (TransactionPriority, error) {
	switch priority {
	case TransactionPriorityProto_NORMAL:
		return TransactionPriorityNormal, nil
	case TransactionPriorityProto_HIGH:
		return TransactionPriorityHigh, nil
	default:
		return TransactionPriorityNormal, fmt.Errorf("Unsupported priority %v", priority)
	}
}

func (s *TransactionsSerializer) toHeaderProto(headers http.Header) map[string]*HeaderValuesProto {
	headersProto := make(map[string]*HeaderValuesProto)
	for key, headerValues := range headers {
		headerValuesProto := HeaderValuesProto{Values: common.StringSliceTransform(headerValues, s.replaceAPIKeys)}
		headersProto[key] = &headerValuesProto
	}
	return headersProto
}

func toTransactionPriorityProto(priority TransactionPriority) (TransactionPriorityProto, error) {
	switch priority {
	case TransactionPriorityNormal:
		return TransactionPriorityProto_NORMAL, nil
	case TransactionPriorityHigh:
		return TransactionPriorityProto_HIGH, nil
	default:
		return TransactionPriorityProto_NORMAL, fmt.Errorf("Unsupported priority %v", priority)
	}
}

func createReplacers(apiKeys []string) (*strings.Replacer, *strings.Replacer) {
	// Copy to not modify apiKeys order
	keys := make([]string, len(apiKeys))
	copy(keys, apiKeys)

	// Sort to always have the same order
	sort.Strings(keys)
	var apiKeyPlaceholder []string
	var placeholderToAPIKey []string
	for i, k := range keys {
		placeholder := fmt.Sprintf(placeHolderFormat, i)
		apiKeyPlaceholder = append(apiKeyPlaceholder, k, placeholder)
		placeholderToAPIKey = append(placeholderToAPIKey, placeholder, k)
	}
	return strings.NewReplacer(apiKeyPlaceholder...), strings.NewReplacer(placeholderToAPIKey...)
}
