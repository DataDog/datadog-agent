// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package adlistener

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/autodiscovery"
	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/autodiscovery/scheduler"
)

func TestListenersGetScheduleCalls(t *testing.T) {
	adsched := scheduler.NewMetaScheduler()
	ac := autodiscovery.NewAutoConfigNoStart(adsched)

	got1 := make(chan struct{}, 1)
	l1 := NewADListener("l1", ac, func(configs []integration.Config) {
		for range configs {
			got1 <- struct{}{}
		}
	}, nil)
	l1.StartListener()

	got2 := make(chan struct{}, 1)
	l2 := NewADListener("l2", ac, func(configs []integration.Config) {
		for range configs {
			got2 <- struct{}{}
		}
	}, nil)
	l2.StartListener()

	adsched.Schedule([]integration.Config{{}})

	// wait for each of the two listeners to get notified
	<-got1
	<-got2
}
