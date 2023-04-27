// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux
// +build linux

package dump

import (
	"time"

	"golang.org/x/text/runes"

	adproto "github.com/DataDog/agent-payload/v5/cws/dumpsv1"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
)

func activityDumpToProto(ad *ActivityDump) *adproto.SecDump {
	if ad == nil {
		return nil
	}

	pad := adproto.SecDumpFromVTPool()
	*pad = adproto.SecDump{
		Host:    ad.Host,
		Service: ad.Service,
		Source:  ad.Source,

		Metadata: adMetadataToProto(&ad.Metadata),

		Tags: make([]string, len(ad.Tags)),
		Tree: make([]*adproto.ProcessActivityNode, 0, len(ad.ProcessActivityTree)),
	}

	copy(pad.Tags, ad.Tags)

	for _, tree := range ad.ProcessActivityTree {
		pad.Tree = append(pad.Tree, processActivityNodeToProto(tree))
	}

	return pad
}

func adMetadataToProto(meta *Metadata) *adproto.Metadata {
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
		MatchedRules:   make([]*adproto.MatchedRule, 0, len(pan.MatchedRules)),
		Children:       make([]*adproto.ProcessActivityNode, 0, len(pan.Children)),
		Files:          make([]*adproto.FileActivityNode, 0, len(pan.Files)),
		DnsNames:       make([]*adproto.DNSNode, 0, len(pan.DNSNames)),
		Sockets:        make([]*adproto.SocketNode, 0, len(pan.Sockets)),
		Syscalls:       make([]uint32, 0, len(pan.Syscalls)),
	}

	for _, rule := range pan.MatchedRules {
		ppan.MatchedRules = append(ppan.MatchedRules, matchedRuleToProto(rule))
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
		Tty:         escape(p.TTYName),
		Comm:        escape(p.Comm),

		ForkTime: timestampToProto(&p.ForkTime),
		ExitTime: timestampToProto(&p.ExitTime),
		ExecTime: timestampToProto(&p.ExecTime),

		Credentials: credentialsToProto(&p.Credentials),

		Args:          copyAndEscape(p.ScrubbedArgv),
		Argv0:         escape(p.Argv0),
		ArgsTruncated: p.ArgsTruncated,

		Envs:          copyAndEscape(p.Envs),
		EnvsTruncated: p.EnvsTruncated,
	}

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
		Uid:               fe.UID,
		User:              fe.User,
		Gid:               fe.GID,
		Group:             fe.Group,
		Mode:              uint32(fe.Mode), // yeah sorry
		Ctime:             fe.CTime,
		Mtime:             fe.MTime,
		MountId:           fe.MountID,
		Inode:             fe.Inode,
		InUpperLayer:      fe.InUpperLayer,
		Path:              escape(fe.PathnameStr),
		Basename:          escape(fe.BasenameStr),
		Filesystem:        escape(fe.Filesystem),
		PackageName:       fe.PkgName,
		PackageVersion:    fe.PkgVersion,
		PackageSrcversion: fe.PkgSrcVersion,
	}

	return fi
}

func fileActivityNodeToProto(fan *FileActivityNode) *adproto.FileActivityNode {
	if fan == nil {
		return nil
	}

	pfan := adproto.FileActivityNodeFromVTPool()
	*pfan = adproto.FileActivityNode{
		MatchedRules:   make([]*adproto.MatchedRule, 0, len(fan.MatchedRules)),
		Name:           escape(fan.Name),
		File:           fileEventToProto(fan.File),
		GenerationType: adproto.GenerationType(fan.GenerationType),
		FirstSeen:      timestampToProto(&fan.FirstSeen),
		Open:           openNodeToProto(fan.Open),
		Children:       make([]*adproto.FileActivityNode, 0, len(fan.Children)),
	}

	for _, rule := range fan.MatchedRules {
		pfan.MatchedRules = append(pfan.MatchedRules, matchedRuleToProto(rule))
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
		MatchedRules: make([]*adproto.MatchedRule, 0, len(dn.MatchedRules)),
		Requests:     make([]*adproto.DNSInfo, 0, len(dn.Requests)),
	}

	for _, rule := range dn.MatchedRules {
		pdn.MatchedRules = append(pdn.MatchedRules, matchedRuleToProto(rule))
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
		Name:  escape(ev.Name),
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
		pbn := &adproto.BindNode{
			MatchedRules: make([]*adproto.MatchedRule, 0, len(bn.MatchedRules)),
			Port:         uint32(bn.Port),
			Ip:           bn.IP,
		}

		for _, rule := range bn.MatchedRules {
			pbn.MatchedRules = append(pbn.MatchedRules, matchedRuleToProto(rule))
		}

		psn.Bind = append(psn.Bind, pbn)
	}

	return psn
}

func matchedRuleToProto(r *model.MatchedRule) *adproto.MatchedRule {
	if r == nil {
		return nil
	}

	pmr := &adproto.MatchedRule{
		RuleId:        r.RuleID,
		RuleVersion:   r.RuleVersion,
		RuleTags:      r.RuleTags,
		PolicyName:    r.PolicyName,
		PolicyVersion: r.PolicyVersion,
	}

	return pmr
}

func timestampToProto(t *time.Time) uint64 {
	if t.IsZero() {
		return 0
	}
	return uint64(t.UnixNano())
}

func copyAndEscape(in []string) []string {
	out := make([]string, 0, len(in))
	transformer := runes.ReplaceIllFormed()
	for _, value := range in {
		out = append(out, transformer.String(value))
	}
	return out
}

func escape(in string) string {
	transformer := runes.ReplaceIllFormed()
	return transformer.String(in)
}
