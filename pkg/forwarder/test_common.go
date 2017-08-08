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

// MockedForwarder a mocked forwarder to be use in other module to test their dependencies with the forwarder
type MockedForwarder struct {
	mock.Mock
}

// Start updates the internal mock struct
func (tf *MockedForwarder) Start() error {
	return tf.Called().Error(0)
}

// Stop updates the internal mock struct
func (tf *MockedForwarder) Stop() {
	tf.Called()
}

// SubmitV1Series updates the internal mock struct
func (tf *MockedForwarder) SubmitV1Series(payload Payloads, extraHeaders map[string]string) error {
	return tf.Called(payload, extraHeaders).Error(0)
}

// SubmitV1Intake updates the internal mock struct
func (tf *MockedForwarder) SubmitV1Intake(payload Payloads, extraHeaders map[string]string) error {
	return tf.Called(payload, extraHeaders).Error(0)
}

// SubmitV1CheckRuns updates the internal mock struct
func (tf *MockedForwarder) SubmitV1CheckRuns(payload Payloads, extraHeaders map[string]string) error {
	return tf.Called(payload, extraHeaders).Error(0)
}

// SubmitV1SketchSeries updates the internal mock struct
func (tf *MockedForwarder) SubmitV1SketchSeries(payload Payloads, extraHeaders map[string]string) error {
	return tf.Called(payload, extraHeaders).Error(0)
}

// SubmitSeries updates the internal mock struct
func (tf *MockedForwarder) SubmitSeries(payload Payloads, extraHeaders map[string]string) error {
	return tf.Called(payload, extraHeaders).Error(0)
}

// SubmitEvents updates the internal mock struct
func (tf *MockedForwarder) SubmitEvents(payload Payloads, extraHeaders map[string]string) error {
	return tf.Called(payload, extraHeaders).Error(0)
}

// SubmitServiceChecks updates the internal mock struct
func (tf *MockedForwarder) SubmitServiceChecks(payload Payloads, extraHeaders map[string]string) error {
	return tf.Called(payload, extraHeaders).Error(0)
}

// SubmitSketchSeries updates the internal mock struct
func (tf *MockedForwarder) SubmitSketchSeries(payload Payloads, extraHeaders map[string]string) error {
	return tf.Called(payload, extraHeaders).Error(0)
}

// SubmitHostMetadata updates the internal mock struct
func (tf *MockedForwarder) SubmitHostMetadata(payload Payloads, extraHeaders map[string]string) error {
	return tf.Called(payload, extraHeaders).Error(0)
}

// SubmitMetadata updates the internal mock struct
func (tf *MockedForwarder) SubmitMetadata(payload Payloads, extraHeaders map[string]string) error {
	return tf.Called(payload, extraHeaders).Error(0)
}
