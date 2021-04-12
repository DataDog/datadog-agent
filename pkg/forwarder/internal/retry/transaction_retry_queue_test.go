package retry

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/forwarder/transaction"
	"github.com/DataDog/datadog-agent/pkg/util/filesystem"
	"github.com/stretchr/testify/assert"
)

func TestTransactionRetryQueueAdd(t *testing.T) {
	a := assert.New(t)
	q, clean := newOnDiskRetryQueueTest(a)
	defer clean()

	container := NewTransactionRetryQueue(createDropPrioritySorter(), q, 100, 0.6, TransactionRetryQueueTelemetry{})

	// When adding the last element `15`, the buffer becomes full and the first 3
	// transactions are flushed to the disk as 10 + 20 + 30 >= 100 * 0.6
	for _, payloadSize := range []int{10, 20, 30, 40, 15} {
		_, err := container.Add(createTransactionWithPayloadSize(payloadSize))
		a.NoError(err)
	}
	a.Equal(40+15, container.getCurrentMemSizeInBytes())
	a.Equal(2, container.GetTransactionCount())

	assertPayloadSizeFromExtractTransactions(a, container, []int{40, 15})

	_, err := container.Add(createTransactionWithPayloadSize(5))
	a.NoError(err)
	a.Equal(5, container.getCurrentMemSizeInBytes())
	a.Equal(1, container.GetTransactionCount())

	assertPayloadSizeFromExtractTransactions(a, container, []int{5})
	assertPayloadSizeFromExtractTransactions(a, container, []int{10, 20, 30})
	assertPayloadSizeFromExtractTransactions(a, container, nil)
}

func TestTransactionRetryQueueSeveralFlushToDisk(t *testing.T) {
	a := assert.New(t)
	q, clean := newOnDiskRetryQueueTest(a)
	defer clean()

	container := NewTransactionRetryQueue(createDropPrioritySorter(), q, 50, 0.1, TransactionRetryQueueTelemetry{})

	// Flush to disk when adding `40`
	for _, payloadSize := range []int{9, 10, 11, 40} {
		container.Add(createTransactionWithPayloadSize(payloadSize))
	}
	a.Equal(40, container.getCurrentMemSizeInBytes())
	a.Equal(3, q.getFilesCount())

	assertPayloadSizeFromExtractTransactions(a, container, []int{40})
	assertPayloadSizeFromExtractTransactions(a, container, []int{11})
	assertPayloadSizeFromExtractTransactions(a, container, []int{10})
	assertPayloadSizeFromExtractTransactions(a, container, []int{9})
	a.Equal(0, q.getFilesCount())
	a.Equal(int64(0), q.getCurrentSizeInBytes())
}

func TestTransactionRetryQueueNoTransactionStorage(t *testing.T) {
	a := assert.New(t)
	container := NewTransactionRetryQueue(createDropPrioritySorter(), nil, 50, 0.1, TransactionRetryQueueTelemetry{})

	for _, payloadSize := range []int{9, 10, 11} {
		dropCount, err := container.Add(createTransactionWithPayloadSize(payloadSize))
		a.Equal(0, dropCount)
		a.NoError(err)
	}

	// Drop when adding `30`
	dropCount, err := container.Add(createTransactionWithPayloadSize(30))
	a.Equal(2, dropCount)
	a.NoError(err)

	a.Equal(11+30, container.getCurrentMemSizeInBytes())

	assertPayloadSizeFromExtractTransactions(a, container, []int{11, 30})
}

func TestTransactionRetryQueueZeroMaxMemSizeInBytes(t *testing.T) {
	a := assert.New(t)
	q, clean := newOnDiskRetryQueueTest(a)
	defer clean()

	maxMemSizeInBytes := 0
	container := NewTransactionRetryQueue(createDropPrioritySorter(), q, maxMemSizeInBytes, 0.1, TransactionRetryQueueTelemetry{})

	inMemTrDropped, err := container.Add(createTransactionWithPayloadSize(10))
	a.NoError(err)
	a.Equal(0, inMemTrDropped)

	// `extractTransactionsForDisk` does not behave the same when there is a existing transaction.
	inMemTrDropped, err = container.Add(createTransactionWithPayloadSize(10))
	a.NoError(err)
	a.Equal(1, inMemTrDropped)
}

func createTransactionWithPayloadSize(payloadSize int) *transaction.HTTPTransaction {
	tr := transaction.NewHTTPTransaction()
	payload := make([]byte, payloadSize)
	tr.Payload = &payload
	return tr
}

func assertPayloadSizeFromExtractTransactions(
	a *assert.Assertions,
	container *TransactionRetryQueue,
	expectedPayloadSize []int) {

	transactions, err := container.ExtractTransactions()
	a.NoError(err)
	a.Equal(0, container.getCurrentMemSizeInBytes())

	var payloadSizes []int
	for _, t := range transactions {
		payloadSizes = append(payloadSizes, t.GetPayloadSize())
	}
	a.EqualValues(expectedPayloadSize, payloadSizes)
}

func createDropPrioritySorter() transaction.SortByCreatedTimeAndPriority {
	return transaction.SortByCreatedTimeAndPriority{HighPriorityFirst: false}
}

func newOnDiskRetryQueueTest(a *assert.Assertions) (*onDiskRetryQueue, func()) {
	path, clean := createTmpFolder(a)
	disk := diskUsageRetrieverMock{
		diskUsage: &filesystem.DiskUsage{
			Available: 10000,
			Total:     10000,
		}}
	diskUsageLimit := newDiskUsageLimit("", disk, 1000, 1)
	q, err := newOnDiskRetryQueue(NewHTTPTransactionsSerializer("", nil), path, diskUsageLimit, onDiskRetryQueueTelemetry{})
	a.NoError(err)
	return q, clean
}
