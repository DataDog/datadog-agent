package aggregator

import (
	// stdlib
	"testing"

	// 3p
	"github.com/stretchr/testify/assert"
)

func TestGenerateContextKey(t *testing.T) {
	mSample := MetricSample{
		Name:       "my.metric.name",
		Value:      1,
		Mtype:      GaugeType,
		Tags:       &[]string{"foo", "bar"},
		SampleRate: 1,
	}

	contextKey := generateContextKey(&mSample)
	assert.Equal(t, "my.metric.name,bar,foo", contextKey)
}

func TestTrackContext(t *testing.T) {
	mSample1 := MetricSample{
		Name:       "my.metric.name",
		Value:      1,
		Mtype:      GaugeType,
		Tags:       &[]string{"foo", "bar"},
		SampleRate: 1,
	}
	mSample2 := MetricSample{
		Name:       "my.metric.name",
		Value:      1,
		Mtype:      GaugeType,
		Tags:       &[]string{"foo", "bar", "baz"},
		SampleRate: 1,
	}
	expectedContext1 := Context{
		Name: mSample1.Name,
		Tags: *(mSample1.Tags),
	}
	expectedContext2 := Context{
		Name: mSample2.Name,
		Tags: *(mSample2.Tags),
	}
	contextResolver := newContextResolver()

	// Track the 2 contexts
	contextKey1 := contextResolver.trackContext(&mSample1, 1)
	contextKey2 := contextResolver.trackContext(&mSample2, 1)

	// When we look up the 2 keys, they return the correct contexts
	context1 := contextResolver.contextsByKey[contextKey1]
	assert.Equal(t, expectedContext1, *context1)

	context2 := contextResolver.contextsByKey[contextKey2]
	assert.Equal(t, expectedContext2, *context2)

	// Looking for a missing context key returns an error
	_, ok := contextResolver.contextsByKey["missingContextKey"]
	assert.False(t, ok)
}

func TestExpireContexts(t *testing.T) {
	mSample1 := MetricSample{
		Name:       "my.metric.name",
		Value:      1,
		Mtype:      GaugeType,
		Tags:       &[]string{"foo", "bar"},
		SampleRate: 1,
	}
	mSample2 := MetricSample{
		Name:       "my.metric.name",
		Value:      1,
		Mtype:      GaugeType,
		Tags:       &[]string{"foo", "bar", "baz"},
		SampleRate: 1,
	}
	contextResolver := newContextResolver()

	// Track the 2 contexts
	contextKey1 := contextResolver.trackContext(&mSample1, 4)
	contextKey2 := contextResolver.trackContext(&mSample2, 6)

	// With an expireTimestap of 3, both contexts are still valid
	assert.Len(t, contextResolver.expireContexts(3), 0)
	_, ok1 := contextResolver.contextsByKey[contextKey1]
	_, ok2 := contextResolver.contextsByKey[contextKey2]
	assert.True(t, ok1)
	assert.True(t, ok2)

	// With an expireTimestap of 5, context 1 is expired
	expiredContextKeys := contextResolver.expireContexts(5)
	if assert.Len(t, expiredContextKeys, 1) {
		assert.Equal(t, contextKey1, expiredContextKeys[0])
	}

	// context 1 is not tracked anymore, but context 2 still is
	_, ok := contextResolver.contextsByKey[contextKey1]
	assert.False(t, ok)
	_, ok = contextResolver.contextsByKey[contextKey2]
	assert.True(t, ok)
}
