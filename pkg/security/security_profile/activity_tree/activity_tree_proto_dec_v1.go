// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package activitytree holds activitytree related files
package activitytree

import (
	"time"

	adproto "github.com/DataDog/agent-payload/v5/cws/dumpsv1"

	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
)

// ProtoDecodeActivityTree decodes an ActivityTree structure
func ProtoDecodeActivityTree(dest *ActivityTree, nodes []*adproto.ProcessActivityNode) {
	for _, node := range nodes {
		dest.ProcessNodes = append(dest.ProcessNodes, protoDecodeProcessActivityNode(node))
	}
}

func protoDecodeProcessActivityNode(pan *adproto.ProcessActivityNode) *ProcessNode {
	if pan == nil {
		return nil
	}

	ppan := &ProcessNode{
		Process:        protoDecodeProcessNode(pan.Process),
		GenerationType: NodeGenerationType(pan.GenerationType),
		MatchedRules:   make([]*model.MatchedRule, 0, len(pan.MatchedRules)),
		Children:       make([]*ProcessNode, 0, len(pan.Children)),
		Files:          make(map[string]*FileNode, len(pan.Files)),
		DNSNames:       make(map[string]*DNSNode, len(pan.DnsNames)),
		Sockets:        make([]*SocketNode, 0, len(pan.Sockets)),
		Syscalls:       make([]int, 0, len(pan.Syscalls)),
	}

	for _, rule := range pan.MatchedRules {
		ppan.MatchedRules = append(ppan.MatchedRules, protoDecodeProtoMatchedRule(rule))
	}

	for _, child := range pan.Children {
		ppan.Children = append(ppan.Children, protoDecodeProcessActivityNode(child))
	}

	for _, fan := range pan.Files {
		protoDecodedFan := protoDecodeFileActivityNode(fan)
		ppan.Files[protoDecodedFan.Name] = protoDecodedFan
	}

	for _, dns := range pan.DnsNames {
		protoDecodedDNS := protoDecodeDNSNode(dns)
		if len(protoDecodedDNS.Requests) != 0 {
			name := protoDecodedDNS.Requests[0].Name
			ppan.DNSNames[name] = protoDecodedDNS
		}
	}

	for _, socket := range pan.Sockets {
		ppan.Sockets = append(ppan.Sockets, protoDecodeProtoSocket(socket))
	}

	for _, sysc := range pan.Syscalls {
		ppan.Syscalls = append(ppan.Syscalls, int(sysc))
	}

	return ppan
}

func protoDecodeProcessNode(p *adproto.ProcessInfo) model.Process {
	if p == nil {
		return model.Process{}
	}

	mp := model.Process{
		PIDContext: model.PIDContext{
			Pid: p.Pid,
			Tid: p.Tid,
		},
		PPid:        p.Ppid,
		Cookie:      p.Cookie64,
		IsThread:    p.IsThread,
		IsExecChild: p.IsExecChild,
		FileEvent:   *protoDecodeFileEvent(p.File),
		ContainerID: p.ContainerId,
		SpanID:      p.SpanId,
		TraceID:     p.TraceId,
		TTYName:     p.Tty,
		Comm:        p.Comm,

		ForkTime: ProtoDecodeTimestamp(p.ForkTime),
		ExitTime: ProtoDecodeTimestamp(p.ExitTime),
		ExecTime: ProtoDecodeTimestamp(p.ExecTime),

		Credentials: protoDecodeCredentials(p.Credentials),

		Argv:          make([]string, len(p.Args)),
		Argv0:         p.Argv0,
		ArgsTruncated: p.ArgsTruncated,

		Envs:          make([]string, len(p.Envs)),
		EnvsTruncated: p.EnvsTruncated,
	}

	copy(mp.Argv, p.Args)
	copy(mp.Envs, p.Envs)
	return mp
}

func protoDecodeCredentials(creds *adproto.Credentials) model.Credentials {
	if creds == nil {
		return model.Credentials{}
	}

	return model.Credentials{
		UID:          creds.Uid,
		GID:          creds.Gid,
		User:         creds.User,
		Group:        creds.Group,
		EUID:         creds.EffectiveUid,
		EGID:         creds.EffectiveGid,
		EUser:        creds.EffectiveUser,
		EGroup:       creds.EffectiveGroup,
		FSUID:        creds.FsUid,
		FSGID:        creds.FsGid,
		FSUser:       creds.FsUser,
		FSGroup:      creds.FsGroup,
		CapEffective: creds.CapEffective,
		CapPermitted: creds.CapPermitted,
	}
}

func protoDecodeFileEvent(fi *adproto.FileInfo) *model.FileEvent {
	if fi == nil {
		return nil
	}

	fe := &model.FileEvent{
		FileFields: model.FileFields{
			UID:   fi.Uid,
			User:  fi.User,
			GID:   fi.Gid,
			Group: fi.Group,
			Mode:  uint16(fi.Mode),
			CTime: fi.Ctime,
			MTime: fi.Mtime,
			PathKey: model.PathKey{
				MountID: fi.MountId,
				Inode:   fi.Inode,
			},
			InUpperLayer: fi.InUpperLayer,
		},
		PathnameStr:   fi.Path,
		BasenameStr:   fi.Basename,
		Filesystem:    fi.Filesystem,
		PkgName:       fi.PackageName,
		PkgVersion:    fi.PackageVersion,
		PkgSrcVersion: fi.PackageSrcversion,
		Hashes:        make([]string, len(fi.Hashes)),
		HashState:     model.HashState(fi.HashState),
	}
	copy(fe.Hashes, fi.Hashes)

	return fe
}

func protoDecodeFileActivityNode(fan *adproto.FileActivityNode) *FileNode {
	if fan == nil {
		return nil
	}

	pfan := &FileNode{
		MatchedRules:   make([]*model.MatchedRule, 0, len(fan.MatchedRules)),
		Name:           fan.Name,
		File:           protoDecodeFileEvent(fan.File),
		GenerationType: NodeGenerationType(fan.GenerationType),
		FirstSeen:      ProtoDecodeTimestamp(fan.FirstSeen),
		Open:           protoDecodeOpenNode(fan.Open),
		Children:       make(map[string]*FileNode, len(fan.Children)),
	}

	for _, rule := range fan.MatchedRules {
		pfan.MatchedRules = append(pfan.MatchedRules, protoDecodeProtoMatchedRule(rule))
	}

	for _, child := range fan.Children {
		node := protoDecodeFileActivityNode(child)
		pfan.Children[node.Name] = node
	}

	return pfan
}

func protoDecodeOpenNode(openNode *adproto.OpenNode) *OpenNode {
	if openNode == nil {
		return nil
	}

	pon := &OpenNode{
		SyscallEvent: model.SyscallEvent{
			Retval: openNode.Retval,
		},
		Flags: openNode.Flags,
		Mode:  openNode.Mode,
	}

	return pon
}

func protoDecodeDNSNode(dn *adproto.DNSNode) *DNSNode {
	if dn == nil {
		return nil
	}

	pdn := &DNSNode{
		MatchedRules: make([]*model.MatchedRule, 0, len(dn.MatchedRules)),
		Requests:     make([]model.DNSEvent, 0, len(dn.Requests)),
	}

	for _, rule := range dn.MatchedRules {
		pdn.MatchedRules = append(pdn.MatchedRules, protoDecodeProtoMatchedRule(rule))
	}

	for _, req := range dn.Requests {
		pdn.Requests = append(pdn.Requests, protoDecodeDNSInfo(req))
	}

	return pdn
}

func protoDecodeDNSInfo(ev *adproto.DNSInfo) model.DNSEvent {
	if ev == nil {
		return model.DNSEvent{}
	}

	return model.DNSEvent{
		Name:  ev.Name,
		Type:  uint16(ev.Type),
		Class: uint16(ev.Class),
		Size:  uint16(ev.Size),
		Count: uint16(ev.Count),
	}
}

func protoDecodeProtoSocket(sn *adproto.SocketNode) *SocketNode {
	if sn == nil {
		return nil
	}

	socketNode := &SocketNode{
		Family: sn.Family,
	}

	for _, bindNode := range sn.GetBind() {
		psn := &BindNode{
			MatchedRules: make([]*model.MatchedRule, 0, len(bindNode.MatchedRules)),
			Port:         uint16(bindNode.Port),
			IP:           bindNode.Ip,
		}

		for _, rule := range bindNode.MatchedRules {
			psn.MatchedRules = append(psn.MatchedRules, protoDecodeProtoMatchedRule(rule))
		}

		socketNode.Bind = append(socketNode.Bind, psn)
	}

	return socketNode
}

func protoDecodeProtoMatchedRule(r *adproto.MatchedRule) *model.MatchedRule {
	if r == nil {
		return nil
	}

	rule := &model.MatchedRule{
		RuleID:        r.RuleId,
		RuleVersion:   r.RuleVersion,
		RuleTags:      r.RuleTags,
		PolicyName:    r.PolicyName,
		PolicyVersion: r.PolicyVersion,
	}

	return rule
}

// ProtoDecodeTimestamp decodes a nanosecond representation of a timestamp
func ProtoDecodeTimestamp(nanos uint64) time.Time {
	return time.Unix(0, int64(nanos))
}
