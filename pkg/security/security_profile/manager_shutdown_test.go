// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package securityprofile

import (
	"context"
	"testing"
	"testing/synctest"

	"github.com/DataDog/datadog-agent/pkg/security/config"
	"github.com/DataDog/datadog-agent/pkg/security/resolvers"
	"github.com/DataDog/datadog-agent/pkg/security/resolvers/tags"
	"github.com/DataDog/datadog-agent/pkg/security/utils"
)

func TestWorkloadCallbacksAfterManagerStops(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		notifier := utils.NewNotifier[tags.Event, *tags.Workload]()
		m := &Manager{
			config:         &config.Config{RuntimeSecurity: &config.RuntimeSecurityConfig{SecurityProfileEnabled: true}},
			resolvers:      &resolvers.EBPFResolvers{TagsResolver: &tags.LinuxResolver{Notifier: notifier}},
			workloadEvents: make(chan *WorkloadEvent, 1),
		}
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		done := make(chan struct{})
		go func() { m.Start(ctx); close(done) }()
		synctest.Wait()
		cancel()
		<-done
		m.workloadEvents <- &WorkloadEvent{}
		notifier.NotifyListeners(tags.WorkloadSelectorResolved, &tags.Workload{})
		notifier.NotifyListeners(tags.WorkloadSelectorDeleted, &tags.Workload{})
	})
}
