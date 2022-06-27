// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux
// +build linux

package probe

import (
	"time"

	"github.com/DataDog/datadog-agent/pkg/security/adproto"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
)

func activityDumpToProto(ad *ActivityDump) *adproto.ActivityDump {
	if ad == nil {
		return nil
	}

	pad := adproto.ActivityDumpFromVTPool()
	*pad = adproto.ActivityDump{
		Host:    ad.Host,
		Service: ad.Service,
		Source:  ad.Source,
		Tags:    ad.Tags,
		Tree:    make([]*adproto.ProcessActivityNode, 0, len(ad.ProcessActivityTree)),
	}

	for _, tree := range ad.ProcessActivityTree {
		pad.Tree = append(pad.Tree, processActivityNodeToProto(tree))
	}

	return pad
}

func processActivityNodeToProto(pan *ProcessActivityNode) *adproto.ProcessActivityNode {
	if pan == nil {
		return nil
	}

	ppan := adproto.ProcessActivityNodeFromVTPool()
	*ppan = adproto.ProcessActivityNode{
		Process:        processNodeToProto(&pan.Process),
		GenerationType: string(pan.GenerationType),
		Files:          make([]*adproto.FileActivityNode, 0, len(pan.Files)),
		DNSNames:       make([]*adproto.DNSNode, 0, len(pan.DNSNames)),
		Children:       make([]*adproto.ProcessActivityNode, 0, len(pan.Children)),
	}

	for _, fan := range pan.Files {
		ppan.Files = append(ppan.Files, fileActivityNodeToProto(fan))
	}

	for _, dns := range pan.DNSNames {
		ppan.DNSNames = append(ppan.DNSNames, dnsNodeToProto(dns))
	}

	for _, child := range pan.Children {
		ppan.Children = append(ppan.Children, processActivityNodeToProto(child))
	}

	return ppan
}

func processNodeToProto(p *model.Process) *adproto.ProcessInfo {
	if p == nil {
		return nil
	}

	ppi := adproto.ProcessInfoFromVTPool()
	*ppi = adproto.ProcessInfo{
		Pid:         p.Pid,
		Tid:         p.Tid,
		Ppid:        p.PPid,
		Cookie:      p.Cookie,
		IsThread:    p.IsThread,
		File:        fileEventToProto(&p.FileEvent),
		ContainerID: p.ContainerID,
		SpanID:      p.SpanID,
		TraceID:     p.TraceID,
		TTY:         p.TTYName,
		Comm:        p.Comm,

		ForkTime: timestamp(&p.ForkTime),
		ExitTime: timestamp(&p.ExitTime),
		ExecTime: timestamp(&p.ExecTime),

		Credentials: credentialsToProto(&p.Credentials),

		Args:          p.ScrubbedArgv,
		Argv0:         p.Argv0,
		ArgsTruncated: p.ArgsTruncated,

		Envs:          p.Envs,
		EnvsTruncated: p.EnvsTruncated,
	}

	return ppi
}

func credentialsToProto(creds *model.Credentials) *adproto.Credentials {
	if creds == nil {
		return nil
	}

	pcreds := &adproto.Credentials{
		UID:            creds.UID,
		GID:            creds.GID,
		User:           creds.User,
		Group:          creds.Group,
		EffectiveUID:   creds.EUID,
		EffectiveGID:   creds.EGID,
		EffectiveUser:  creds.EUser,
		EffectiveGroup: creds.EGroup,
		FSUID:          creds.FSUID,
		FSGID:          creds.FSGID,
		FSUser:         creds.FSUser,
		FSGroup:        creds.FSGroup,
		CapEffective:   creds.CapEffective,
		CapPermitted:   creds.CapPermitted,
	}

	return pcreds
}

func fileEventToProto(fe *model.FileEvent) *adproto.FileInfo {
	if fe == nil {
		return nil
	}

	fi := adproto.FileInfoFromVTPool()
	*fi = adproto.FileInfo{
		UID:          fe.UID,
		User:         fe.User,
		GID:          fe.GID,
		Group:        fe.Group,
		Mode:         uint32(fe.Mode), // yeah sorry
		CTime:        fe.CTime,
		MTime:        fe.MTime,
		MountID:      fe.MountID,
		INode:        fe.Inode,
		InUpperLayer: fe.InUpperLayer,
		Path:         fe.PathnameStr,
		Basename:     fe.BasenameStr,
		FileSystem:   fe.Filesystem,
	}

	return fi
}

func fileActivityNodeToProto(fan *FileActivityNode) *adproto.FileActivityNode {
	if fan == nil {
		return nil
	}

	pfan := adproto.FileActivityNodeFromVTPool()
	*pfan = adproto.FileActivityNode{
		Name:           fan.Name,
		File:           fileEventToProto(fan.File),
		GenerationType: string(fan.GenerationType),
		FirstSeen:      timestamp(&fan.FirstSeen),
		Open:           openNodeToProto(fan.Open),
		Children:       make([]*adproto.FileActivityNode, 0, len(fan.Children)),
	}

	for _, child := range fan.Children {
		pfan.Children = append(pfan.Children, fileActivityNodeToProto(child))
	}

	return pfan
}

func openNodeToProto(openNode *OpenNode) *adproto.OpenNode {
	if openNode == nil {
		return nil
	}

	pon := &adproto.OpenNode{
		Retval: openNode.Retval,
		Flags:  openNode.Flags,
		Mode:   openNode.Mode,
	}

	return pon
}

func dnsNodeToProto(dn *DNSNode) *adproto.DNSNode {
	if dn == nil {
		return nil
	}

	pdn := &adproto.DNSNode{
		Requests: make([]*adproto.DNSInfo, 0, len(dn.requests)),
	}

	for _, req := range dn.requests {
		pdn.Requests = append(pdn.Requests, dnsEventToProto(&req))
	}

	return pdn
}

func dnsEventToProto(ev *model.DNSEvent) *adproto.DNSInfo {
	if ev == nil {
		return nil
	}

	return &adproto.DNSInfo{
		Name:  ev.Name,
		Type:  uint32(ev.Type),
		Class: uint32(ev.Class),
		Size:  uint32(ev.Size),
		Count: uint32(ev.Count),
	}
}

func timestamp(t *time.Time) uint64 {
	return uint64(t.Unix())
}
