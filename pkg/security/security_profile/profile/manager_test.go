// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package profile holds profile related files
package profile

import (
	"fmt"
	"math/rand"
	"testing"
	"time"
	"unsafe"

	"github.com/stretchr/testify/assert"
	"go.uber.org/atomic"

	"github.com/DataDog/datadog-agent/pkg/security/config"
	cgroupModel "github.com/DataDog/datadog-agent/pkg/security/resolvers/cgroup/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	activity_tree "github.com/DataDog/datadog-agent/pkg/security/security_profile/activity_tree"
)

type testIteration struct {
	name                string                     // test name
	result              EventFilteringProfileState // expected result
	newProfile          bool                       // true if a new profile have to be generated for this test
	containerCreatedAt  time.Duration              // time diff from t0
	addFakeProcessNodes int64                      // number of fake process nodes to add (adds 1024 to approx size)
	eventTimestampRaw   time.Duration              // time diff from t0
	eventType           model.EventType            // only exec for now, TODO: add dns
	eventProcessPath    string                     // exec path
	eventDNSReq         string                     // dns request name (only for eventType == DNSEventType)
	loopUntil           time.Duration              // if not 0, will loop until the given duration is reached
	loopIncrement       time.Duration              // if loopUntil is not 0, will increment this duration at each loop
}

func craftFakeEvent(t0 time.Time, ti *testIteration, defaultContainerID string) *model.Event {
	event := model.NewDefaultEvent()
	event.Type = uint32(ti.eventType)
	event.ContainerContext.CreatedAt = uint64(t0.Add(ti.containerCreatedAt).UnixNano())
	event.TimestampRaw = uint64(t0.Add(ti.eventTimestampRaw).UnixNano())
	event.Timestamp = t0.Add(ti.eventTimestampRaw)

	// setting process
	event.ProcessCacheEntry = model.NewPlaceholderProcessCacheEntry(42, 42, false)
	event.ProcessCacheEntry.ContainerID = defaultContainerID
	event.ProcessCacheEntry.FileEvent.PathnameStr = ti.eventProcessPath
	event.ProcessCacheEntry.FileEvent.Inode = 42
	event.ProcessCacheEntry.Args = "foo"
	event.ProcessContext = &event.ProcessCacheEntry.ProcessContext
	switch ti.eventType {
	case model.ExecEventType:
		event.Exec.Process = &event.ProcessCacheEntry.ProcessContext.Process
		break
	case model.DNSEventType:
		event.DNS.Name = ti.eventDNSReq
		event.DNS.Type = 1  // A
		event.DNS.Class = 1 // INET
		event.DNS.Size = uint16(len(ti.eventDNSReq))
		event.DNS.Count = 1
		break
	}

	// setting process ancestor
	event.ProcessCacheEntry.Ancestor = model.NewPlaceholderProcessCacheEntry(1, 1, false)
	event.ProcessCacheEntry.Ancestor.FileEvent.PathnameStr = "systemd"
	event.ProcessCacheEntry.Ancestor.FileEvent.Inode = 41
	event.ProcessCacheEntry.Ancestor.Args = "foo"
	return event
}

func TestSecurityProfileManager_tryAutolearn(t *testing.T) {
	AnomalyDetectionMinimumStablePeriod := time.Hour
	AnomalyDetectionWorkloadWarmupPeriod := time.Minute
	AnomalyDetectionUnstableProfileTimeThreshold := time.Hour * 48
	MaxNbProcess := int64(1000)
	AnomalyDetectionUnstableProfileSizeThreshold := int64(unsafe.Sizeof(activity_tree.ProcessNode{})) * MaxNbProcess
	defaultContainerID := "424242424242424242424242424242424242424242424242424242424242424"

	tests := []testIteration{
		// checking warmup period for exec:
		{
			name:                "warmup-exec/not-warmup",
			result:              AutoLearning,
			newProfile:          true,
			containerCreatedAt:  -AnomalyDetectionWorkloadWarmupPeriod - time.Second,
			addFakeProcessNodes: 0,
			eventTimestampRaw:   0,
			eventType:           model.ExecEventType,
			eventProcessPath:    "/bin/foo0",
		},
		{
			name:                "warmup-exec/warmup",
			result:              WorkloadWarmup,
			newProfile:          true,
			containerCreatedAt:  -AnomalyDetectionWorkloadWarmupPeriod + time.Second,
			addFakeProcessNodes: 0,
			eventTimestampRaw:   0,
			eventType:           model.ExecEventType,
			eventProcessPath:    "/bin/foo0",
		},
		// and for dns:
		{
			name:                "warmup-dns/not-warmup",
			result:              AutoLearning,
			newProfile:          true,
			containerCreatedAt:  -AnomalyDetectionWorkloadWarmupPeriod - time.Second,
			addFakeProcessNodes: 0,
			eventTimestampRaw:   0,
			eventType:           model.DNSEventType,
			eventProcessPath:    "/bin/foo0",
			eventDNSReq:         "foo.bar",
		},
		{
			name:                "warmup-dns/warmup",
			result:              WorkloadWarmup,
			newProfile:          true,
			containerCreatedAt:  -AnomalyDetectionWorkloadWarmupPeriod + time.Second,
			addFakeProcessNodes: 0,
			eventTimestampRaw:   0,
			eventType:           model.DNSEventType,
			eventProcessPath:    "/bin/foo0",
			eventDNSReq:         "foo.baz",
		},

		// checking stable period for exec:
		{
			name:                "stable-exec/add-first-event",
			result:              AutoLearning,
			newProfile:          true,
			containerCreatedAt:  time.Minute * -5,
			addFakeProcessNodes: 0,
			eventTimestampRaw:   time.Second,
			eventType:           model.ExecEventType,
			eventProcessPath:    "/bin/foo0",
		},
		{
			name:                "stable-exec/add-second-event",
			result:              AutoLearning,
			newProfile:          false,
			containerCreatedAt:  time.Minute * -5,
			addFakeProcessNodes: 0,
			eventTimestampRaw:   time.Second * 2,
			eventType:           model.ExecEventType,
			eventProcessPath:    "/bin/foo1",
		},
		{
			name:                "stable-exec/wait-stable-period",
			result:              StableEventType,
			newProfile:          false,
			containerCreatedAt:  time.Minute * -5,
			addFakeProcessNodes: 0,
			eventTimestampRaw:   time.Second*2 + AnomalyDetectionMinimumStablePeriod,
			eventType:           model.ExecEventType,
			eventProcessPath:    "/bin/foo2",
		},
		{
			name:                "stable-exec/still-stable",
			result:              StableEventType,
			newProfile:          false,
			containerCreatedAt:  time.Minute * -5,
			addFakeProcessNodes: 0,
			eventTimestampRaw:   time.Second*3 + AnomalyDetectionMinimumStablePeriod,
			eventType:           model.ExecEventType,
			eventProcessPath:    "/bin/foo3",
		},
		{
			name:                "stable-exec/dont-get-unstable",
			result:              StableEventType,
			newProfile:          false,
			containerCreatedAt:  time.Minute * -5,
			addFakeProcessNodes: 0,
			eventTimestampRaw:   AnomalyDetectionUnstableProfileTimeThreshold + time.Minute,
			eventType:           model.ExecEventType,
			eventProcessPath:    "/bin/foo4",
		},
		{
			name:                "stable-exec/meanwhile-dns-still-learning",
			result:              AutoLearning,
			newProfile:          false,
			containerCreatedAt:  time.Minute * -5,
			addFakeProcessNodes: 0,
			eventTimestampRaw:   time.Second,
			eventType:           model.DNSEventType,
			eventProcessPath:    "/bin/foo",
			eventDNSReq:         "foo0.bar",
		},
		// and for dns:
		{
			name:                "stable-dns/add-first-event",
			result:              AutoLearning,
			newProfile:          true,
			containerCreatedAt:  time.Minute * -5,
			addFakeProcessNodes: 0,
			eventTimestampRaw:   time.Second,
			eventType:           model.DNSEventType,
			eventProcessPath:    "/bin/foo",
			eventDNSReq:         "foo0.bar",
		},
		{
			name:                "stable-dns/add-second-event",
			result:              AutoLearning,
			newProfile:          false,
			containerCreatedAt:  time.Minute * -5,
			addFakeProcessNodes: 0,
			eventTimestampRaw:   time.Second * 2,
			eventType:           model.DNSEventType,
			eventProcessPath:    "/bin/foo",
			eventDNSReq:         "foo1.bar",
		},
		{
			name:                "stable-dns/wait-stable-period",
			result:              StableEventType,
			newProfile:          false,
			containerCreatedAt:  time.Minute * -5,
			addFakeProcessNodes: 0,
			eventTimestampRaw:   time.Second*2 + AnomalyDetectionMinimumStablePeriod,
			eventType:           model.DNSEventType,
			eventProcessPath:    "/bin/foo",
			eventDNSReq:         "foo2.bar",
		},
		{
			name:                "stable-dns/still-stable",
			result:              StableEventType,
			newProfile:          false,
			containerCreatedAt:  time.Minute * -5,
			addFakeProcessNodes: 0,
			eventTimestampRaw:   time.Second*3 + AnomalyDetectionMinimumStablePeriod,
			eventType:           model.DNSEventType,
			eventProcessPath:    "/bin/foo",
			eventDNSReq:         "foo3.bar",
		},
		{
			name:                "stable-dns/dont-get-unstable",
			result:              StableEventType,
			newProfile:          false,
			containerCreatedAt:  time.Minute * -5,
			addFakeProcessNodes: 0,
			eventTimestampRaw:   AnomalyDetectionUnstableProfileTimeThreshold + time.Minute,
			eventType:           model.DNSEventType,
			eventProcessPath:    "/bin/foo",
			eventDNSReq:         "foo4.bar",
		},
		{
			name:                "stable-dns/meanwhile-exec-still-learning",
			result:              AutoLearning,
			newProfile:          false,
			containerCreatedAt:  time.Minute * -5,
			addFakeProcessNodes: 0,
			eventTimestampRaw:   time.Second,
			eventType:           model.ExecEventType,
			eventProcessPath:    "/bin/foo1",
		},

		// checking unstable period for exec:
		{
			name:                "unstable-exec/wait-unstable-period",
			result:              AutoLearning,
			newProfile:          true,
			containerCreatedAt:  time.Minute * -5,
			addFakeProcessNodes: 0,
			eventTimestampRaw:   0,
			eventType:           model.ExecEventType,
			eventProcessPath:    "/bin/foo_",
			loopUntil:           AnomalyDetectionUnstableProfileTimeThreshold - time.Second,
			loopIncrement:       AnomalyDetectionMinimumStablePeriod - time.Second,
		},
		{
			name:                "unstable-exec/still-unstable",
			result:              UnstableEventType,
			newProfile:          false,
			containerCreatedAt:  time.Minute * -5,
			addFakeProcessNodes: 0,
			eventTimestampRaw:   AnomalyDetectionUnstableProfileTimeThreshold + time.Second,
			eventType:           model.ExecEventType,
			eventProcessPath:    "/bin/foo1",
		},
		{
			name:                "unstable-exec/still-unstable-after-stable-period",
			result:              UnstableEventType,
			newProfile:          false,
			containerCreatedAt:  time.Minute * -5,
			addFakeProcessNodes: 0,
			eventTimestampRaw:   AnomalyDetectionUnstableProfileTimeThreshold + AnomalyDetectionMinimumStablePeriod + time.Second,
			eventType:           model.ExecEventType,
			eventProcessPath:    "/bin/foo2",
		},
		{
			name:                "unstable-exec/meanwhile-dns-still-learning",
			result:              AutoLearning,
			newProfile:          false,
			containerCreatedAt:  time.Minute * -5,
			addFakeProcessNodes: 0,
			eventTimestampRaw:   time.Second,
			eventType:           model.DNSEventType,
			eventProcessPath:    "/bin/foo",
			eventDNSReq:         "foo0.bar",
		},
		// and for dns:
		{
			name:                "unstable-dns/wait-unstable-period",
			result:              AutoLearning,
			newProfile:          true,
			containerCreatedAt:  time.Minute * -5,
			addFakeProcessNodes: 0,
			eventTimestampRaw:   0,
			eventType:           model.DNSEventType,
			eventProcessPath:    "/bin/foo",
			eventDNSReq:         ".foo.bar",
			loopUntil:           AnomalyDetectionUnstableProfileTimeThreshold - time.Second,
			loopIncrement:       AnomalyDetectionMinimumStablePeriod - time.Second,
		},
		{
			name:                "unstable-dns/still-unstable",
			result:              UnstableEventType,
			newProfile:          false,
			containerCreatedAt:  time.Minute * -5,
			addFakeProcessNodes: 0,
			eventTimestampRaw:   AnomalyDetectionUnstableProfileTimeThreshold + time.Second,
			eventType:           model.DNSEventType,
			eventProcessPath:    "/bin/foo",
			eventDNSReq:         "foo1.bar",
		},
		{
			name:                "unstable-dns/still-unstable-after-stable-period",
			result:              UnstableEventType,
			newProfile:          false,
			containerCreatedAt:  time.Minute * -5,
			addFakeProcessNodes: 0,
			eventTimestampRaw:   AnomalyDetectionUnstableProfileTimeThreshold + AnomalyDetectionMinimumStablePeriod + time.Second,
			eventType:           model.DNSEventType,
			eventProcessPath:    "/bin/foo",
			eventDNSReq:         "foo2.bar",
		},
		{
			name:                "unstable-dns/meanwhile-exec-still-learning",
			result:              AutoLearning,
			newProfile:          false,
			containerCreatedAt:  time.Minute * -5,
			addFakeProcessNodes: 0,
			eventTimestampRaw:   time.Second,
			eventType:           model.ExecEventType,
			eventProcessPath:    "/bin/foo1",
		},

		// checking max size threshold different cases for exec:
		{
			name:                "profile-at-max-size-exec/add-first-event",
			result:              WorkloadWarmup,
			newProfile:          true,
			containerCreatedAt:  0,
			addFakeProcessNodes: 0,
			eventTimestampRaw:   time.Second,
			eventType:           model.ExecEventType,
			eventProcessPath:    "/bin/foo0",
		},
		{
			name:                "profile-at-max-size-exec/warmup-stable",
			result:              WorkloadWarmup,
			newProfile:          false,
			containerCreatedAt:  0,
			addFakeProcessNodes: MaxNbProcess,
			eventTimestampRaw:   time.Second,
			eventType:           model.ExecEventType,
			eventProcessPath:    "/bin/foo0",
		},
		{
			name:                "profile-at-max-size-exec/warmup-unstable",
			result:              ProfileAtMaxSize,
			newProfile:          false,
			containerCreatedAt:  0,
			addFakeProcessNodes: MaxNbProcess,
			eventTimestampRaw:   time.Second,
			eventType:           model.ExecEventType,
			eventProcessPath:    "/bin/foo1",
		},
		{
			name:                "profile-at-max-size-exec/stable",
			result:              ProfileAtMaxSize,
			newProfile:          false,
			containerCreatedAt:  time.Minute * -5,
			addFakeProcessNodes: MaxNbProcess,
			eventTimestampRaw:   time.Second,
			eventType:           model.ExecEventType,
			eventProcessPath:    "/bin/foo0",
		},
		{
			name:                "profile-at-max-size-exec/unstable",
			result:              ProfileAtMaxSize,
			newProfile:          false,
			containerCreatedAt:  time.Minute * -5,
			addFakeProcessNodes: MaxNbProcess,
			eventTimestampRaw:   time.Second,
			eventType:           model.ExecEventType,
			eventProcessPath:    "/bin/foo1",
		},
		{
			name:                "profile-at-max-size-exec/warmup-NOT-at-max-size",
			result:              WorkloadWarmup,
			newProfile:          true,
			containerCreatedAt:  0,
			addFakeProcessNodes: MaxNbProcess - 1,
			eventTimestampRaw:   time.Second,
			eventType:           model.ExecEventType,
			eventProcessPath:    "/bin/foo2",
		},
		{
			name:                "profile-at-max-size-exec/NOT-at-max-size",
			result:              AutoLearning,
			newProfile:          true,
			containerCreatedAt:  time.Minute * -5,
			addFakeProcessNodes: MaxNbProcess - 1,
			eventTimestampRaw:   time.Second,
			eventType:           model.ExecEventType,
			eventProcessPath:    "/bin/foo2",
		},
		// and for dns:
		{
			name:                "profile-at-max-size-dns/add-first-event",
			result:              WorkloadWarmup,
			newProfile:          true,
			containerCreatedAt:  0,
			addFakeProcessNodes: 0,
			eventTimestampRaw:   time.Second,
			eventType:           model.DNSEventType,
			eventProcessPath:    "/bin/foo",
			eventDNSReq:         "foo0.bar",
		},
		{
			name:                "profile-at-max-size-dns/warmup-stable",
			result:              WorkloadWarmup,
			newProfile:          false,
			containerCreatedAt:  0,
			addFakeProcessNodes: MaxNbProcess,
			eventTimestampRaw:   time.Second,
			eventType:           model.DNSEventType,
			eventProcessPath:    "/bin/foo",
			eventDNSReq:         "foo0.bar",
		},
		{
			name:                "profile-at-max-size-dns/warmup-unstable",
			result:              ProfileAtMaxSize,
			newProfile:          false,
			containerCreatedAt:  0,
			addFakeProcessNodes: MaxNbProcess,
			eventTimestampRaw:   time.Second,
			eventType:           model.DNSEventType,
			eventProcessPath:    "/bin/foo",
			eventDNSReq:         "foo1.bar",
		},
		{
			name:                "profile-at-max-size-dns/stable",
			result:              ProfileAtMaxSize,
			newProfile:          false,
			containerCreatedAt:  time.Minute * -5,
			addFakeProcessNodes: MaxNbProcess,
			eventTimestampRaw:   time.Second,
			eventType:           model.DNSEventType,
			eventProcessPath:    "/bin/foo",
			eventDNSReq:         "foo0.bar",
		},
		{
			name:                "profile-at-max-size-dns/unstable",
			result:              ProfileAtMaxSize,
			newProfile:          false,
			containerCreatedAt:  time.Minute * -5,
			addFakeProcessNodes: MaxNbProcess,
			eventTimestampRaw:   time.Second,
			eventType:           model.DNSEventType,
			eventProcessPath:    "/bin/foo",
			eventDNSReq:         "foo1.bar",
		},
		{
			name:                "profile-at-max-size-dns/warmup-NOT-at-max-size",
			result:              WorkloadWarmup,
			newProfile:          true,
			containerCreatedAt:  0,
			addFakeProcessNodes: MaxNbProcess - 1,
			eventTimestampRaw:   time.Second,
			eventType:           model.DNSEventType,
			eventProcessPath:    "/bin/foo",
			eventDNSReq:         "foo2.bar",
		},
		{
			name:                "profile-at-max-size-dns/NOT-at-max-size",
			result:              AutoLearning,
			newProfile:          true,
			containerCreatedAt:  time.Minute * -5,
			addFakeProcessNodes: MaxNbProcess - 1,
			eventTimestampRaw:   time.Second,
			eventType:           model.DNSEventType,
			eventProcessPath:    "/bin/foo",
			eventDNSReq:         "foo2.bar",
		},

		// checking from max-size to stable, for exec:
		{
			name:                "profile-at-max-size-to-stable-exec/max-size",
			result:              ProfileAtMaxSize,
			newProfile:          true,
			containerCreatedAt:  time.Minute * -5,
			addFakeProcessNodes: MaxNbProcess,
			eventTimestampRaw:   time.Second,
			eventType:           model.ExecEventType,
			eventProcessPath:    "/bin/foo0",
		},
		{
			name:                "profile-at-max-size-to-stable-exec/stable",
			result:              StableEventType,
			newProfile:          false,
			containerCreatedAt:  time.Minute * -5,
			addFakeProcessNodes: MaxNbProcess,
			eventTimestampRaw:   AnomalyDetectionUnstableProfileTimeThreshold + time.Second,
			eventType:           model.ExecEventType,
			eventProcessPath:    "/bin/foo1",
		},
		{
			name:                "profile-at-max-size-to-stable-exec/meanwhile-dns-still-at-max-size",
			result:              ProfileAtMaxSize,
			newProfile:          false,
			containerCreatedAt:  time.Minute * -5,
			addFakeProcessNodes: 0,
			eventTimestampRaw:   time.Second,
			eventType:           model.DNSEventType,
			eventProcessPath:    "/bin/foo",
			eventDNSReq:         "foo0.bar",
		},
		// and for dns:
		{
			name:                "profile-at-max-size-to-stable-dns/max-size",
			result:              ProfileAtMaxSize,
			newProfile:          true,
			containerCreatedAt:  time.Minute * -5,
			addFakeProcessNodes: MaxNbProcess,
			eventTimestampRaw:   time.Second,
			eventType:           model.DNSEventType,
			eventProcessPath:    "/bin/foo",
			eventDNSReq:         "foo0.bar",
		},
		{
			name:                "profile-at-max-size-to-stable-dns/stable",
			result:              StableEventType,
			newProfile:          false,
			containerCreatedAt:  time.Minute * -5,
			addFakeProcessNodes: MaxNbProcess,
			eventTimestampRaw:   AnomalyDetectionUnstableProfileTimeThreshold + time.Second,
			eventType:           model.DNSEventType,
			eventProcessPath:    "/bin/foo",
			eventDNSReq:         "foo1.bar",
		},
		{
			name:                "profile-at-max-size-to-stable-dns/meanwhile-exec-still-at-max-size",
			result:              ProfileAtMaxSize,
			newProfile:          false,
			containerCreatedAt:  time.Minute * -5,
			addFakeProcessNodes: 0,
			eventTimestampRaw:   time.Second,
			eventType:           model.ExecEventType,
			eventProcessPath:    "/bin/foo1",
		},

		// from checking max-size to unstable for exec:
		{
			name:                "profile-at-max-size-to-unstable-exec/max-size",
			result:              ProfileAtMaxSize,
			newProfile:          true,
			containerCreatedAt:  time.Minute * -5,
			addFakeProcessNodes: MaxNbProcess,
			eventTimestampRaw:   time.Second,
			eventType:           model.ExecEventType,
			eventProcessPath:    "/bin/foo0",
		},
		{
			name:                "profile-at-max-size-to-unstable-exec/unstable",
			result:              ProfileAtMaxSize,
			newProfile:          false,
			containerCreatedAt:  time.Minute * -5,
			addFakeProcessNodes: 0,
			eventTimestampRaw:   0,
			eventType:           model.ExecEventType,
			eventProcessPath:    "/bin/foo_",
			loopUntil:           AnomalyDetectionUnstableProfileTimeThreshold - time.Second,
			loopIncrement:       AnomalyDetectionMinimumStablePeriod - time.Second,
		},
		{
			name:                "profile-at-max-size-to-unstable-exec/unstable",
			result:              UnstableEventType,
			newProfile:          false,
			containerCreatedAt:  time.Minute * -5,
			addFakeProcessNodes: 0,
			eventTimestampRaw:   AnomalyDetectionUnstableProfileTimeThreshold + time.Second,
			eventType:           model.ExecEventType,
			eventProcessPath:    "/bin/foo1",
		},
		{
			name:                "profile-at-max-size-to-unstable-exec/meanwhile-dns-still-at-max-size",
			result:              ProfileAtMaxSize,
			newProfile:          false,
			containerCreatedAt:  time.Minute * -5,
			addFakeProcessNodes: 0,
			eventTimestampRaw:   time.Second,
			eventType:           model.DNSEventType,
			eventProcessPath:    "/bin/foo",
			eventDNSReq:         "foo0.bar",
		},
		// and for dns:
		{
			name:                "profile-at-max-size-to-unstable-dns/max-size",
			result:              ProfileAtMaxSize,
			newProfile:          true,
			containerCreatedAt:  time.Minute * -5,
			addFakeProcessNodes: MaxNbProcess,
			eventTimestampRaw:   time.Second,
			eventType:           model.DNSEventType,
			eventProcessPath:    "/bin/foo0",
			eventDNSReq:         "foo0.bar",
		},
		{
			name:                "profile-at-max-size-to-unstable-dns/unstable",
			result:              ProfileAtMaxSize,
			newProfile:          false,
			containerCreatedAt:  time.Minute * -5,
			addFakeProcessNodes: 0,
			eventTimestampRaw:   0,
			eventType:           model.DNSEventType,
			eventProcessPath:    "/bin/foo",
			eventDNSReq:         ".foo.bar",
			loopUntil:           AnomalyDetectionUnstableProfileTimeThreshold - time.Second,
			loopIncrement:       AnomalyDetectionMinimumStablePeriod - time.Second,
		},
		{
			name:                "profile-at-max-size-to-unstable-dns/unstable",
			result:              UnstableEventType,
			newProfile:          false,
			containerCreatedAt:  time.Minute * -5,
			addFakeProcessNodes: 0,
			eventTimestampRaw:   AnomalyDetectionUnstableProfileTimeThreshold + time.Second,
			eventType:           model.DNSEventType,
			eventProcessPath:    "/bin/foo",
			eventDNSReq:         "foo1.bar",
		},
		{
			name:                "profile-at-max-size-to-unstable-dns/meanwhile-exec-still-at-max-size",
			result:              ProfileAtMaxSize,
			newProfile:          false,
			containerCreatedAt:  time.Minute * -5,
			addFakeProcessNodes: 0,
			eventTimestampRaw:   time.Second,
			eventType:           model.ExecEventType,
			eventProcessPath:    "/bin/foo1",
		},
	}

	// Initial time reference
	t0 := time.Now()

	// secprofile manager, only use for config and stats
	spm := &SecurityProfileManager{
		eventFiltering: make(map[eventFilteringEntry]*atomic.Uint64),
		config: &config.Config{
			RuntimeSecurity: &config.RuntimeSecurityConfig{
				AnomalyDetectionDefaultMinimumStablePeriod:   AnomalyDetectionMinimumStablePeriod,
				AnomalyDetectionWorkloadWarmupPeriod:         AnomalyDetectionWorkloadWarmupPeriod,
				AnomalyDetectionUnstableProfileTimeThreshold: AnomalyDetectionUnstableProfileTimeThreshold,
				AnomalyDetectionUnstableProfileSizeThreshold: AnomalyDetectionUnstableProfileSizeThreshold,
			},
		},
	}
	spm.initMetricsMap()

	var profile *SecurityProfile
	for _, ti := range tests {
		t.Run(ti.name, func(t *testing.T) {
			if ti.newProfile || profile == nil {
				profile = NewSecurityProfile(cgroupModel.WorkloadSelector{Image: "image", Tag: "tag"}, []model.EventType{model.ExecEventType, model.DNSEventType})
				profile.ActivityTree = activity_tree.NewActivityTree(profile, nil, "security_profile")
				profile.Instances = append(profile.Instances, &cgroupModel.CacheEntry{
					ContainerContext: model.ContainerContext{
						ID: defaultContainerID,
					},
					WorkloadSelector: cgroupModel.WorkloadSelector{Image: "image", Tag: "tag"},
				})
				profile.loadedNano = uint64(t0.UnixNano())
			}
			profile.ActivityTree.Stats.ProcessNodes += ti.addFakeProcessNodes

			if ti.loopUntil != 0 {
				currentIncrement := time.Duration(0)
				basePath := ti.eventProcessPath
				baseDNSReq := ti.eventDNSReq
				for currentIncrement < ti.loopUntil {
					if ti.eventType == model.ExecEventType {
						ti.eventProcessPath = basePath + fmt.Sprintf("%d", rand.Int())
					} else if ti.eventType == model.DNSEventType {
						ti.eventDNSReq = fmt.Sprintf("%d", rand.Int()) + baseDNSReq
					}
					ti.eventTimestampRaw = currentIncrement
					event := craftFakeEvent(t0, &ti, defaultContainerID)
					assert.Equal(t, ti.result, spm.tryAutolearn(profile, event))
					currentIncrement += ti.loopIncrement
				}
			} else { // only run once
				event := craftFakeEvent(t0, &ti, defaultContainerID)
				assert.Equal(t, ti.result, spm.tryAutolearn(profile, event))
			}

			// TODO: also check profile stats and global metrics
		})
	}
}
