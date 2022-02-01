// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package workloadmeta

const (
	mockSource = Source("mockSource")
)

// MockStore is a store designed to be used in unit tests
type MockStore struct {
	*store
}

// NewMockStore returns a MockStore
func NewMockStore() *MockStore {
	return &MockStore{
		store: newStore(nil),
	}
}

// Notify overrides store to allow for synchronous event processing
func (ms *MockStore) Notify(events []CollectorEvent) {
	ms.handleEvents(events)
}

// Extra interface to ease working with MockStore

// SetEntity generates a Set event
func (ms *MockStore) SetEntity(e Entity) {
	ms.Notify([]CollectorEvent{
		{
			Type:   EventTypeSet,
			Source: mockSource,
			Entity: e,
		},
	})
}

// UnsetEntity generates an Unset event
func (ms *MockStore) UnsetEntity(e Entity) {
	ms.Notify([]CollectorEvent{
		{
			Type:   EventTypeUnset,
			Source: mockSource,
			Entity: e,
		},
	})
}
