// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build ignore
// +build ignore

package ebpf

/*
   #include "./c/process-types.h"
*/
import "C"

type ProcessEventType C.event_type
type ProcessKEvent C.kevent_t
type ProcessExecEvent C.exec_event_t
type ProcessExitEvent C.exit_event_t
type ProcessContext C.process_context_t
type ProcessProcCache C.proc_cache_t
type ProcessPidCache C.pid_cache_t
type ProcessContainerContext C.container_context_t

const (
	ProcessEventTypeAny  ProcessEventType = C.EVENT_ANY
	ProcessEventTypeFork ProcessEventType = C.EVENT_FORK
	ProcessEventTypeExec ProcessEventType = C.EVENT_EXEC
	ProcessEventTypeExit ProcessEventType = C.EVENT_EXIT
)
