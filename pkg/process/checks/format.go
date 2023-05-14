// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package checks

import (
	_ "embed"
	"errors"
	"fmt"
	"io"
	"math"
	"sort"
	"strconv"
	"text/template"
	"time"

	model "github.com/DataDog/agent-payload/v5/process"

	"github.com/dustin/go-humanize"
)

var (
	// ErrNoHumanFormat is thrown when a check without human-readable support is passed to the HumanFormat method
	ErrNoHumanFormat = errors.New("no implementation of human-readable output for this check")

	// ErrUnexpectedMessageType is thrown when message type is incompatible with check
	ErrUnexpectedMessageType = errors.New("unexpected message type")

	//go:embed templates/host.tmpl
	hostTemplate string

	//go:embed templates/processes.tmpl
	processesTemplate string

	//go:embed templates/rtprocesses.tmpl
	rtProcessesTemplate string

	//go:embed templates/containers.tmpl
	containersTemplate string

	//go:embed templates/rtcontainers.tmpl
	rtContainersTemplate string

	//go:embed templates/discovery.tmpl
	discoveryTemplate string

	//go:embed templates/events.tmpl
	eventsTemplate string

	fnMap = template.FuncMap{
		"humanize":  humanize.Commaf,
		"bytes":     humanize.Bytes,
		"timeMilli": func(v int64) string { return time.UnixMilli(v).UTC().Format(time.RFC3339) },
		"timeNano":  func(v int64) string { return time.Unix(0, v).UTC().Format(time.RFC3339) },
		"time":      func(v int64) string { return time.Unix(v, 0).UTC().Format(time.RFC3339) },
		"cpupct":    func(v float32) string { return humanize.FtoaWithDigits(math.Round(float64(v)*100)/100, 2) + "%" },
		"io": func(v float32) string {
			if v < 0 {
				return "-"
			}
			return humanize.Ftoa(float64(v))
		},
	}
)

// HumanFormat takes the messages produced by a check run and outputs them in a human-readable format
func HumanFormat(check string, msgs []model.MessageBody, w io.Writer) error {
	switch check {
	case ProcessCheckName:
		return humanFormatProcess(msgs, w)
	case RTProcessCheckName:
		return humanFormatRealTimeProcess(msgs, w)
	case ContainerCheckName:
		return humanFormatContainer(msgs, w)
	case RTContainerCheckName:
		return humanFormatRealTimeContainer(msgs, w)
	case DiscoveryCheckName:
		return humanFormatProcessDiscovery(msgs, w)
	case ProcessEventsCheckName:
		return HumanFormatProcessEvents(msgs, w, true)
	}
	return ErrNoHumanFormat
}

func humanFormatProcess(msgs []model.MessageBody, w io.Writer) error {
	var data struct {
		Processes  []*model.Process
		Containers []*model.Container
		Hostname   string
		CPUCount   int
		Memory     uint64
	}

	var (
		processes  = map[int32]*model.Process{}
		containers = map[string]*model.Container{}
		pids       []int
	)

	for _, m := range msgs {
		proc, ok := m.(*model.CollectorProc)
		if !ok {
			return ErrUnexpectedMessageType
		}
		data.Hostname = proc.HostName
		data.CPUCount = len(proc.Info.Cpus)
		data.Memory = uint64(proc.Info.TotalMemory)
		for _, p := range proc.Processes {
			processes[p.Pid] = p
			pids = append(pids, int(p.Pid))
		}

		for _, c := range proc.Containers {
			containers[c.Id] = c
		}
	}

	containerIDs := make([]string, 0, len(containers))
	for cid := range containers {
		containerIDs = append(containerIDs, cid)
	}

	pidsSorted := sort.IntSlice(pids)
	pidsSorted.Sort()
	data.Processes = make([]*model.Process, 0, len(pidsSorted))
	for _, pid := range pidsSorted {
		data.Processes = append(data.Processes, processes[int32(pid)])
	}

	containerIDsSorted := sort.StringSlice(containerIDs)
	containerIDsSorted.Sort()
	data.Containers = make([]*model.Container, 0, len(containerIDsSorted))
	for _, cid := range containerIDsSorted {
		data.Containers = append(data.Containers, containers[cid])
	}

	return renderTemplates(
		w,
		data,
		hostTemplate,
		processesTemplate,
		containersTemplate,
	)
}

func humanFormatRealTimeProcess(msgs []model.MessageBody, w io.Writer) error {
	var data struct {
		ProcessStats   []*model.ProcessStat
		ContainerStats []*model.ContainerStat
		Hostname       string
		CPUCount       int
		Memory         uint64
	}

	var (
		processStats   = map[int32]*model.ProcessStat{}
		containerStats = map[string]*model.ContainerStat{}
		pids           []int
		containerIDs   []string
	)

	for _, m := range msgs {
		proc, ok := m.(*model.CollectorRealTime)
		if !ok {
			return ErrUnexpectedMessageType
		}
		data.Hostname = proc.HostName
		data.Memory = uint64(proc.TotalMemory)
		data.CPUCount = int(proc.NumCpus)
		for _, p := range proc.Stats {
			processStats[p.Pid] = p
			pids = append(pids, int(p.Pid))
		}

		for _, c := range proc.ContainerStats {
			containerStats[c.Id] = c
		}
	}

	for cid := range containerStats {
		containerIDs = append(containerIDs, cid)
	}

	pidsSorted := sort.IntSlice(pids)
	pidsSorted.Sort()
	for _, pid := range pidsSorted {
		data.ProcessStats = append(data.ProcessStats, processStats[int32(pid)])
	}

	containerIDsSorted := sort.StringSlice(containerIDs)
	containerIDsSorted.Sort()
	for _, cid := range containerIDsSorted {
		data.ContainerStats = append(data.ContainerStats, containerStats[cid])
	}

	return renderTemplates(
		w,
		data,
		hostTemplate,
		rtProcessesTemplate,
		rtContainersTemplate,
	)
}

func humanFormatContainer(msgs []model.MessageBody, w io.Writer) error {
	var data struct {
		Containers []*model.Container
		Hostname   string
		CPUCount   int
		Memory     uint64
	}

	var (
		containers   = map[string]*model.Container{}
		containerIDs []string
	)

	for _, m := range msgs {
		cont, ok := m.(*model.CollectorContainer)
		if !ok {
			return ErrUnexpectedMessageType
		}
		data.Hostname = cont.HostName
		data.CPUCount = len(cont.Info.Cpus)
		data.Memory = uint64(cont.Info.TotalMemory)
		for _, c := range cont.Containers {
			containers[c.Id] = c
		}
	}

	for cid := range containers {
		containerIDs = append(containerIDs, cid)
	}

	containerIDsSorted := sort.StringSlice(containerIDs)
	containerIDsSorted.Sort()
	for _, cid := range containerIDsSorted {
		data.Containers = append(data.Containers, containers[cid])
	}

	return renderTemplates(
		w,
		data,
		hostTemplate,
		containersTemplate,
	)
}

func humanFormatRealTimeContainer(msgs []model.MessageBody, w io.Writer) error {
	var data struct {
		ContainerStats []*model.ContainerStat
		Hostname       string
		CPUCount       int
		Memory         uint64
	}

	var (
		stats        = map[string]*model.ContainerStat{}
		containerIDs []string
	)

	for _, m := range msgs {
		cont, ok := m.(*model.CollectorContainerRealTime)
		if !ok {
			return ErrUnexpectedMessageType
		}
		data.Hostname = cont.HostName
		data.CPUCount = int(cont.NumCpus)
		data.Memory = uint64(cont.TotalMemory)
		for _, c := range cont.Stats {
			stats[c.Id] = c
		}
	}

	for cid := range stats {
		containerIDs = append(containerIDs, cid)
	}

	containerIDsSorted := sort.StringSlice(containerIDs)
	containerIDsSorted.Sort()
	for _, cid := range containerIDsSorted {
		data.ContainerStats = append(data.ContainerStats, stats[cid])
	}

	return renderTemplates(
		w,
		data,
		hostTemplate,
		rtContainersTemplate,
	)
}

func humanFormatProcessDiscovery(msgs []model.MessageBody, w io.Writer) error {
	var data struct {
		Discoveries []*model.ProcessDiscovery
		Hostname    string
	}

	var (
		discoveries = map[int32]*model.ProcessDiscovery{}
		pids        []int
	)

	for _, m := range msgs {
		proc, ok := m.(*model.CollectorProcDiscovery)
		if !ok {
			return ErrUnexpectedMessageType
		}
		data.Hostname = proc.HostName
		for _, d := range proc.ProcessDiscoveries {
			discoveries[d.Pid] = d
			pids = append(pids, int(d.Pid))
		}
	}

	pidsSorted := sort.IntSlice(pids)
	pidsSorted.Sort()
	for _, pid := range pidsSorted {
		data.Discoveries = append(data.Discoveries, discoveries[int32(pid)])
	}

	return renderTemplates(
		w,
		data,
		discoveryTemplate,
	)
}

// HumanFormatProcessEvents takes the messages produced by a process_events run and outputs them in a human-readable format
func HumanFormatProcessEvents(msgs []model.MessageBody, w io.Writer, checkOutput bool) error {
	// ProcessEvent's TypedEvent is an interface
	// As text/template does not cast an interface to the underlying concrete type, we need to perform the cast before
	// rendering the template
	type extendedEvent struct {
		*model.ProcessEvent
		Exec *model.ProcessExec
		Exit *model.ProcessExit
	}

	var data struct {
		CheckOutput bool
		Hostname    string
		Events      []*extendedEvent
	}

	data.CheckOutput = checkOutput
	for _, m := range msgs {
		evtMsg, ok := m.(*model.CollectorProcEvent)
		if !ok {
			return ErrUnexpectedMessageType
		}
		data.Hostname = evtMsg.Hostname

		for _, e := range evtMsg.Events {
			extended := &extendedEvent{ProcessEvent: e}
			switch typedEvent := e.TypedEvent.(type) {
			case *model.ProcessEvent_Exec:
				extended.Exec = typedEvent.Exec
			case *model.ProcessEvent_Exit:
				extended.Exit = typedEvent.Exit
			}
			data.Events = append(data.Events, extended)
		}
	}
	return renderTemplates(
		w,
		data,
		eventsTemplate,
	)
}

func renderTemplates(w io.Writer, data interface{}, templates ...string) error {
	for idx, name := range templates {
		t := template.Must(template.New("tmpl-" + strconv.Itoa(idx)).Funcs(fnMap).Parse(name))
		err := t.Execute(w, data)
		if err != nil {
			return err
		}

		fmt.Fprintln(w, "")
	}
	return nil
}
