package forwarder

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/util/filesystem"
	"github.com/stretchr/testify/assert"
)

func TestTransactionContainerAdd(t *testing.T) {
	a := assert.New(t)
	s, clean := newTransactionsFileStorageTest(a)
	defer clean()

	container := newTransactionContainer(createDropPrioritySorter(), s, 100, 0.6, transactionContainerTelemetry{})

	// When adding the last element `15`, the buffer becomes full and the first 3
	// transactions are flushed to the disk as 10 + 20 + 30 >= 100 * 0.6
	for _, payloadSize := range []int{10, 20, 30, 40, 15} {
		_, err := container.add(createTransactionWithPayloadSize(payloadSize))
		a.NoError(err)
	}
	a.Equal(40+15, container.getCurrentMemSizeInBytes())
	a.Equal(2, container.getTransactionCount())

	assertPayloadSizeFromExtractTransactions(a, container, []int{40, 15})

	_, err := container.add(createTransactionWithPayloadSize(5))
	a.NoError(err)
	a.Equal(5, container.getCurrentMemSizeInBytes())
	a.Equal(1, container.getTransactionCount())

	assertPayloadSizeFromExtractTransactions(a, container, []int{5})
	assertPayloadSizeFromExtractTransactions(a, container, []int{10, 20, 30})
	assertPayloadSizeFromExtractTransactions(a, container, nil)
}

func TestTransactionContainerSeveralFlushToDisk(t *testing.T) {
	a := assert.New(t)
	s, clean := newTransactionsFileStorageTest(a)
	defer clean()

	container := newTransactionContainer(createDropPrioritySorter(), s, 50, 0.1, transactionContainerTelemetry{})

	// Flush to disk when adding `40`
	for _, payloadSize := range []int{9, 10, 11, 40} {
		container.add(createTransactionWithPayloadSize(payloadSize))
	}
	a.Equal(40, container.getCurrentMemSizeInBytes())
	a.Equal(3, s.getFilesCount())

	assertPayloadSizeFromExtractTransactions(a, container, []int{40})
	assertPayloadSizeFromExtractTransactions(a, container, []int{11})
	assertPayloadSizeFromExtractTransactions(a, container, []int{10})
	assertPayloadSizeFromExtractTransactions(a, container, []int{9})
	a.Equal(0, s.getFilesCount())
	a.Equal(int64(0), s.getCurrentSizeInBytes())
}

func TestTransactionContainerNoTransactionStorage(t *testing.T) {
	a := assert.New(t)
	container := newTransactionContainer(createDropPrioritySorter(), nil, 50, 0.1, transactionContainerTelemetry{})

	for _, payloadSize := range []int{9, 10, 11} {
		dropCount, err := container.add(createTransactionWithPayloadSize(payloadSize))
		a.Equal(0, dropCount)
		a.NoError(err)
	}

	// Drop when adding `30`
	dropCount, err := container.add(createTransactionWithPayloadSize(30))
	a.Equal(2, dropCount)
	a.NoError(err)

	a.Equal(11+30, container.getCurrentMemSizeInBytes())

	assertPayloadSizeFromExtractTransactions(a, container, []int{11, 30})
}

func TestTransactionContainerZeroMaxMemSizeInBytes(t *testing.T) {
	a := assert.New(t)
	s, clean := newTransactionsFileStorageTest(a)
	defer clean()

	maxMemSizeInBytes := 0
	container := newTransactionContainer(createDropPrioritySorter(), s, maxMemSizeInBytes, 0.1, transactionContainerTelemetry{})

	inMemTrDropped, err := container.add(createTransactionWithPayloadSize(10))
	a.NoError(err)
	a.Equal(0, inMemTrDropped)

	// `extractTransactionsForDisk` does not behave the same when there is a existing transaction.
	inMemTrDropped, err = container.add(createTransactionWithPayloadSize(10))
	a.NoError(err)
	a.Equal(1, inMemTrDropped)
}

func createTransactionWithPayloadSize(payloadSize int) *HTTPTransaction {
	tr := NewHTTPTransaction()
	payload := make([]byte, payloadSize)
	tr.Payload = &payload
	return tr
}

func assertPayloadSizeFromExtractTransactions(
	a *assert.Assertions,
	container *transactionContainer,
	expectedPayloadSize []int) {

	transactions, err := container.extractTransactions()
	a.NoError(err)
	a.Equal(0, container.getCurrentMemSizeInBytes())

	var payloadSizes []int
	for _, t := range transactions {
		payloadSizes = append(payloadSizes, t.GetPayloadSize())
	}
	a.EqualValues(expectedPayloadSize, payloadSizes)
}

func createDropPrioritySorter() sortByCreatedTimeAndPriority {
	return sortByCreatedTimeAndPriority{highPriorityFirst: false}
}

func newTransactionsFileStorageTest(a *assert.Assertions) (*transactionsFileStorage, func()) {
	path, clean := createTmpFolder(a)
	disk := diskUsageRetrieverMock{
		diskUsage: &filesystem.DiskUsage{
			Available: 10000,
			Total:     10000,
		}}
	maxStorage := newForwarderMaxStorage("", disk, 1000, 1)
	s, err := newTransactionsFileStorage(NewTransactionsSerializer("", nil), path, maxStorage, transactionsFileStorageTelemetry{})
	a.NoError(err)
	return s, clean
}
