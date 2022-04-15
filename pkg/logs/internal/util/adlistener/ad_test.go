// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package adlistener

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/autodiscovery/scheduler"
	"github.com/stretchr/testify/require"
)

func reset() {
	adMetaSchedulerCh = make(chan *scheduler.MetaScheduler, 1)
}

func emptyChan(ch chan struct{}) bool {
	select {
	case <-ch:
		return false
	default:
		return true
	}
}

func TestListenersWaitToStart(t *testing.T) {
	reset()

	got1 := make(chan struct{}, 1)
	l1 := NewADListener("l1", func([]integration.Config) { got1 <- struct{}{} }, nil)
	l1.StartListener()

	got2 := make(chan struct{}, 1)
	l2 := NewADListener("l2", func([]integration.Config) { got2 <- struct{}{} }, nil)
	l2.StartListener()

	adsched := scheduler.NewMetaScheduler()
	adsched.Schedule([]integration.Config{})

	require.True(t, emptyChan(got1))
	require.True(t, emptyChan(got2))

	SetADMetaScheduler(adsched)

	// wait for the registration to occur before sending a config
	<-l1.registered
	<-l2.registered

	adsched.Schedule([]integration.Config{})

	// wait for each of the two listeners to get notified
	<-got1
	<-got2
}
