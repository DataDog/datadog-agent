package forwarder

import (
	"io/ioutil"
	"os"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
)

const domainName = "domain"

func TestTransactionsFileStorage(t *testing.T) {
	a := assert.New(t)
	path, clean := createTmpFolder(a)
	defer clean()

	s := newTestTransactionsFileStorage(a, path, 1000)
	err := s.Serialize(createHTTPTransactionCollectionTests("endpoint1", "endpoint2"))
	a.NoError(err)
	err = s.Serialize(createHTTPTransactionCollectionTests("endpoint3", "endpoint4"))
	a.NoError(err)
	a.Equal(2, s.getFilesCount())

	transactions, err := s.Deserialize()
	a.NoError(err)
	a.Equal([]string{"endpoint3", "endpoint4"}, getEndpointsFromTransactions(transactions))
	a.Greater(s.getCurrentSizeInBytes(), int64(0))

	transactions, err = s.Deserialize()
	a.NoError(err)
	a.Equal([]string{"endpoint1", "endpoint2"}, getEndpointsFromTransactions(transactions))
	a.Equal(0, s.getFilesCount())
	a.Equal(int64(0), s.getCurrentSizeInBytes())
}

func TestTransactionsFileStorageMaxSize(t *testing.T) {
	a := assert.New(t)
	path, clean := createTmpFolder(a)
	defer clean()

	maxSizeInBytes := int64(100)
	s := newTestTransactionsFileStorage(a, path, maxSizeInBytes)

	i := 0
	err := s.Serialize(createHTTPTransactionCollectionTests(strconv.Itoa(i)))
	a.NoError(err)
	maxNumberOfFiles := int(maxSizeInBytes / s.getCurrentSizeInBytes())
	a.Greaterf(maxNumberOfFiles, 2, "Not enough files for this test, increase maxSizeInBytes")

	fileToDrop := 2
	for i++; i < maxNumberOfFiles+fileToDrop; i++ {
		err := s.Serialize(createHTTPTransactionCollectionTests(strconv.Itoa(i)))
		a.NoError(err)
	}
	a.LessOrEqual(s.getCurrentSizeInBytes(), maxSizeInBytes)
	a.Equal(maxNumberOfFiles, s.getFilesCount())

	for i--; i >= fileToDrop; i-- {
		transactions, err := s.Deserialize()
		a.NoError(err)
		a.Equal([]string{strconv.Itoa(i)}, getEndpointsFromTransactions(transactions))
	}

	a.Equal(0, s.getFilesCount())
}

func TestTransactionsFileStorageReloadExistingRetryFiles(t *testing.T) {
	a := assert.New(t)
	path, clean := createTmpFolder(a)
	defer clean()

	storage := newTestTransactionsFileStorage(a, path, 1000)
	err := storage.Serialize(createHTTPTransactionCollectionTests("endpoint1", "endpoint2"))
	a.NoError(err)

	newStorage := newTestTransactionsFileStorage(a, path, 1000)
	a.Equal(storage.getCurrentSizeInBytes(), newStorage.getCurrentSizeInBytes())
	a.Equal(storage.getFilesCount(), newStorage.getFilesCount())
	transactions, err := newStorage.Deserialize()
	a.NoError(err)
	a.Equal([]string{"endpoint1", "endpoint2"}, getEndpointsFromTransactions(transactions))
}

func createHTTPTransactionCollectionTests(endpoints ...string) []Transaction {
	var transactions []Transaction

	for _, d := range endpoints {
		t := NewHTTPTransaction()
		t.Domain = domainName
		t.Endpoint.name = d
		transactions = append(transactions, t)
	}
	return transactions
}

func createTmpFolder(a *assert.Assertions) (string, func()) {
	path, err := ioutil.TempDir("", "tests")
	a.NoError(err)
	return path, func() { _ = os.Remove(path) }
}

func getEndpointsFromTransactions(transactions []Transaction) []string {
	var endpoints []string
	for _, t := range transactions {
		httpTransaction := t.(*HTTPTransaction)
		endpoints = append(endpoints, httpTransaction.Endpoint.name)
	}
	return endpoints
}

func newTestTransactionsFileStorage(a *assert.Assertions, path string, maxSizeInBytes int64) *transactionsFileStorage {
	telemetry := transactionsFileStorageTelemetry{}
	storage, err := newTransactionsFileStorage(NewTransactionsSerializer(domainName, nil), path, maxSizeInBytes, telemetry)
	a.NoError(err)
	return storage
}
