package forwarder

import (
	"io/ioutil"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestRetryTransactionsFileStorageSerializeDeserialize(t *testing.T) {
	a := assert.New(t)
	path, clean := createTmpFolder(a)
	defer clean()

	tr := NewHTTPTransaction()
	tr.Domain = "domain"
	tr.Endpoint = "endpoint"
	tr.Headers.Set("Key", "value")
	payload := []byte{1, 2, 3}
	tr.Payload = &payload
	tr.ErrorCount = 1
	tr.CreatedAt = time.Now()
	tr.Retryable = true
	tr.Priority = TransactionPriorityHigh

	r := newRetryTransactionsFileStorage(path, 1000)
	err := r.Serialize([]Transaction{tr})
	a.NoError(err)
	transactions, err := r.DeserializeLast()
	a.NoError(err)
	a.Len(transactions, 1)
	transaction := transactions[0].(*HTTPTransaction)
	a.Equal(tr.Domain, transaction.Domain)
	a.Equal(tr.Endpoint, transaction.Endpoint)
	a.Equal(tr.Headers, transaction.Headers)
	a.Equal(tr.Payload, transaction.Payload)
	a.Equal(tr.ErrorCount, transaction.ErrorCount)

	// Ignore monotonic clock
	a.Equal(tr.CreatedAt.Format(time.RFC3339Nano), transaction.CreatedAt.Format(time.RFC3339Nano))

	a.Equal(tr.Retryable, transaction.Retryable)
	a.Equal(tr.Priority, transaction.Priority)
	err = r.Stop()
	a.NoError(err)
}

func TestRetryTransactionsFileStorageSerializeDeserializeMultiple(t *testing.T) {
	a := assert.New(t)
	path, clean := createTmpFolder(a)
	defer clean()

	r := newRetryTransactionsFileStorage(path, 1000)
	err := r.Serialize(createHTTPTransactionTests("endpoint1", "endpoint2"))
	a.NoError(err)
	err = r.Serialize(createHTTPTransactionTests("endpoint3", "endpoint4"))
	a.NoError(err)
	a.Equal(2, r.GetFileCount())

	transactions, err := r.DeserializeLast()
	a.NoError(err)
	a.Equal([]string{"endpoint3", "endpoint4"}, transactionsToEndpoints(transactions))

	transactions, err = r.DeserializeLast()
	a.NoError(err)
	a.Equal([]string{"endpoint1", "endpoint2"}, transactionsToEndpoints(transactions))
	a.Equal(0, r.GetFileCount())
}

func TestRetryTransactionsFileStorageDropOldFile(t *testing.T) {
	a := assert.New(t)
	path, clean := createTmpFolder(a)
	defer clean()

	r := newRetryTransactionsFileStorage(path, 500)

	// drop the 2 oldest files
	i := 0
	for ; r.GetFileCount()+2 > i; i++ {
		err := r.Serialize(createHTTPTransactionTests(strconv.Itoa(i)))
		a.NoError(err)
	}

	a.Greater(i, 2)
	for i--; i >= 2; i-- {
		transactions, err := r.DeserializeLast()
		a.NoError(err)
		a.Equal([]string{strconv.Itoa(i)}, transactionsToEndpoints(transactions))
	}

	transactions, err := r.DeserializeLast()
	a.NoError(err)
	a.Len(transactions, 0)
	a.Equal(0, r.GetFileCount())
}

func TestRetryTransactionsFileStorageStop(t *testing.T) {
	a := assert.New(t)
	path, clean := createTmpFolder(a)
	defer clean()

	r := newRetryTransactionsFileStorage(path, 1000)
	err := r.Serialize(createHTTPTransactionTests("endpoint"))
	a.NoError(err)

	a.Equal(1, r.GetFileCount())
	err = r.Stop()
	a.NoError(err)
	a.Equal(0, r.GetFileCount())
}

func createHTTPTransactionTests(endpoint ...string) []Transaction {
	var transactions []Transaction

	for _, e := range endpoint {
		t := NewHTTPTransaction()
		t.Endpoint = e
		transactions = append(transactions, t)
	}
	return transactions
}

func createTmpFolder(a *assert.Assertions) (string, func()) {
	path, err := ioutil.TempDir("", "tests")
	a.NoError(err)
	return path, func() { _ = os.Remove(path) }
}

func transactionsToEndpoints(transactions []Transaction) []string {
	var endpoints []string
	for _, t := range transactions {
		httpTransaction := t.(*HTTPTransaction)
		endpoints = append(endpoints, httpTransaction.Endpoint)
	}
	return endpoints
}
