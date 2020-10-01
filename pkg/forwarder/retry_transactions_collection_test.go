package forwarder

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

type retryTransactionsStorageMock struct {
	mock.Mock
	payloadsToFlush []Transaction
}

func (m *retryTransactionsStorageMock) Serialize(payloadsToFlush []Transaction) error {
	m.payloadsToFlush = payloadsToFlush
	return m.Called(payloadsToFlush).Error(0)
}

func (m *retryTransactionsStorageMock) DeserializeLast() ([]Transaction, error) {
	call := m.Called()
	return call.Get(0).([]Transaction), call.Error(1)
}

func (m *retryTransactionsStorageMock) Stop() error {
	return m.Called().Error(0)
}

func TestRetryTransactionsAdd(t *testing.T) {
	a := assert.New(t)
	s := retryTransactionsStorageMock{}
	s.On("Serialize", mock.AnythingOfType("[]forwarder.Transaction")).Return(nil).Times(1)
	s.On("DeserializeLast").Return([]Transaction{}, nil).Times(1)
	r := newRetryTransactionsCollection(100, &s)

	// Before adding 15, the current size is 100 and so we flush
	// 10 + 20 + 30 which is greater than 100/2
	for _, payloadSize := range []int{10, 20, 30, 40, 15} {
		tr := newTestTransaction()
		tr.On("GetPayloadSize").Return(payloadSize)
		r.Add(tr)
	}
	a.Len(s.payloadsToFlush, 3)
	a.Equal(10, s.payloadsToFlush[0].GetPayloadSize())
	a.Equal(20, s.payloadsToFlush[1].GetPayloadSize())
	a.Equal(30, s.payloadsToFlush[2].GetPayloadSize())

	transactions, err := r.GetRetryTransactions()
	a.NoError(err)
	a.Len(transactions, 2)
	a.Equal(40, transactions[0].GetPayloadSize())
	a.Equal(15, transactions[1].GetPayloadSize())

	transactions, err = r.GetRetryTransactions()
	a.NoError(err)
	a.Len(transactions, 0)
	s.AssertExpectations(t)
}
