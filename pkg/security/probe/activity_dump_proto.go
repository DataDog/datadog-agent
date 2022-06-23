// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux
// +build linux

package probe

import (
	"github.com/DataDog/datadog-agent/pkg/security/adproto"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func adToProto(ad *ActivityDump) *adproto.ActivityDump {
	pad := &adproto.ActivityDump{
		Host:    ad.Host,
		Service: ad.Service,
		Source:  ad.Source,
		Tags:    ad.Tags,
		Tree:    make([]*adproto.ProcessActivityNode, 0, len(ad.ProcessActivityTree)),
	}

	for _, tree := range ad.ProcessActivityTree {
		pad.Tree = append(pad.Tree, panToProto(tree))
	}

	return pad
}

func panToProto(pan *ProcessActivityNode) *adproto.ProcessActivityNode {
	if pan == nil {
		return nil
	}

	ppan := &adproto.ProcessActivityNode{
		Process:        processNodeToProto(&pan.Process),
		GenerationType: string(pan.GenerationType),
		Files:          make([]*adproto.FileActivityNode, 0, len(pan.Files)),
		DNSNames:       make([]*adproto.DNSNode, 0, len(pan.DNSNames)),
		Children:       make([]*adproto.ProcessActivityNode, 0, len(pan.Children)),
	}

	for _, fan := range pan.Files {
		ppan.Files = append(ppan.Files, fanToProto(fan))
	}

	for _, dns := range pan.DNSNames {
		ppan.DNSNames = append(ppan.DNSNames, dnsNodeToProto(dns))
	}

	for _, child := range pan.Children {
		ppan.Children = append(ppan.Children, panToProto(child))
	}

	return ppan
}

func processNodeToProto(p *model.Process) *adproto.ProcessInfo {
	if p == nil {
		return nil
	}

	return &adproto.ProcessInfo{
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

		ForkTime: timestamppb.New(p.ForkTime),
		ExitTime: timestamppb.New(p.ExitTime),
		ExecTime: timestamppb.New(p.ExecTime),

		Credentials: credentialsToProto(&p.Credentials),

		Args:          p.ScrubbedArgv,
		Argv0:         p.Argv0,
		ArgsTruncated: p.ArgsTruncated,

		Envs:          p.Envs,
		EnvsTruncated: p.EnvsTruncated,
	}
}

func credentialsToProto(creds *model.Credentials) *adproto.Credentials {
	if creds == nil {
		return nil
	}

	return &adproto.Credentials{
		UID:   creds.UID,
		GID:   creds.GID,
		User:  creds.User,
		Group: creds.Group,

		EffectiveUID:   creds.EUID,
		EffectiveGID:   creds.EGID,
		EffectiveUser:  creds.EUser,
		EffectiveGroup: creds.EGroup,

		FSUID:   creds.FSUID,
		FSGID:   creds.FSGID,
		FSUser:  creds.FSUser,
		FSGroup: creds.FSGroup,

		CapEffective: creds.CapEffective,
		CapPermitted: creds.CapPermitted,
	}
}

func fileEventToProto(fe *model.FileEvent) *adproto.FileInfo {
	if fe == nil {
		return nil
	}

	return &adproto.FileInfo{
		UID:          fe.UID,
		User:         fe.User,
		GID:          fe.GID,
		Group:        fe.Group,
		Mode:         uint32(fe.Mode),  // yeah sorry
		CTime:        uint32(fe.CTime), // TODO: discuss this
		MTime:        uint32(fe.MTime),
		MountID:      fe.MountID,
		INode:        fe.Inode,
		InUpperLayer: fe.InUpperLayer,

		Path:       fe.PathnameStr,
		Basename:   fe.BasenameStr,
		FileSystem: fe.Filesystem,
	}
}

func fanToProto(fan *FileActivityNode) *adproto.FileActivityNode {
	if fan == nil {
		return nil
	}

	pfan := &adproto.FileActivityNode{
		Name:           fan.Name,
		File:           fileEventToProto(fan.File),
		GenerationType: string(fan.GenerationType),
		FirstSeen:      timestamppb.New(fan.FirstSeen),
		Open:           openNodeToProto(fan.Open),

		Children: make([]*adproto.FileActivityNode, 0, len(fan.Children)),
	}

	for _, child := range fan.Children {
		pfan.Children = append(pfan.Children, fanToProto(child))
	}

	return pfan
}

func openNodeToProto(openNode *OpenNode) *adproto.OpenNode {
	if openNode == nil {
		return nil
	}

	return &adproto.OpenNode{
		Retval: openNode.Retval,
		Flags:  openNode.Flags,
		Mode:   openNode.Mode,
	}
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
