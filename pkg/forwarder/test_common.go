package forwarder

import (
	"net/http"
	"time"

	"github.com/stretchr/testify/mock"
)

type testTransaction struct {
	mock.Mock
	processed chan bool
}

func newTestTransaction() *testTransaction {
	t := new(testTransaction)
	t.processed = make(chan bool, 1)
	return t
}

func (t *testTransaction) GetNextFlush() time.Time {
	return t.Called().Get(0).(time.Time)
}

func (t *testTransaction) GetCreatedAt() time.Time {
	return t.Called().Get(0).(time.Time)
}

func (t *testTransaction) Process(client *http.Client) error {
	defer func() { t.processed <- true }()
	return t.Called(client).Error(0)
}

func (t *testTransaction) Reschedule() {
	t.Called()
}
