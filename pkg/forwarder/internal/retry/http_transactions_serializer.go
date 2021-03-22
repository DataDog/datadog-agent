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
	"time"

	"github.com/DataDog/datadog-agent/pkg/forwarder/transaction"
	"github.com/DataDog/datadog-agent/pkg/util/common"
	"github.com/DataDog/datadog-agent/pkg/util/log"

	proto "github.com/golang/protobuf/proto"
)

const transactionsSerializerVersion = 1

// Use an non US ASCII char as a separator (Should neither appear in an HTTP header value nor in a URL).
const squareChar = "\xfe"
const placeHolderPrefix = squareChar + "API_KEY" + squareChar
const placeHolderFormat = placeHolderPrefix + "%v" + squareChar

// HTTPTransactionsSerializer serializes Transaction instances.
// To support a new Transaction implementation, add a new
// method `func (s *HTTPTransactionsSerializer) Add(transaction NEW_TYPE) error {`
type HTTPTransactionsSerializer struct {
	collection          HttpTransactionProtoCollection
	apiKeyToPlaceholder *strings.Replacer
	placeholderToAPIKey *strings.Replacer
	domain              string
}

// NewHTTPTransactionsSerializer creates a new instance of HTTPTransactionsSerializer
func NewHTTPTransactionsSerializer(domain string, apiKeys []string) *HTTPTransactionsSerializer {
	apiKeyToPlaceholder, placeholderToAPIKey := createReplacers(apiKeys)

	return &HTTPTransactionsSerializer{
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
func (s *HTTPTransactionsSerializer) Add(transaction *transaction.HTTPTransaction) error {
	if transaction.Domain != s.domain {
		// This error is not supposed to happen (Sanity check).
		return fmt.Errorf("The domain of the transaction %v does not match the domain %v", transaction.Domain, s.domain)
	}

	priority, err := toTransactionPriorityProto(transaction.Priority)
	if err != nil {
		return err
	}

	var payload []byte
	if transaction.Payload != nil {
		payload = *transaction.Payload
	}

	endpoint := transaction.Endpoint
	transactionProto := HttpTransactionProto{
		// The domain is not stored on the disk for security reasons.
		// If a user can update the domain for some serialized transactions, they can replace the domain
		// by a local address like http://127.0.0.1:1234. The Agent would send the HTTP transactions to the url
		// http://127.0.0.1:1234/intake/?api_key=API_KEY which contains the API_KEY.
		Domain:     "",
		Endpoint:   &EndpointProto{Route: s.replaceAPIKeys(endpoint.Route), Name: endpoint.Name},
		Headers:    s.toHeaderProto(transaction.Headers),
		Payload:    payload,
		ErrorCount: int64(transaction.ErrorCount),
		CreatedAt:  transaction.CreatedAt.Unix(),
		Retryable:  transaction.Retryable,
		Priority:   priority,
	}
	s.collection.Values = append(s.collection.Values, &transactionProto)
	return nil
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

	var httpTransactions []transaction.Transaction
	errorCount := 0
	for _, tr := range collection.Values {
		var route string
		var proto http.Header
		e := tr.Endpoint

		priority, err := fromTransactionPriorityProto(tr.Priority)
		if err == nil {
			route, err = s.restoreAPIKeys(e.Route)
			if err == nil {
				proto, err = s.fromHeaderProto(tr.Headers)
			}
		}

		if err != nil {
			log.Errorf("Error when deserializing a transaction: %v", err)
			errorCount++
			continue
		}
		tr := transaction.HTTPTransaction{
			Domain:         s.domain,
			Endpoint:       transaction.Endpoint{Route: route, Name: e.Name},
			Headers:        proto,
			Payload:        &tr.Payload,
			ErrorCount:     int(tr.ErrorCount),
			CreatedAt:      time.Unix(tr.CreatedAt, 0),
			Retryable:      tr.Retryable,
			StorableOnDisk: true,
			Priority:       priority,
		}
		tr.SetDefaultHandlers()
		httpTransactions = append(httpTransactions, &tr)
	}
	return httpTransactions, errorCount, nil
}

func (s *HTTPTransactionsSerializer) replaceAPIKeys(str string) string {
	return s.apiKeyToPlaceholder.Replace(str)
}

func (s *HTTPTransactionsSerializer) restoreAPIKeys(str string) (string, error) {
	newStr := s.placeholderToAPIKey.Replace(str)

	if strings.Contains(newStr, placeHolderPrefix) {
		return "", errors.New("cannot restore the transaction as an API Key is missing")
	}
	return newStr, nil
}

func (s *HTTPTransactionsSerializer) fromHeaderProto(headersProto map[string]*HeaderValuesProto) (http.Header, error) {
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
