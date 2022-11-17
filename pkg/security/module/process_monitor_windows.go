// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package module

import (
	"strings"

	"github.com/DataDog/datadog-agent/pkg/process/events/model"
	sprobe "github.com/DataDog/datadog-agent/pkg/security/probe"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// HandleEvent implement the EventHandler interface
func (p *ProcessMonitoring) HandleEvent(event *sprobe.Event) {

	cmdline := strings.Split(event.WPT.CmdLine, " ")

	e := &model.ProcessEvent{
		EventType:      model.NewEventType(event.GetEventType().String()),
		CollectionTime: event.Timestamp,
		Pid:            uint32(event.WPT.Pid),
		ContainerID:    "",
		Ppid:           0,
		UID:            0,
		GID:            0,
		Username:       "",
		Group:          "",
		Exe:            event.WPT.ImageFile, //entry.FileEvent.PathnameStr, // FileEvent is not a pointer, so it can be directly accessed
		Cmdline:        cmdline,
		//ForkTime:       entry.ForkTime,
		//ExecTime:       entry.ExecTime,
		//ExitTime:       entry.ExitTime,
		//ExitCode:       event.Exit.Code,
	}
	log.Infof("Sending process event %v", e)
	data, err := e.MarshalMsg(nil)
	if err != nil {
		log.Error("Failed to marshal Process Lifecycle Event: ", err)
		return
	}

	p.module.apiServer.SendProcessEvent(data)
}
