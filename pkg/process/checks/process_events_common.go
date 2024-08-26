// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package checks

import (
	payload "github.com/DataDog/agent-payload/v5/process"

	"github.com/DataDog/datadog-agent/pkg/process/events/model"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// FmtProcessEvents formats process lifecyle events to be sent in an agent payload
func FmtProcessEvents(events []*model.ProcessEvent) []*payload.ProcessEvent {
	payloadEvents := make([]*payload.ProcessEvent, 0, len(events))

	for _, e := range events {
		pE := &payload.ProcessEvent{
			CollectionTime: e.CollectionTime.UnixNano(),
			Pid:            e.Pid,
			ContainerId:    e.ContainerID,
			Command: &payload.Command{
				Exe:  e.Exe,
				Args: e.Cmdline,
				Ppid: int32(e.Ppid),
			},
			User: &payload.ProcessUser{
				Name: e.Username,
				Uid:  int32(e.UID),
				Gid:  int32(e.GID),
			},
		}

		switch e.EventType {
		case model.Exec:
			pE.Type = payload.ProcEventType_exec
			exec := &payload.ProcessExec{
				ForkTime: e.ForkTime.UnixNano(),
				ExecTime: e.ExecTime.UnixNano(),
			}
			pE.TypedEvent = &payload.ProcessEvent_Exec{Exec: exec}
		case model.Exit:
			pE.Type = payload.ProcEventType_exit
			exit := &payload.ProcessExit{
				ExecTime: e.ExecTime.UnixNano(),
				ExitTime: e.ExitTime.UnixNano(),
				ExitCode: int32(e.ExitCode),
			}
			pE.TypedEvent = &payload.ProcessEvent_Exit{Exit: exit}
		default:
			log.Error("Unexpected event type, dropping it")
			continue
		}

		payloadEvents = append(payloadEvents, pE)
	}

	return payloadEvents
}
