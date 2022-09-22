// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package orchestrator

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestNewLambdaOrchestrator(t *testing.T) {
	orchestrator := NewLambdaOrchestrator()
	lambdaOrchestrator, ok := orchestrator.(*lambdaOrchestrator)
	if !ok {
		t.Fatal("should be a lambdaOrchestrator")
	}
	assert.False(t, lambdaOrchestrator.hasHitHelloRoute.Load())
	assert.False(t, lambdaOrchestrator.hasHitFlushRoute.Load())
	assert.False(t, lambdaOrchestrator.hasHitEndInvocation.Load())
	assert.False(t, lambdaOrchestrator.hasHitTimeout.Load())
	assert.False(t, lambdaOrchestrator.hasReceivedRuntimeDone.Load())
	assert.False(t, lambdaOrchestrator.hasInvocationStarted.Load())
}

func TestHitHelloRoute(t *testing.T) {
	orchestrator := NewLambdaOrchestrator()
	lambdaOrchestrator, ok := orchestrator.(*lambdaOrchestrator)
	if !ok {
		t.Fatal("should be a lambdaOrchestrator")
	}
	orchestrator.HitHelloRoute()
	assert.True(t, lambdaOrchestrator.hasHitHelloRoute.Load())
}

func TestHitFlushRoute(t *testing.T) {
	orchestrator := NewLambdaOrchestrator()
	lambdaOrchestrator, ok := orchestrator.(*lambdaOrchestrator)
	if !ok {
		t.Fatal("should be a lambdaOrchestrator")
	}
	orchestrator.HitFlushRoute()
	assert.True(t, lambdaOrchestrator.hasHitFlushRoute.Load())
}

func TestHitEndInvocation(t *testing.T) {
	orchestrator := NewLambdaOrchestrator()
	lambdaOrchestrator, ok := orchestrator.(*lambdaOrchestrator)
	if !ok {
		t.Fatal("should be a lambdaOrchestrator")
	}
	orchestrator.HitEndInvocation()
	assert.True(t, lambdaOrchestrator.hasHitEndInvocation.Load())
}

func TestHitTimeout(t *testing.T) {
	orchestrator := NewLambdaOrchestrator()
	lambdaOrchestrator, ok := orchestrator.(*lambdaOrchestrator)
	if !ok {
		t.Fatal("should be a lambdaOrchestrator")
	}
	orchestrator.HitTimeout()
	assert.True(t, lambdaOrchestrator.hasHitTimeout.Load())
}

func TestRuntimeDoneReceived(t *testing.T) {
	orchestrator := NewLambdaOrchestrator()
	lambdaOrchestrator, ok := orchestrator.(*lambdaOrchestrator)
	if !ok {
		t.Fatal("should be a lambdaOrchestrator")
	}
	orchestrator.RuntimeDoneReceived()
	assert.True(t, lambdaOrchestrator.hasReceivedRuntimeDone.Load())
}

func TestHasHitInvokeEvent(t *testing.T) {
	orchestrator := NewLambdaOrchestrator()
	lambdaOrchestrator, ok := orchestrator.(*lambdaOrchestrator)
	if !ok {
		t.Fatal("should be a lambdaOrchestrator")
	}
	orchestrator.HitInvokeEvent()
	assert.True(t, lambdaOrchestrator.hasInvocationStarted.Load())
}

func TestReset(t *testing.T) {
	orchestrator := NewLambdaOrchestrator()
	lambdaOrchestrator, ok := orchestrator.(*lambdaOrchestrator)
	if !ok {
		t.Fatal("should be a lambdaOrchestrator")
	}
	orchestrator.HitHelloRoute()
	orchestrator.HitFlushRoute()
	orchestrator.HitEndInvocation()
	orchestrator.HitTimeout()
	orchestrator.HitInvokeEvent()
	orchestrator.Reset()
	assert.False(t, lambdaOrchestrator.hasHitHelloRoute.Load())
	assert.False(t, lambdaOrchestrator.hasHitFlushRoute.Load())
	assert.False(t, lambdaOrchestrator.hasHitEndInvocation.Load())
	assert.False(t, lambdaOrchestrator.hasHitTimeout.Load())
	assert.False(t, lambdaOrchestrator.hasReceivedRuntimeDone.Load())
	assert.False(t, lambdaOrchestrator.hasInvocationStarted.Load())
}

func TestWaitForExtensionLifecycle(t *testing.T) {
	timeout := time.After(100 * time.Millisecond)
	orchestrator := NewLambdaOrchestrator()
	waitChan := make(chan bool)
	go func() {
		orchestrator.WaitForExtensionLifecycle()
		waitChan <- true
	}()
	select {
	case <-timeout:
		// nothing to do here
	case <-waitChan:
		t.Fatal("Should not be done")
	}
}

func TestWaitForExtensionLifecycleTimeout(t *testing.T) {
	timeout := time.After(100 * time.Millisecond)
	orchestrator := NewLambdaOrchestrator()
	waitChan := make(chan bool)
	go func() {
		orchestrator.WaitForExtensionLifecycle()
		waitChan <- true
	}()
	orchestrator.HitTimeout()
	select {
	case <-timeout:
		t.Fatal("Should not wait forever")
	case <-waitChan:
		// nothing to do here
	}
}

func TestWaitForExtensionLifecycleHitFlush(t *testing.T) {
	timeout := time.After(100 * time.Millisecond)
	orchestrator := NewLambdaOrchestrator()
	lambdaOrchestrator, ok := orchestrator.(*lambdaOrchestrator)
	if !ok {
		t.Fatal("should be a lambdaOrchestrator")
	}
	waitChan := make(chan bool)
	go func() {
		orchestrator.WaitForExtensionLifecycle()
		waitChan <- true
	}()
	orchestrator.HitFlushRoute()
	select {
	case <-timeout:
		t.Fatal("Should not wait forever")
	case <-waitChan:
		// nothing to do here
	}
	// test reset
	assert.False(t, lambdaOrchestrator.hasHitFlushRoute.Load())
}

func TestWaitForExtensionLifecycleHitEndInvocation(t *testing.T) {
	timeout := time.After(100 * time.Millisecond)
	orchestrator := NewLambdaOrchestrator()
	lambdaOrchestrator, ok := orchestrator.(*lambdaOrchestrator)
	if !ok {
		t.Fatal("should be a lambdaOrchestrator")
	}
	waitChan := make(chan bool)
	go func() {
		orchestrator.WaitForExtensionLifecycle()
		waitChan <- true
	}()
	orchestrator.HitEndInvocation()
	select {
	case <-timeout:
		t.Fatal("Should not wait forever")
	case <-waitChan:
		// nothing to do here
	}
	// test reset
	assert.False(t, lambdaOrchestrator.hasHitEndInvocation.Load())
}

func TestIsFlushPossibleFalse(t *testing.T) {
	orchestrator := NewLambdaOrchestrator()
	assert.False(t, orchestrator.IsFlushPossible())
}

func TestIsFlushPossibleTrueWithRuntimeDone(t *testing.T) {
	orchestrator := NewLambdaOrchestrator()
	orchestrator.RuntimeDoneReceived()
	assert.True(t, orchestrator.IsFlushPossible())
}

func TestIsFlushPossibleTrueWithFlushRoute(t *testing.T) {
	orchestrator := NewLambdaOrchestrator()
	orchestrator.HitFlushRoute()
	assert.True(t, orchestrator.IsFlushPossible())
}

func TestIsFlushPossibleTrueWithHitEndInvocation(t *testing.T) {
	orchestrator := NewLambdaOrchestrator()
	orchestrator.HitEndInvocation()
	assert.True(t, orchestrator.IsFlushPossible())
}

func CanAcceptTracesBlocked(t *testing.T) {
	timeout := time.After(100 * time.Millisecond)
	orchestrator := NewLambdaOrchestrator()
	waitChan := make(chan bool)
	go func() {
		orchestrator.CanAcceptTraces()
		waitChan <- true
	}()
	select {
	case <-timeout:
		// nothing to do here
	case <-waitChan:
		t.Fatal("Should wait forever")
	}
}

func CanAcceptTracesNotBlocked(t *testing.T) {
	timeout := time.After(100 * time.Millisecond)
	orchestrator := NewLambdaOrchestrator()
	waitChan := make(chan bool)
	go func() {
		orchestrator.CanAcceptTraces()
		waitChan <- true
	}()
	orchestrator.HitInvokeEvent()
	select {
	case <-timeout:
		t.Fatal("Should not wait forever")
	case <-waitChan:
		// nothing to do here
	}
}
