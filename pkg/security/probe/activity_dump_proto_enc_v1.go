// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux
// +build linux

package probe

import (
	"time"

	adproto "github.com/DataDog/datadog-agent/pkg/security/adproto/v1"
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

		Metadata: adMetadataToProto(&ad.DumpMetadata),

		Tags: make([]string, len(ad.Tags)),
		Tree: make([]*adproto.ProcessActivityNode, 0, len(ad.ProcessActivityTree)),
	}

	copy(pad.Tags, ad.Tags)

	for _, tree := range ad.ProcessActivityTree {
		pad.Tree = append(pad.Tree, processActivityNodeToProto(tree))
	}

	return pad
}

func adMetadataToProto(meta *DumpMetadata) *adproto.Metadata {
	if meta == nil {
		return nil
	}

	pmeta := &adproto.Metadata{
		AgentVersion:      meta.AgentVersion,
		AgentCommit:       meta.AgentCommit,
		KernelVersion:     meta.KernelVersion,
		LinuxDistribution: meta.LinuxDistribution,

		Name:              meta.Name,
		ProtobufVersion:   meta.ProtobufVersion,
		DifferentiateArgs: meta.DifferentiateArgs,
		Comm:              meta.Comm,
		ContainerId:       meta.ContainerID,
		Start:             timestampToProto(&meta.Start),
		End:               timestampToProto(&meta.End),
		Size:              meta.Size,
		Arch:              meta.Arch,
		Serialization:     meta.Serialization,
	}

	return pmeta
}

func processActivityNodeToProto(pan *ProcessActivityNode) *adproto.ProcessActivityNode {
	if pan == nil {
		return nil
	}

	ppan := adproto.ProcessActivityNodeFromVTPool()
	*ppan = adproto.ProcessActivityNode{
		Process:        processNodeToProto(&pan.Process),
		GenerationType: adproto.GenerationType(pan.GenerationType),
		Children:       make([]*adproto.ProcessActivityNode, 0, len(pan.Children)),
		Files:          make([]*adproto.FileActivityNode, 0, len(pan.Files)),
		DnsNames:       make([]*adproto.DNSNode, 0, len(pan.DNSNames)),
		Sockets:        make([]*adproto.SocketNode, 0, len(pan.Sockets)),
		Syscalls:       make([]uint32, 0, len(pan.Syscalls)),
	}

	for _, child := range pan.Children {
		ppan.Children = append(ppan.Children, processActivityNodeToProto(child))
	}

	for _, fan := range pan.Files {
		ppan.Files = append(ppan.Files, fileActivityNodeToProto(fan))
	}

	for _, dns := range pan.DNSNames {
		ppan.DnsNames = append(ppan.DnsNames, dnsNodeToProto(dns))
	}

	for _, socket := range pan.Sockets {
		ppan.Sockets = append(ppan.Sockets, socketNodeToProto(socket))
	}

	for _, sysc := range pan.Syscalls {
		ppan.Syscalls = append(ppan.Syscalls, uint32(sysc))
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
		ContainerId: p.ContainerID,
		SpanId:      p.SpanID,
		TraceId:     p.TraceID,
		Tty:         p.TTYName,
		Comm:        p.Comm,

		ForkTime: timestampToProto(&p.ForkTime),
		ExitTime: timestampToProto(&p.ExitTime),
		ExecTime: timestampToProto(&p.ExecTime),

		Credentials: credentialsToProto(&p.Credentials),

		Args:          make([]string, len(p.ScrubbedArgv)),
		Argv0:         p.Argv0,
		ArgsTruncated: p.ArgsTruncated,

		Envs:          make([]string, len(p.Envs)),
		EnvsTruncated: p.EnvsTruncated,
	}

	copy(ppi.Args, p.ScrubbedArgv)
	copy(ppi.Envs, p.Envs)

	return ppi
}

func credentialsToProto(creds *model.Credentials) *adproto.Credentials {
	if creds == nil {
		return nil
	}

	pcreds := &adproto.Credentials{
		Uid:            creds.UID,
		Gid:            creds.GID,
		User:           creds.User,
		Group:          creds.Group,
		EffectiveUid:   creds.EUID,
		EffectiveGid:   creds.EGID,
		EffectiveUser:  creds.EUser,
		EffectiveGroup: creds.EGroup,
		FsUid:          creds.FSUID,
		FsGid:          creds.FSGID,
		FsUser:         creds.FSUser,
		FsGroup:        creds.FSGroup,
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
		Uid:            fe.UID,
		User:           fe.User,
		Gid:            fe.GID,
		Group:          fe.Group,
		Mode:           uint32(fe.Mode), // yeah sorry
		Ctime:          fe.CTime,
		Mtime:          fe.MTime,
		MountId:        fe.MountID,
		Inode:          fe.Inode,
		InUpperLayer:   fe.InUpperLayer,
		Path:           fe.PathnameStr,
		Basename:       fe.BasenameStr,
		Filesystem:     fe.Filesystem,
		PackageName:    fe.PkgName,
		PackageVersion: fe.PkgVersion,
		PackageMajor:   int32(fe.PkgMajor),
		PackageMinor:   int32(fe.PkgMinor),
		PackagePatch:   int32(fe.PkgPatch),
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
		GenerationType: adproto.GenerationType(fan.GenerationType),
		FirstSeen:      timestampToProto(&fan.FirstSeen),
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
		Requests: make([]*adproto.DNSInfo, 0, len(dn.Requests)),
	}

	for _, req := range dn.Requests {
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

func socketNodeToProto(sn *SocketNode) *adproto.SocketNode {
	if sn == nil {
		return nil
	}

	psn := &adproto.SocketNode{
		Family: sn.Family,
		Bind:   make([]*adproto.BindNode, 0, len(sn.Bind)),
	}

	for _, bn := range sn.Bind {
		psn.Bind = append(psn.Bind, &adproto.BindNode{
			Port: uint32(bn.Port),
			Ip:   bn.IP,
		})
	}

	return psn
}

func timestampToProto(t *time.Time) uint64 {
	if t.IsZero() {
		return 0
	}
	return uint64(t.UnixNano())
}
