// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package forwarder

import (
	"fmt"
	"net/http"
	"time"

	proto "github.com/golang/protobuf/proto"
)

const transactionsSerializerVersion = 1

// TransactionsSerializer serializes Transaction instances.
// To support a new Transaction implementation, add a new
// method `func (s *TransactionsSerializer) Add(transaction NEW_TYPE) error {`
type TransactionsSerializer struct {
	collection HttpTransactionProtoCollection
}

// NewTransactionsSerializer creates a new instance of TransactionsSerializer
func NewTransactionsSerializer() *TransactionsSerializer {
	return &TransactionsSerializer{
		collection: HttpTransactionProtoCollection{
			Version: transactionsSerializerVersion,
		},
	}
}

// Add adds a transaction to the serializer.
// This function uses references on HTTPTransaction.Payload and HTTPTransaction.Headers
// and so the transaction must not be updated until a call to `GetBytesAndReset`.
func (s *TransactionsSerializer) Add(transaction *HTTPTransaction) error {
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
		Domain:     transaction.Domain,
		Endpoint:   &EndpointProto{Route: endpoint.route, Name: endpoint.name},
		Headers:    toHeaderProto(transaction.Headers),
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
func (s *TransactionsSerializer) Deserialize(bytes []byte) ([]Transaction, error) {
	collection := HttpTransactionProtoCollection{}

	if err := proto.Unmarshal(bytes, &collection); err != nil {
		return nil, err
	}

	var httpTransactions []Transaction
	for _, transaction := range collection.Values {
		priority, err := fromTransactionPriorityProto(transaction.Priority)
		if err != nil {
			return nil, err
		}
		e := transaction.Endpoint
		tr := HTTPTransaction{
			Domain:     transaction.Domain,
			Endpoint:   endpoint{route: e.Route, name: e.Name},
			Headers:    fromHeaderProto(transaction.Headers),
			Payload:    &transaction.Payload,
			ErrorCount: int(transaction.ErrorCount),
			createdAt:  time.Unix(transaction.CreatedAt, 0),
			retryable:  transaction.Retryable,
			priority:   priority,
		}
		tr.setDefaultHandlers()
		httpTransactions = append(httpTransactions, &tr)
	}
	return httpTransactions, nil
}

func fromHeaderProto(headersProto map[string]*HeaderValuesProto) http.Header {
	headers := make(http.Header)
	for key, headerValuesProto := range headersProto {
		headers[key] = headerValuesProto.Values
	}
	return headers
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

func toHeaderProto(headers http.Header) map[string]*HeaderValuesProto {
	headersProto := make(map[string]*HeaderValuesProto)
	for key, headerValues := range headers {
		headerValuesProto := HeaderValuesProto{Values: headerValues}
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
