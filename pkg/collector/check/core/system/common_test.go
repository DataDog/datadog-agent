package system

import (
	"github.com/stretchr/testify/mock"
)

type MockSender struct {
	mock.Mock
}

func (m *MockSender) Destroy() {
	m.Called()
}

func (m *MockSender) Rate(metric string, value float64, hostname string, tags []string) {
	m.Called(metric, value, hostname, tags)
}

func (m *MockSender) Histogram(metric string, value float64, hostname string, tags []string) {
	m.Called(metric, value, hostname, tags)
}

func (m *MockSender) Commit() {
	m.Called()
}

func (m *MockSender) Gauge(metric string, value float64, hostname string, tags []string) {
	m.Called(metric, value, hostname, tags)
}
