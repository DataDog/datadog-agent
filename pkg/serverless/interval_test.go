// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package serverless

import (
	"fmt"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/serverless/flush"
	"github.com/stretchr/testify/assert"
)

func TestAutoSelectStrategy(t *testing.T) {
	assert := assert.New(t)
	d := Daemon{
		lastInvocations: make([]time.Time, 0),
		flushStrategy:   &flush.AtTheEnd{},
		clientLibReady:  false,
	}

	now := time.Now()
	// when the client library hasn't registered with the extension,
	// fallback to periodically strategy
	d.clientLibReady = false
	assert.Equal((flush.NewPeriodically(10 * time.Second)).String(), d.AutoSelectStrategy().String(), "wrong strategy has been selected") // default strategy

	// when not enough data, the flush at the end strategy should be selected
	// -----
	d.clientLibReady = true

	assert.Equal((&flush.AtTheEnd{}).String(), d.AutoSelectStrategy().String(), "not the good strategy has been selected") // default strategy

	assert.True(d.StoreInvocationTime(now.Add(-time.Second * 140)))
	assert.Equal((&flush.AtTheEnd{}).String(), d.AutoSelectStrategy().String(), "not the good strategy has been selected")
	assert.True(d.StoreInvocationTime(now.Add(-time.Second * 70)))
	assert.Equal((&flush.AtTheEnd{}).String(), d.AutoSelectStrategy().String(), "not the good strategy has been selected")

	// add a third invocation, after this, we have enough data to decide to switch
	// to the "periodically" strategy since the function is invoked more often
	// than 1 time a minute.
	// -----

	assert.True(d.StoreInvocationTime(now.Add(-time.Second * 1)))
	assert.Equal(flush.NewPeriodically(10*time.Second).String(), d.AutoSelectStrategy().String(), "not the good strategy has been selected")

	// simulate a function invoked less than 1 time a minute
	// -----

	// reset the data
	d.lastInvocations = make([]time.Time, 0)
	assert.Equal((&flush.AtTheEnd{}).String(), d.AutoSelectStrategy().String(), "not the good strategy has been selected") // default strategy

	assert.True(d.StoreInvocationTime(now.Add(-time.Minute * 16)))
	assert.Equal((&flush.AtTheEnd{}).String(), d.AutoSelectStrategy().String(), "not the good strategy has been selected")
	assert.True(d.StoreInvocationTime(now.Add(-time.Minute * 10)))
	assert.Equal((&flush.AtTheEnd{}).String(), d.AutoSelectStrategy().String(), "not the good strategy has been selected")
	assert.True(d.StoreInvocationTime(now.Add(-time.Minute * 6)))
	// because of the interval, we should kept the "flush at the end" strategy
	fmt.Println(d.InvocationInterval())
	assert.Equal((&flush.AtTheEnd{}).String(), d.AutoSelectStrategy().String(), "not the good strategy has been selected")
}

func TestStoreInvocationTime(t *testing.T) {
	assert := assert.New(t)
	d := Daemon{
		lastInvocations: make([]time.Time, 0),
		flushStrategy:   &flush.AtTheEnd{},
		clientLibReady:  true,
	}

	now := time.Now()
	for i := 100; i > 0; i-- {
		d.StoreInvocationTime(now.Add(-time.Second * time.Duration(i)))
	}

	assert.True(len(d.lastInvocations) <= maxInvocationsStored, "the amount of stored invocations should be lower or equal to 50")
	// validate that the proper entries were removed
	assert.Equal(now.Add(-time.Second*10), d.lastInvocations[0])
	assert.Equal(now.Add(-time.Second*9), d.lastInvocations[1])
}

func TestInvocationInterval(t *testing.T) {
	assert := assert.New(t)

	d := Daemon{
		lastInvocations: make([]time.Time, 0),
		flushStrategy:   &flush.AtTheEnd{},
		clientLibReady:  true,
	}

	// first scenario, validate that we're not computing the interval if we only have 2 invocations done
	// -----

	for i := 0; i < 2; i++ {
		time.Sleep(100 * time.Millisecond)
		d.lastInvocations = append(d.lastInvocations, time.Now())
		assert.Equal(time.Duration(0), d.InvocationInterval(), "we should not compute any interval just yet since we don't have enough data")
	}
	time.Sleep(100 * time.Millisecond)
	d.lastInvocations = append(d.lastInvocations, time.Now())

	//	assert.Equal(d.InvocationInterval(), time.Duration(0), "we should not compute any interval just yet since we don't have enough data")
	assert.NotEqual(time.Duration(0), d.InvocationInterval(), "we should compute some interval now")

	// second scenario, validate the interval that has been computed
	// -----

	// reset the data
	d.lastInvocations = make([]time.Time, 0)

	// function executed every second

	now := time.Now()
	for i := 100; i > 1; i-- {
		d.StoreInvocationTime(now.Add(-time.Second * time.Duration(i)))
	}

	// because we've added 50 execution, one every last 50 seconds, the interval
	// computed between each function execution should be 1s
	assert.Equal(maxInvocationsStored, len(d.lastInvocations), fmt.Sprintf("the amount of invocations stored should be %d", maxInvocationsStored))
	assert.Equal(time.Second, d.InvocationInterval(), "the compute interval should be 1s")

	// function executed 100ms

	for i := 100; i > 1; i-- {
		d.StoreInvocationTime(now.Add(-time.Millisecond * 10 * time.Duration(i)))
	}

	// because we've added 50 execution, one every last 50 seconds, the interval
	// computed between each function execution should be 1s
	assert.Equal(maxInvocationsStored, len(d.lastInvocations), fmt.Sprintf("the amount of invocations stored should be %d", maxInvocationsStored))
	assert.Equal(time.Millisecond*10, d.InvocationInterval(), "the compute interval should be 100ms")
}

func TestUpdateStrategy(t *testing.T) {
	assert := assert.New(t)

	d := Daemon{
		lastInvocations:  make([]time.Time, 0),
		flushStrategy:    flush.NewPeriodically(10 * time.Second),
		clientLibReady:   true,
		useAdaptiveFlush: false,
	}

	d.UpdateStrategy()
	assert.Equal(d.flushStrategy, flush.NewPeriodically(10*time.Second), "strategy changed when useAdaptiveFlush was false")

	d.useAdaptiveFlush = true
	d.UpdateStrategy()

	assert.Equal(d.flushStrategy, &flush.AtTheEnd{}, "strategy didn't change when useAdaptiveFlush was true")
}
