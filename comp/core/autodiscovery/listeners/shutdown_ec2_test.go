// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build ec2

package listeners

import (
	"testing"
	"testing/synctest"

	"github.com/DataDog/datadog-agent/pkg/databasemonitoring/aws"
)

func TestDatabaseServiceSendStops(t *testing.T) {
	for _, name := range []string{"rds", "aurora"} {
		t.Run(name, func(t *testing.T) {
			synctest.Test(t, func(_ *testing.T) {
				var listener ServiceListener
				var send func()
				if name == "rds" {
					l := &DBMRdsListener{stop: make(chan bool), services: make(map[string]Service)}
					listener = l
					send = func() { l.createService("id", aws.Instance{}) }
				} else {
					l := &DBMAuroraListener{stop: make(chan bool), services: make(map[string]Service)}
					listener = l
					send = func() { l.createService("id", "postgres", aws.Instance{}) }
				}
				done := make(chan struct{})
				go func() {
					send()
					close(done)
				}()
				synctest.Wait()
				listener.Stop()
				<-done
			})
		})
	}
}
