// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package remoteflags

import (
	"runtime"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/config/model"
)

const testKey = "feature.enabled"

// layeredSetter is a ConfigSetter that models one value per source and
// resolves the effective value by source priority, like the real configuration.
type layeredSetter struct {
	mu     sync.Mutex
	layers map[model.Source]bool
	sets   []layeredCall
	unsets []layeredCall
}

type layeredCall struct {
	value  any
	source model.Source
}

func newLayeredSetter() *layeredSetter {
	return &layeredSetter{layers: map[model.Source]bool{}}
}

func (l *layeredSetter) Set(_ string, value any, source model.Source) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.layers[source] = value.(bool)
	l.sets = append(l.sets, layeredCall{value, source})
}

func (l *layeredSetter) UnsetForSource(_ string, source model.Source) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.layers, source)
	l.unsets = append(l.unsets, layeredCall{nil, source})
}

func (l *layeredSetter) GetSource(string) model.Source {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.topLocked()
}

func (l *layeredSetter) topLocked() model.Source {
	best, bestIdx := model.SourceDefault, -1
	for src := range l.layers {
		if idx := slices.Index(model.Sources, src); idx > bestIdx {
			best, bestIdx = src, idx
		}
	}
	return best
}

// effective returns the winning value, false when no layer is set.
func (l *layeredSetter) effective() bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	if len(l.layers) == 0 {
		return false
	}
	return l.layers[l.topLocked()]
}

func (l *layeredSetter) hasLayer(source model.Source) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	_, ok := l.layers[source]
	return ok
}

func (l *layeredSetter) wroteFalse() bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return slices.ContainsFunc(l.sets, func(c layeredCall) bool { return c.value == false })
}

func TestConfigKeyHandlerPrecedence(t *testing.T) {
	// Local layers a user or the agent may populate before the flag arrives.
	localSources := []model.Source{
		model.SourceFile,
		model.SourceEnvVar,
		model.SourceFleetPolicies,
		model.SourceAgentRuntime,
	}

	// A local value must be left alone by every flag transition.
	transitions := map[string]func(t *testing.T, h FlagHandler){
		"flag true":    func(t *testing.T, h FlagHandler) { require.NoError(t, h.OnChange(true)) },
		"flag false":   func(t *testing.T, h FlagHandler) { require.NoError(t, h.OnChange(false)) },
		"flag removed": func(_ *testing.T, h FlagHandler) { h.OnNoConfig() },
		"recover":      func(_ *testing.T, h FlagHandler) { h.SafeRecover(assert.AnError, true) },
	}
	for _, src := range localSources {
		for _, localValue := range []bool{true, false} {
			for name, apply := range transitions {
				t.Run(string(src)+"/local="+boolName(localValue)+"/"+name, func(t *testing.T) {
					s := newLayeredSetter()
					s.Set(testKey, localValue, src)
					s.sets = nil
					apply(t, NewConfigKeyHandler("f", s, testKey, nil))
					assert.Equal(t, localValue, s.effective(), "local value must keep winning")
					assert.False(t, s.hasLayer(model.SourceRC), "no remote layer may be written over a local value")
					assert.False(t, s.wroteFalse())
				})
			}
		}
	}

	t.Run("no local value/flag true turns the key on", func(t *testing.T) {
		s := newLayeredSetter()
		require.NoError(t, NewConfigKeyHandler("f", s, testKey, nil).OnChange(true))
		assert.True(t, s.effective())
		assert.Equal(t, model.SourceRC, s.GetSource(testKey))
	})

	t.Run("no local value/flag true twice stays on", func(t *testing.T) {
		s := newLayeredSetter()
		h := NewConfigKeyHandler("f", s, testKey, nil)
		require.NoError(t, h.OnChange(true))
		require.NoError(t, h.OnChange(true))
		assert.True(t, s.effective())
	})

	releases := map[string]func(t *testing.T, h FlagHandler){
		"flag false":   func(t *testing.T, h FlagHandler) { require.NoError(t, h.OnChange(false)) },
		"flag removed": func(_ *testing.T, h FlagHandler) { h.OnNoConfig() },
		"recover":      func(_ *testing.T, h FlagHandler) { h.SafeRecover(assert.AnError, true) },
	}
	for name, release := range releases {
		t.Run("flag true then "+name+" gives the layer back", func(t *testing.T) {
			s := newLayeredSetter()
			h := NewConfigKeyHandler("f", s, testKey, nil)
			require.NoError(t, h.OnChange(true))
			release(t, h)
			assert.False(t, s.hasLayer(model.SourceRC))
			assert.False(t, s.effective())
			assert.False(t, s.wroteFalse(), "false must never be written")
			assert.Equal(t, []layeredCall{{nil, model.SourceRC}}, s.unsets)
		})
	}

	t.Run("local true then flag false keeps local true", func(t *testing.T) {
		s := newLayeredSetter()
		h := NewConfigKeyHandler("f", s, testKey, nil)
		require.NoError(t, h.OnChange(true))
		s.Set(testKey, true, model.SourceFile) // user turns it on while the flag is on
		require.NoError(t, h.OnChange(false))
		assert.True(t, s.effective())
		assert.False(t, s.wroteFalse())
	})

	t.Run("flag false then local true keeps local true", func(t *testing.T) {
		s := newLayeredSetter()
		h := NewConfigKeyHandler("f", s, testKey, nil)
		require.NoError(t, h.OnChange(false))
		s.Set(testKey, true, model.SourceFile)
		assert.True(t, s.effective())
		assert.False(t, s.wroteFalse())
	})

	t.Run("local false set after the flag turned the key on loses to the remote layer", func(t *testing.T) {
		s := newLayeredSetter()
		h := NewConfigKeyHandler("f", s, testKey, nil)
		require.NoError(t, h.OnChange(true))
		s.Set(testKey, false, model.SourceFile)
		assert.True(t, s.effective(), "documented limitation: the remote layer outranks file values")
	})

	t.Run("recovered flag stays off when the client does not renotify", func(t *testing.T) {
		s := newLayeredSetter()
		h := NewConfigKeyHandler("f", s, testKey, nil)
		require.NoError(t, h.OnChange(true))
		h.SafeRecover(assert.AnError, true)
		s.mu.Lock()
		s.sets = nil
		s.mu.Unlock()
		// The client deduplicates repeated values, so no OnChange follows.
		assert.False(t, s.effective())
		assert.Empty(t, s.sets)
	})
}

func boolName(b bool) string {
	if b {
		return "true"
	}
	return "false"
}

func TestConfigKeyHandlerIdentityAndHealth(t *testing.T) {
	s := newLayeredSetter()
	h := NewConfigKeyHandler("my_flag", s, testKey, nil)
	assert.Equal(t, FlagName("my_flag"), h.FlagName())
	assert.True(t, h.IsHealthy(), "nil health function means healthy")

	ok := true
	h = NewConfigKeyHandler("my_flag", s, testKey, func() bool { return ok })
	assert.True(t, h.IsHealthy())
	ok = false
	assert.False(t, h.IsHealthy())
}

func TestSubscribers(t *testing.T) {
	s := newLayeredSetter()
	h1 := NewConfigKeyHandler("a", s, testKey, nil)
	h2 := NewConfigKeyHandler("b", s, testKey, nil)
	got := Subscribers(h1, h2).Handlers()
	assert.Equal(t, []FlagHandler{h1, h2}, got)
	assert.Empty(t, Subscribers().Handlers())
}

func TestConfigKeyHandlerWithClient(t *testing.T) {
	s := newLayeredSetter()
	c := NewClient().WithConfigSetter(s)
	defer c.Stop()
	for _, h := range Subscribers(NewConfigKeyHandler("f", s, testKey, nil)).Handlers() {
		require.NoError(t, c.SubscribeWithHandler(h))
	}
}

// gatedSetter blocks GetSource until the gate is closed, to hold a handler call
// between its read and its write.
type gatedSetter struct {
	*layeredSetter
	reading chan struct{}
	gate    chan struct{}
}

func (g *gatedSetter) GetSource(key string) model.Source {
	src := g.layeredSetter.GetSource(key)
	select {
	case g.reading <- struct{}{}:
		<-g.gate
	default:
	}
	return src
}

// releaseBlockedOnLock reports whether a goroutine is parked acquiring a mutex
// inside the handler's OnNoConfig. testing/synctest cannot be used here because
// a goroutine waiting on a sync.Mutex is not durably blocked, so synctest.Wait
// would never return while the handler holds its lock.
func releaseBlockedOnLock() bool {
	buf := make([]byte, 1<<20)
	buf = buf[:runtime.Stack(buf, true)]
	for _, g := range strings.Split(string(buf), "\n\n") {
		if strings.Contains(g, "configKeyHandler).OnNoConfig") &&
			(strings.Contains(g, "sync.(*Mutex).Lock") || strings.Contains(g, "sync.runtime_SemacquireMutex")) {
			return true
		}
	}
	return false
}

func TestConfigKeyHandlerSerializesReadAndWrite(t *testing.T) {
	s := &gatedSetter{
		layeredSetter: newLayeredSetter(),
		reading:       make(chan struct{}, 1),
		gate:          make(chan struct{}),
	}
	h := NewConfigKeyHandler("f", s, testKey, nil)

	enableDone := make(chan struct{})
	go func() {
		defer close(enableDone)
		_ = h.OnChange(true)
	}()
	<-s.reading // the enable has read the source and is paused before writing

	releaseDone := make(chan struct{})
	go func() {
		defer close(releaseDone)
		h.OnNoConfig()
	}()

	// The release must end up parked on the handler's lock while the enable is
	// paused. If it finishes instead, the lock does not serialize the two calls.
	deadline := time.Now().Add(10 * time.Second)
	for !releaseBlockedOnLock() {
		select {
		case <-releaseDone:
			close(s.gate)
			<-enableDone
			t.Fatal("the release ran between the read and the write of the enable")
		default:
		}
		if time.Now().After(deadline) {
			close(s.gate)
			<-enableDone
			<-releaseDone
			t.Fatal("the release goroutine never blocked on the handler lock")
		}
		time.Sleep(time.Millisecond)
	}

	close(s.gate)
	<-enableDone
	<-releaseDone

	assert.False(t, s.hasLayer(model.SourceRC), "the release issued last must win over the paused enable")
}
