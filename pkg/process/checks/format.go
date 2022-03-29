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
	"sort"
	"strconv"
	"text/template"

	model "github.com/DataDog/agent-payload/v5/process"
	"github.com/DataDog/datadog-agent/pkg/process/config"

	"github.com/dustin/go-humanize"
)

var (
	// ErrNoHumanFormat is thrown when a check without human-readable support is passed to the HumanFormat method
	ErrNoHumanFormat = errors.New("no implementation of human-readable output for this check")

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

	fnMap = template.FuncMap{
		"humanize":        humanize.Commaf,
		"percent":         func(v float64) string { return fmt.Sprintf("%02.1f", v*100) },
		"containerHealth": func(v model.ContainerHealth) string { return v.String() },
		"containerState":  func(v model.ContainerState) string { return v.String() },
	}
)

// HumanFormat takes the messages produced by a check run and outputs them in a human-readable format
func HumanFormat(check string, msgs []model.MessageBody, w io.Writer) error {
	switch check {
	case config.ProcessCheckName:
		return humanFormatProcess(msgs, w)
	case config.RTProcessCheckName:
		return humanFormatRealTimeProcess(msgs, w)
	case config.ContainerCheckName:
		return humanFormatContainer(msgs, w)
	case config.RTContainerCheckName:
		return humanFormatRealTimeContainer(msgs, w)
	case config.DiscoveryCheckName:
		return humanFormatProcessDiscovery(msgs, w)
	}
	return ErrNoHumanFormat
}

func humanFormatProcess(msgs []model.MessageBody, w io.Writer) error {
	var data struct {
		Processes  []*model.Process
		Containers []*model.Container
	}

	var (
		processes    = map[int32]*model.Process{}
		containers   = map[string]*model.Container{}
		pids         []int
		containerIDs []string
	)

	for _, m := range msgs {
		proc := m.(*model.CollectorProc)
		for _, p := range proc.Processes {
			processes[p.Pid] = p
			pids = append(pids, int(p.Pid))
		}

		for _, c := range proc.Containers {
			containers[c.Id] = c
		}
	}

	for cid := range containers {
		containerIDs = append(containerIDs, cid)
	}

	pidsSorted := sort.IntSlice(pids)
	pidsSorted.Sort()
	for _, pid := range pidsSorted {
		data.Processes = append(data.Processes, processes[int32(pid)])
	}

	containerIDsSorted := sort.StringSlice(containerIDs)
	containerIDsSorted.Sort()
	for _, cid := range containerIDsSorted {
		data.Containers = append(data.Containers, containers[cid])
	}

	templates := []string{
		processesTemplate,
		containersTemplate,
	}

	for idx, name := range templates {
		t := template.Must(template.New("process-" + strconv.Itoa(idx)).Funcs(fnMap).Parse(name))
		err := t.Execute(w, data)
		if err != nil {
			return err
		}
		w.Write(([]byte)("\n"))
	}
	return nil
}

func humanFormatRealTimeProcess(msgs []model.MessageBody, w io.Writer) error {
	var data struct {
		ProcessStats   []*model.ProcessStat
		ContainerStats []*model.ContainerStat
	}

	var (
		processStats   = map[int32]*model.ProcessStat{}
		containerStats = map[string]*model.ContainerStat{}
		pids           []int
		containerIDs   []string
	)

	for _, m := range msgs {
		proc := m.(*model.CollectorRealTime)
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

	templates := []string{
		rtProcessesTemplate,
		rtContainersTemplate,
	}

	for idx, name := range templates {
		t := template.Must(template.New("process-" + strconv.Itoa(idx)).Funcs(fnMap).Parse(name))
		err := t.Execute(w, data)
		if err != nil {
			return err
		}
		w.Write(([]byte)("\n"))
	}
	return nil
}

func humanFormatContainer(msgs []model.MessageBody, w io.Writer) error {
	var data struct {
		Containers []*model.Container
	}

	var (
		containers   = map[string]*model.Container{}
		containerIDs []string
	)

	for _, m := range msgs {
		cont := m.(*model.CollectorContainer)
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

	t := template.Must(template.New("container").Funcs(fnMap).Parse(containersTemplate))
	return t.Execute(w, data)
}

func humanFormatRealTimeContainer(msgs []model.MessageBody, w io.Writer) error {
	var data struct {
		ContainerStats []*model.ContainerStat
	}

	var (
		stats        = map[string]*model.ContainerStat{}
		containerIDs []string
	)

	for _, m := range msgs {
		cont := m.(*model.CollectorContainerRealTime)
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

	t := template.Must(template.New("rtcontainer").Funcs(fnMap).Parse(rtContainersTemplate))
	return t.Execute(w, data)
}

func humanFormatProcessDiscovery(msgs []model.MessageBody, w io.Writer) error {
	var data struct {
		Discoveries []*model.ProcessDiscovery
	}

	var (
		discoveries = map[int32]*model.ProcessDiscovery{}
		pids        []int
	)

	for _, m := range msgs {
		proc := m.(*model.CollectorProcDiscovery)
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

	t := template.Must(template.New("discovery").Funcs(fnMap).Parse(discoveryTemplate))
	return t.Execute(w, data)
}
