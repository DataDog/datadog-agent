// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2020 Datadog, Inc.

package traps

import (
	"math/rand"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestServerEmpty(t *testing.T) {
	b := newBuilder(t)
	b.Configure()
	s := b.StartServer()
	s.Stop()
}

func TestServerV2(t *testing.T) {
	b := newBuilder(t)
	config := b.Add(TrapListenerConfig{CommunityStrings: []string{"public"}})
	b.Configure()

	s := b.StartServer()
	defer s.Stop()

	sendTestV2Trap(t, config, "public")
	p := receivePacket(t, s)
	require.NotNil(t, p)
	assertV2(t, p, config)
	assertV2Variables(t, p)
}

func TestServerV2BadCredentials(t *testing.T) {
	b := newBuilder(t)
	config := b.Add(TrapListenerConfig{CommunityStrings: []string{"public"}})
	b.Configure()

	s := b.StartServer()
	defer s.Stop()

	sendTestV2Trap(t, config, "wrong")
	assertNoPacketReceived(t, s)
}

func TestConcurrency(t *testing.T) {
	b := newBuilder(t)
	configs := []TrapListenerConfig{
		b.Add(TrapListenerConfig{CommunityStrings: []string{"public0"}}),
		b.Add(TrapListenerConfig{CommunityStrings: []string{"public1"}}),
		b.Add(TrapListenerConfig{CommunityStrings: []string{"public2"}}),
	}
	b.Configure()

	s := b.StartServer()
	defer s.Stop()

	numMessagesPerListener := 100
	totalMessages := numMessagesPerListener * len(configs)

	wg := sync.WaitGroup{}
	wg.Add(len(configs) + 1)

	for _, config := range configs {
		c := config
		go func() {
			defer wg.Done()
			for i := 0; i < numMessagesPerListener; i++ {
				time.Sleep(time.Duration(rand.Float64()) * time.Microsecond) // Prevent serial execution.
				sendTestV2Trap(t, c, c.CommunityStrings[0])
			}
		}()
	}

	go func() {
		defer wg.Done()
		for i := 0; i < totalMessages; i++ {
			p := receivePacket(t, s)
			require.NotNil(t, p)
			assertV2Variables(t, p)
		}
	}()

	wg.Wait()
}

func TestPortConflict(t *testing.T) {
	b := newBuilder(t)
	port := b.GetPort()

	// Triggers an "address already in use" error for one of the listeners.
	b.Add(TrapListenerConfig{Port: port, CommunityStrings: []string{"public0"}})
	b.Add(TrapListenerConfig{Port: port, CommunityStrings: []string{"public1"}})
	b.Configure()

	s, err := NewTrapServer()
	require.Error(t, err)
	assert.Nil(t, s)
}
