// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && kubelet && !serverless

package providers

import (
	"context"
	"testing"
	"testing/synctest"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
)

type shutdownStore struct {
	workloadmeta.Component
	events       chan workloadmeta.EventBundle
	unsubscribed chan struct{}
}

func (s *shutdownStore) Subscribe(string, workloadmeta.SubscriberPriority, *workloadmeta.Filter) chan workloadmeta.EventBundle {
	return s.events
}
func (s *shutdownStore) Unsubscribe(chan workloadmeta.EventBundle) { close(s.unsubscribed) }

func TestStreamShutdownWithBlockedOutput(t *testing.T) {
	for _, name := range []string{"container", "prometheus", "process"} {
		t.Run(name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				store := &shutdownStore{events: make(chan workloadmeta.EventBundle), unsubscribed: make(chan struct{})}
				ctx, cancel := context.WithCancel(t.Context())
				defer cancel()
				var stream func(context.Context) <-chan integration.ConfigChanges
				count := 1
				switch name {
				case "container":
					stream = (&ContainerConfigProvider{workloadmetaStore: store}).Stream
				case "prometheus":
					stream = (&PrometheusPodsConfigProvider{workloadmetaStore: store}).Stream
				case "process":
					stream = (&processLogConfigProvider{workloadmetaStore: store}).Stream
					count = 2 // Fill its one-element output buffer before blocking.
				}
				_ = stream(ctx)
				acks := make([]chan struct{}, count)
				for i := range acks {
					acks[i] = make(chan struct{})
					store.events <- workloadmeta.EventBundle{Ch: acks[i]}
				}
				synctest.Wait()
				cancel()
				synctest.Wait()
				select {
				case <-store.unsubscribed:
				default:
					t.Fatal("provider did not unsubscribe")
				}
				for _, ack := range acks {
					select {
					case <-ack:
					default:
						t.Fatal("event was not acknowledged")
					}
				}
			})
		})
	}
}
