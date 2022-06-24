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

func activityDumpToProto(ad *ActivityDump) *adproto.ActivityDump {
	if ad == nil {
		return nil
	}

	pad := adproto.ActivityDumpFromVTPool()

	pad.Host = ad.Host
	pad.Service = ad.Service
	pad.Source = ad.Source
	pad.Tags = ad.Tags
	pad.Tree = make([]*adproto.ProcessActivityNode, 0, len(ad.ProcessActivityTree))

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

	ppan.Process = processNodeToProto(&pan.Process)
	ppan.GenerationType = string(pan.GenerationType)
	ppan.Files = make([]*adproto.FileActivityNode, 0, len(pan.Files))
	ppan.DNSNames = make([]*adproto.DNSNode, 0, len(pan.DNSNames))
	ppan.Children = make([]*adproto.ProcessActivityNode, 0, len(pan.Children))

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

	ppi.Pid = p.Pid
	ppi.Tid = p.Tid
	ppi.Ppid = p.PPid
	ppi.Cookie = p.Cookie
	ppi.IsThread = p.IsThread
	ppi.File = fileEventToProto(&p.FileEvent)
	ppi.ContainerID = p.ContainerID
	ppi.SpanID = p.SpanID
	ppi.TraceID = p.TraceID
	ppi.TTY = p.TTYName
	ppi.Comm = p.Comm

	ppi.ForkTime = timestamppb.New(p.ForkTime)
	ppi.ExitTime = timestamppb.New(p.ExitTime)
	ppi.ExecTime = timestamppb.New(p.ExecTime)

	ppi.Credentials = credentialsToProto(&p.Credentials)

	ppi.Args = p.ScrubbedArgv
	ppi.Argv0 = p.Argv0
	ppi.ArgsTruncated = p.ArgsTruncated

	ppi.Envs = p.Envs
	ppi.EnvsTruncated = p.EnvsTruncated

	return ppi
}

func credentialsToProto(creds *model.Credentials) *adproto.Credentials {
	if creds == nil {
		return nil
	}

	pcreds := &adproto.Credentials{}
	pcreds.UID = creds.UID
	pcreds.GID = creds.GID
	pcreds.User = creds.User
	pcreds.Group = creds.Group
	pcreds.EffectiveUID = creds.EUID
	pcreds.EffectiveGID = creds.EGID
	pcreds.EffectiveUser = creds.EUser
	pcreds.EffectiveGroup = creds.EGroup
	pcreds.FSUID = creds.FSUID
	pcreds.FSGID = creds.FSGID
	pcreds.FSUser = creds.FSUser
	pcreds.FSGroup = creds.FSGroup
	pcreds.CapEffective = creds.CapEffective
	pcreds.CapPermitted = creds.CapPermitted
	return pcreds
}

func fileEventToProto(fe *model.FileEvent) *adproto.FileInfo {
	if fe == nil {
		return nil
	}

	fi := adproto.FileInfoFromVTPool()

	fi.UID = fe.UID
	fi.User = fe.User
	fi.GID = fe.GID
	fi.Group = fe.Group
	fi.Mode = uint32(fe.Mode)   // yeah sorry
	fi.CTime = uint32(fe.CTime) // TODO: discuss this
	fi.MTime = uint32(fe.MTime)
	fi.MountID = fe.MountID
	fi.INode = fe.Inode
	fi.InUpperLayer = fe.InUpperLayer
	fi.Path = fe.PathnameStr
	fi.Basename = fe.BasenameStr
	fi.FileSystem = fe.Filesystem
	return fi
}

func fileActivityNodeToProto(fan *FileActivityNode) *adproto.FileActivityNode {
	if fan == nil {
		return nil
	}

	pfan := adproto.FileActivityNodeFromVTPool()

	pfan.Name = fan.Name
	pfan.File = fileEventToProto(fan.File)
	pfan.GenerationType = string(fan.GenerationType)
	pfan.FirstSeen = timestamppb.New(fan.FirstSeen)
	pfan.Open = openNodeToProto(fan.Open)
	pfan.Children = make([]*adproto.FileActivityNode, 0, len(fan.Children))

	for _, child := range fan.Children {
		pfan.Children = append(pfan.Children, fileActivityNodeToProto(child))
	}

	return pfan
}

func openNodeToProto(openNode *OpenNode) *adproto.OpenNode {
	if openNode == nil {
		return nil
	}

	pon := &adproto.OpenNode{}
	pon.Retval = openNode.Retval
	pon.Flags = openNode.Flags
	pon.Mode = openNode.Mode
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
