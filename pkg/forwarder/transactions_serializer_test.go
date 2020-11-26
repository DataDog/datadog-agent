package forwarder

import (
	"net/http"
	"reflect"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestSerializeDeserialize(t *testing.T) {
	a := assert.New(t)
	tr := createHTTPTransactionTests()
	serializer := NewTransactionsSerializer()

	a.NoError(serializer.Add(tr))
	bytes, err := serializer.GetBytesAndReset()
	a.NoError(err)

	transactions, err := serializer.Deserialize(bytes)
	a.NoError(err)
	a.Len(transactions, 1)
	transactionDeserialized := transactions[0].(*HTTPTransaction)

	assertTransactionEqual(a, tr, transactionDeserialized)

	bytes, err = serializer.GetBytesAndReset()
	a.NoError(err)
	transactions, err = serializer.Deserialize(bytes)
	a.NoError(err)
	a.Len(transactions, 0)
}

func TestPartialDeserialize(t *testing.T) {
	a := assert.New(t)
	transaction := createHTTPTransactionTests()
	serializer := NewTransactionsSerializer()

	a.NoError(serializer.Add(transaction))
	a.NoError(serializer.Add(transaction))
	bytes, err := serializer.GetBytesAndReset()
	a.NoError(err)

	for end := len(bytes); end >= 0; end-- {
		trs, err := serializer.Deserialize(bytes[:end])

		// If there is no error, transactions should be valid.
		if err == nil {
			for _, tr := range trs {
				assertTransactionEqual(a, tr.(*HTTPTransaction), transaction)
			}
		}
	}
}

func TestHTTPTransactionFieldsCount(t *testing.T) {
	transaction := HTTPTransaction{}
	transactionType := reflect.TypeOf(transaction)
	assert.Equalf(t, 10, transactionType.NumField(),
		"A field was added or remove from HTTPTransaction. "+
			"You probably need to update the implementation of "+
			"TransactionsSerializer and then adjust this unit test.")
}

func createHTTPTransactionTests() *HTTPTransaction {
	payload := []byte{1, 2, 3}
	tr := NewHTTPTransaction()
	tr.Domain = "domain"
	tr.Endpoint = endpoint{route: "route", name: "name"}
	tr.Headers = http.Header{"Key": []string{"value1", "value2"}}
	tr.Payload = &payload
	tr.ErrorCount = 1
	tr.createdAt = time.Now()
	tr.retryable = true
	tr.priority = TransactionPriorityHigh
	return tr
}

func assertTransactionEqual(a *assert.Assertions, tr1 *HTTPTransaction, tr2 *HTTPTransaction) {
	a.Equal(tr1.Domain, tr2.Domain)
	a.Equal(tr1.Endpoint, tr2.Endpoint)
	a.EqualValues(tr1.Headers, tr2.Headers)
	a.Equal(tr1.retryable, tr2.retryable)
	a.Equal(tr1.priority, tr2.priority)
	a.Equal(tr1.ErrorCount, tr2.ErrorCount)

	a.NotNil(tr1.Payload)
	a.NotNil(tr2.Payload)
	a.Equal(*tr1.Payload, *tr2.Payload)

	// Ignore monotonic clock
	a.Equal(tr1.createdAt.Format(time.RFC3339), tr2.createdAt.Format(time.RFC3339))
}
