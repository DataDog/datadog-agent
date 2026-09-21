// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package foldspace

import "time"

// Core is the sans-I/O foldspace client the driver consumes.
//
// The contract forbids two callers inside one region at once. The regions are
// ingest, one per sender, and notification draining. A panic rather than an
// error, because the violation is memory-safe and silently protocol-breaking.
type Core interface {
	Start() Progress
	HasCapacity() bool
	PushLog(record Record, nowNanos uint64, metadataID uint64) (Admission, Progress)
	Flush() (Admission, Progress)
	BeginShutdown() (Admission, Progress)
	Abandon() Progress
	IsDrained() bool
	PollSender(sender SenderID) []Effect
	HandleStreamOpened(sender SenderID, stream StreamID) Progress
	HandleAck(sender SenderID, stream StreamID, batchID uint32, status int32) Progress
	HandleStreamError(sender SenderID, stream StreamID, message string) Progress
	HandleTimer(sender SenderID, stream StreamID, timer TimerKind) Progress
	PollNotifications() []Notification
	TakeClockAdvance() time.Duration
	Close()
	SenderCount() int
	SenderClass(sender SenderID) SenderClass
}
