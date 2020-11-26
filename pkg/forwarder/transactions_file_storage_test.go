package forwarder

import (
	"io/ioutil"
	"os"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestTransactionsFileStorage(t *testing.T) {
	a := assert.New(t)
	path, clean := createTmpFolder(a)
	defer clean()

	s := newTransactionsFileStorage(NewTransactionsSerializer(), path, 1000)
	err := s.Serialize(createHTTPTransactionCollectionTests("domain1", "domain2"))
	a.NoError(err)
	err = s.Serialize(createHTTPTransactionCollectionTests("domain3", "domain4"))
	a.NoError(err)
	a.Equal(2, s.GetFilesCount())

	transactions, err := s.Deserialize()
	a.NoError(err)
	a.Equal([]string{"domain3", "domain4"}, getDomainsFromTransactions(transactions))
	a.Greater(s.GetCurrentSizeInBytes(), int64(0))

	transactions, err = s.Deserialize()
	a.NoError(err)
	a.Equal([]string{"domain1", "domain2"}, getDomainsFromTransactions(transactions))
	a.Equal(0, s.GetFilesCount())
	a.Equal(int64(0), s.GetCurrentSizeInBytes())
}

func TestTransactionsFileStorageMaxSize(t *testing.T) {
	a := assert.New(t)
	path, clean := createTmpFolder(a)
	defer clean()

	maxSizeInBytes := int64(100)
	s := newTransactionsFileStorage(NewTransactionsSerializer(), path, maxSizeInBytes)

	i := 0
	err := s.Serialize(createHTTPTransactionCollectionTests(strconv.Itoa(i)))
	a.NoError(err)
	maxNumberOfFiles := int(maxSizeInBytes / s.GetCurrentSizeInBytes())
	a.Greaterf(maxNumberOfFiles, 2, "Not enough files for this test, increase maxSizeInBytes")

	fileToDrop := 2
	for i++; i < maxNumberOfFiles+fileToDrop; i++ {
		err := s.Serialize(createHTTPTransactionCollectionTests(strconv.Itoa(i)))
		a.NoError(err)
	}
	a.LessOrEqual(s.GetCurrentSizeInBytes(), maxSizeInBytes)
	a.Equal(maxNumberOfFiles, s.GetFilesCount())

	for i--; i >= fileToDrop; i-- {
		transactions, err := s.Deserialize()
		a.NoError(err)
		a.Equal([]string{strconv.Itoa(i)}, getDomainsFromTransactions(transactions))
	}

	a.Equal(0, s.GetFilesCount())
}

func createHTTPTransactionCollectionTests(domain ...string) []Transaction {
	var transactions []Transaction

	for _, d := range domain {
		t := NewHTTPTransaction()
		t.Domain = d
		transactions = append(transactions, t)
	}
	return transactions
}

func createTmpFolder(a *assert.Assertions) (string, func()) {
	path, err := ioutil.TempDir("", "tests")
	a.NoError(err)
	return path, func() { _ = os.Remove(path) }
}

func getDomainsFromTransactions(transactions []Transaction) []string {
	var domain []string
	for _, t := range transactions {
		httpTransaction := t.(*HTTPTransaction)
		domain = append(domain, httpTransaction.Domain)
	}
	return domain
}
