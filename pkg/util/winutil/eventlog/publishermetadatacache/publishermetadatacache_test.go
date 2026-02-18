// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build windows

package publishermetadatacache

import (
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"golang.org/x/sys/windows"

	evtapi "github.com/DataDog/datadog-agent/pkg/util/winutil/eventlog/api"
	fakeevtapi "github.com/DataDog/datadog-agent/pkg/util/winutil/eventlog/api/fake"
)

// mockEvtAPI wraps fakeevtapi and allows mocking specific methods
type mockEvtAPI struct {
	*fakeevtapi.API
	mock.Mock
}

func newMockEvtAPI() *mockEvtAPI {
	return &mockEvtAPI{
		API: fakeevtapi.New(),
	}
}

// Override only the methods we want to mock
func (m *mockEvtAPI) EvtOpenPublisherMetadata(PublisherID string, LogFilePath string) (evtapi.EventPublisherMetadataHandle, error) {
	args := m.Called(PublisherID, LogFilePath)
	return args.Get(0).(evtapi.EventPublisherMetadataHandle), args.Error(1)
}

func (m *mockEvtAPI) EvtFormatMessage(PublisherMetadata evtapi.EventPublisherMetadataHandle, Event evtapi.EventRecordHandle, MessageID uint, Values evtapi.EvtVariantValues, Flags uint) (string, error) {
	args := m.Called(PublisherMetadata, Event, MessageID, Values, Flags)
	return args.Get(0).(string), args.Error(1)
}

func (m *mockEvtAPI) EvtClose(h windows.Handle) {
	m.Called(h)
}

func TestPublisherMetadataCache_Get_Success(t *testing.T) {
	mockAPI := newMockEvtAPI()
	cache := New(mockAPI)

	publisherName1 := "Publisher1"
	publisherName2 := "Publisher2"
	handle1 := evtapi.EventPublisherMetadataHandle(100)
	handle2 := evtapi.EventPublisherMetadataHandle(200)

	mockAPI.On("EvtOpenPublisherMetadata", publisherName1, "").Return(handle1, nil).Once()
	mockAPI.On("EvtOpenPublisherMetadata", publisherName2, "").Return(handle2, nil).Once()

	// First call should create and cache handle1
	result1, err := cache.Get(publisherName1)
	assert.NoError(t, err)
	assert.Equal(t, handle1, result1)

	// Second call should create and cache handle2
	result2, err := cache.Get(publisherName2)
	assert.NoError(t, err)
	assert.Equal(t, handle2, result2)

	// Verify items are in cache
	cachedValue, found := cache.cache.Load(publisherName1)
	assert.True(t, found)
	assert.Equal(t, handle1, cachedValue.(cacheEntry).handle)

	cachedValue, found = cache.cache.Load(publisherName2)
	assert.True(t, found)
	assert.Equal(t, handle2, cachedValue.(cacheEntry).handle)

	// Third call should return cached handle1 (no new API call)
	result1Again, err := cache.Get(publisherName1)
	assert.NoError(t, err)
	assert.Equal(t, handle1, result1Again)

	mockAPI.AssertExpectations(t)
}

func TestPublisherMetadataCache_Get_Error(t *testing.T) {
	mockAPI := newMockEvtAPI()
	cache := New(mockAPI)

	publisherName := "NonExistentPublisher"
	expectedErr := errors.New("publisher not found")

	mockAPI.On("EvtOpenPublisherMetadata", publisherName, "").Return(evtapi.EventPublisherMetadataHandle(0), expectedErr).Once()

	// Should return error and cache InvalidHandle
	handle, err := cache.Get(publisherName)
	assert.Equal(t, expectedErr, err)
	assert.Equal(t, invalidHandle, handle)

	// Verify invalid handle is cached
	cachedValue, found := cache.cache.Load(publisherName)
	assert.True(t, found)
	assert.Equal(t, invalidHandle, cachedValue.(cacheEntry).handle)
	assert.Equal(t, expectedErr, cachedValue.(cacheEntry).err)

	// Second call should return cached error (no new API call within expiration)
	handle, err = cache.Get(publisherName)
	assert.Equal(t, expectedErr, err)
	assert.Equal(t, invalidHandle, handle)

	mockAPI.AssertExpectations(t)
}

func TestPublisherMetadataCache_Get_InvalidHandleExpiration(t *testing.T) {
	mockAPI := newMockEvtAPI()
	cache := New(mockAPI)
	cache.expiration = 10 * time.Millisecond // Short expiration for testing

	publisherName := "PublisherWithError"
	expectedErr := errors.New("publisher not available")
	validHandle := evtapi.EventPublisherMetadataHandle(123)

	// First call returns error
	mockAPI.On("EvtOpenPublisherMetadata", publisherName, "").Return(evtapi.EventPublisherMetadataHandle(0), expectedErr).Once()
	handle, err := cache.Get(publisherName)
	assert.Equal(t, expectedErr, err)
	assert.Equal(t, invalidHandle, handle)

	// Wait for expiration
	time.Sleep(15 * time.Millisecond)

	// After expiration, should retry and succeed
	mockAPI.On("EvtOpenPublisherMetadata", publisherName, "").Return(validHandle, nil).Once()
	handle, err = cache.Get(publisherName)
	assert.NoError(t, err)
	assert.Equal(t, validHandle, handle)

	mockAPI.AssertExpectations(t)
}

func TestPublisherMetadataCache_FormatMessage_Success(t *testing.T) {
	mockAPI := newMockEvtAPI()
	cache := New(mockAPI)

	publisherName := "TestPublisher"
	eventHandle := evtapi.EventRecordHandle(100)
	pubHandle := evtapi.EventPublisherMetadataHandle(42)
	expectedMessage := "Test event message"

	mockAPI.On("EvtOpenPublisherMetadata", publisherName, "").Return(pubHandle, nil).Once()
	mockAPI.On("EvtFormatMessage", pubHandle, eventHandle, uint(0), evtapi.EvtVariantValues(nil), uint(0)).Return(expectedMessage, nil).Once()

	message, err := cache.FormatMessage(publisherName, eventHandle, 0)
	assert.NoError(t, err)
	assert.Equal(t, expectedMessage, message)

	// Verify handle is still cached
	_, found := cache.cache.Load(publisherName)
	assert.True(t, found)

	mockAPI.AssertExpectations(t)
}

func TestPublisherMetadataCache_FormatMessage_MessageNotFoundError(t *testing.T) {
	mockAPI := newMockEvtAPI()
	cache := New(mockAPI)

	publisherName := "TestPublisher"
	eventHandle := evtapi.EventRecordHandle(100)
	pubHandle := evtapi.EventPublisherMetadataHandle(42)

	mockAPI.On("EvtOpenPublisherMetadata", publisherName, "").Return(pubHandle, nil).Once()
	mockAPI.On("EvtFormatMessage", pubHandle, eventHandle, uint(0), evtapi.EvtVariantValues(nil), uint(0)).
		Return("", windows.ERROR_EVT_MESSAGE_NOT_FOUND).Once()

	message, err := cache.FormatMessage(publisherName, eventHandle, 0)
	assert.Empty(t, message)
	assert.Equal(t, windows.ERROR_EVT_MESSAGE_NOT_FOUND, err)

	// Verify handle is STILL cached (expected error doesn't invalidate cache)
	_, found := cache.cache.Load(publisherName)
	assert.True(t, found)

	mockAPI.AssertExpectations(t)
}

func TestPublisherMetadataCache_FormatMessage_UnexpectedError(t *testing.T) {
	mockAPI := newMockEvtAPI()
	cache := New(mockAPI)

	publisherName := "TestPublisher"
	eventHandle := evtapi.EventRecordHandle(100)
	pubHandle := evtapi.EventPublisherMetadataHandle(42)
	unexpectedErr := errors.New("unexpected error")

	mockAPI.On("EvtOpenPublisherMetadata", publisherName, "").Return(pubHandle, nil).Once()
	mockAPI.On("EvtFormatMessage", pubHandle, eventHandle, uint(0), evtapi.EvtVariantValues(nil), uint(0)).
		Return("", unexpectedErr).Once()
	mockAPI.On("EvtClose", windows.Handle(pubHandle)).Once()

	message, err := cache.FormatMessage(publisherName, eventHandle, 0)
	assert.Empty(t, message)
	assert.Equal(t, unexpectedErr, err)

	// Verify cache entry was removed
	_, found := cache.cache.Load(publisherName)
	assert.False(t, found)

	mockAPI.AssertExpectations(t)
}

func TestPublisherMetadataCache_FormatMessage_WithInvalidHandle(t *testing.T) {
	mockAPI := newMockEvtAPI()
	cache := New(mockAPI)

	publisherName := "TestPublisher"
	eventHandle := evtapi.EventRecordHandle(100)
	expectedErr := errors.New("publisher not found")

	mockAPI.On("EvtOpenPublisherMetadata", publisherName, "").Return(evtapi.EventPublisherMetadataHandle(0), expectedErr).Once()

	// FormatMessage should return early with InvalidHandle
	message, err := cache.FormatMessage(publisherName, eventHandle, 0)
	assert.Empty(t, message)
	assert.Equal(t, expectedErr, err)

	// Verify cache still contains invalid handle
	cachedValue, found := cache.cache.Load(publisherName)
	assert.True(t, found)
	assert.Equal(t, invalidHandle, cachedValue.(cacheEntry).handle)

	mockAPI.AssertExpectations(t)
}

func TestPublisherMetadataCache_Flush_CleansUpAllHandles(t *testing.T) {
	mockAPI := newMockEvtAPI()
	cache := New(mockAPI)

	publisher1 := "Publisher1"
	publisher2 := "Publisher2"
	handle1 := evtapi.EventPublisherMetadataHandle(100)
	handle2 := evtapi.EventPublisherMetadataHandle(200)

	mockAPI.On("EvtOpenPublisherMetadata", publisher1, "").Return(handle1, nil).Once()
	mockAPI.On("EvtOpenPublisherMetadata", publisher2, "").Return(handle2, nil).Once()

	cache.Get(publisher1)
	cache.Get(publisher2)

	// Verify items are in cache before flushing
	_, found1 := cache.cache.Load(publisher1)
	assert.True(t, found1)
	_, found2 := cache.cache.Load(publisher2)
	assert.True(t, found2)

	// Expect EvtClose calls for valid handles
	mockAPI.On("EvtClose", windows.Handle(handle1)).Once()
	mockAPI.On("EvtClose", windows.Handle(handle2)).Once()

	cache.Flush()

	// Verify cache is empty after flush
	_, found1 = cache.cache.Load(publisher1)
	assert.False(t, found1)
	_, found2 = cache.cache.Load(publisher2)
	assert.False(t, found2)

	mockAPI.AssertExpectations(t)
}

func TestPublisherMetadataCache_Flush_SkipsInvalidHandles(t *testing.T) {
	mockAPI := newMockEvtAPI()
	cache := New(mockAPI)

	validPublisher := "ValidPublisher"
	invalidPublisher := "InvalidPublisher"
	validHandle := evtapi.EventPublisherMetadataHandle(100)

	mockAPI.On("EvtOpenPublisherMetadata", validPublisher, "").Return(validHandle, nil).Once()
	mockAPI.On("EvtOpenPublisherMetadata", invalidPublisher, "").Return(evtapi.EventPublisherMetadataHandle(0), errors.New("not found")).Once()

	cache.Get(validPublisher)
	cache.Get(invalidPublisher)

	// Expect EvtClose only for valid handle
	mockAPI.On("EvtClose", windows.Handle(validHandle)).Once()

	cache.Flush()

	mockAPI.AssertExpectations(t)
}

func TestPublisherMetadataCache_Concurrency(_ *testing.T) {
	mockAPI := newMockEvtAPI()
	cache := New(mockAPI)

	publishers := []string{"Publisher1", "Publisher2", "Publisher3"}
	eventHandle := evtapi.EventRecordHandle(100)
	numGoroutinesPerPublisher := 10

	for i, publisher := range publishers {
		handle := evtapi.EventPublisherMetadataHandle(100 + i)
		mockAPI.On("EvtOpenPublisherMetadata", publisher, "").Return(handle, nil).Once()
		mockAPI.On("EvtFormatMessage", handle, eventHandle, uint(0), evtapi.EvtVariantValues(nil), uint(0)).
			Return("Message from "+publisher, nil).Times(100 * numGoroutinesPerPublisher)
	}

	var wg sync.WaitGroup

	// Launch multiple goroutines for each publisher
	for _, publisher := range publishers {
		for range numGoroutinesPerPublisher {
			wg.Add(1)
			go func(pub string) {
				defer wg.Done()
				for range 100 {
					cache.FormatMessage(pub, eventHandle, 0)
				}
			}(publisher)
		}
	}

	wg.Wait()
}
