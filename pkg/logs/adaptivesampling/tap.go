// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package adaptivesampling

import "sync/atomic"

// TokenizedLogEvent is emitted by the Logs Agent after tokenizer execution and
// before adaptive sampling.
type TokenizedLogEvent struct {
	Content            string
	Status             string
	Tags               []string
	Hostname           string
	TimestampUnixMilli int64
	ContainerID        string
	Pattern            string
	PatternHash        string
}

// TokenizedLogObserver receives tokenized log observations from the Logs Agent.
type TokenizedLogObserver interface {
	ObserveTokenizedLog(TokenizedLogEvent)
}

type tokenizedLogObserverHolder struct {
	observer TokenizedLogObserver
}

var tokenizedLogObserver atomic.Pointer[tokenizedLogObserverHolder]

// SetTokenizedLogObserver installs a process-wide tokenized log observer and
// returns a cleanup function that restores the previous observer.
func SetTokenizedLogObserver(observer TokenizedLogObserver) func() {
	holder := &tokenizedLogObserverHolder{observer: observer}
	previous := tokenizedLogObserver.Swap(holder)
	return func() {
		tokenizedLogObserver.CompareAndSwap(holder, previous)
	}
}

// HasTokenizedLogObserver reports whether ObserveTokenizedLog has a receiver.
func HasTokenizedLogObserver() bool {
	holder := tokenizedLogObserver.Load()
	return holder != nil && holder.observer != nil
}

// ObserveTokenizedLog forwards event to the registered observer, if any.
func ObserveTokenizedLog(event TokenizedLogEvent) {
	holder := tokenizedLogObserver.Load()
	if holder == nil || holder.observer == nil {
		return
	}
	holder.observer.ObserveTokenizedLog(event)
}
