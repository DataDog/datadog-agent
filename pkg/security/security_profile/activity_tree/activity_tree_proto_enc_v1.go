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
	"golang.org/x/text/runes"

	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
)

// ToProto encodes an activity tree to its protobuf representation
func ToProto(at *ActivityTree) []*adproto.ProcessActivityNode {
	out := make([]*adproto.ProcessActivityNode, 0, len(at.ProcessNodes))

	for _, node := range at.ProcessNodes {
		out = append(out, processActivityNodeToProto(node))
	}
	return out
}

func processActivityNodeToProto(pan *ProcessNode) *adproto.ProcessActivityNode {
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
		ImdsEvents:     make([]*adproto.IMDSNode, 0, len(pan.IMDSEvents)),
		Sockets:        make([]*adproto.SocketNode, 0, len(pan.Sockets)),
		NodeBase:       nodeBaseToProto(&pan.NodeBase),
		SyscallNodes:   make([]*adproto.SyscallNode, 0, len(pan.Syscalls)),
		NetworkDevices: make([]*adproto.NetworkDeviceNode, 0, len(pan.NetworkDevices)),
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

	for _, imds := range pan.IMDSEvents {
		ppan.ImdsEvents = append(ppan.ImdsEvents, imdsNodeToProto(imds))
	}

	for _, socket := range pan.Sockets {
		ppan.Sockets = append(ppan.Sockets, socketNodeToProto(socket))
	}

	for _, sysc := range pan.Syscalls {
		ppan.SyscallNodes = append(ppan.SyscallNodes, syscallNodeToProto(sysc))
	}

	for _, networkDevice := range pan.NetworkDevices {
		ppan.NetworkDevices = append(ppan.NetworkDevices, networkDeviceToProto(networkDevice))
	}

	return ppan
}

func networkDeviceToProto(device *NetworkDeviceNode) *adproto.NetworkDeviceNode {
	if device == nil {
		return nil
	}

	ndn := &adproto.NetworkDeviceNode{
		MatchedRules: make([]*adproto.MatchedRule, 0, len(device.MatchedRules)),
		Netns:        device.Context.NetNS,
		Ifindex:      device.Context.IfIndex,
		Ifname:       device.Context.IfName,
		FlowNodes:    make([]*adproto.FlowNode, 0, len(device.FlowNodes)),
	}

	for _, rule := range device.MatchedRules {
		ndn.MatchedRules = append(ndn.MatchedRules, matchedRuleToProto(rule))
	}

	for _, flowNode := range device.FlowNodes {
		ndn.FlowNodes = append(ndn.FlowNodes, flowNodeToProto(flowNode.Flow, &flowNode.NodeBase))
	}

	return ndn
}

func flowNodeToProto(flow model.Flow, nodeBase *NodeBase) *adproto.FlowNode {
	return &adproto.FlowNode{
		NodeBase:    nodeBaseToProto(nodeBase),
		L3Protocol:  uint32(flow.L3Protocol),
		L4Protocol:  uint32(flow.L4Protocol),
		Source:      ipPortContextToProto(&flow.Source),
		Destination: ipPortContextToProto(&flow.Destination),
		Ingress:     networkStatsToProto(&flow.Ingress),
		Egress:      networkStatsToProto(&flow.Egress),
	}
}

func ipPortContextToProto(ipPort *model.IPPortContext) *adproto.IPPortContext {
	if ipPort == nil {
		return nil
	}
	return &adproto.IPPortContext{
		Ip:   ipPort.IPNet.IP.String(),
		Port: uint32(ipPort.Port),
	}
}

func networkStatsToProto(stats *model.NetworkStats) *adproto.NetworkStats {
	if stats == nil {
		return nil
	}
	return &adproto.NetworkStats{
		DataSize:    stats.DataSize,
		PacketCount: stats.PacketCount,
	}
}

func syscallNodeToProto(sysc *SyscallNode) *adproto.SyscallNode {
	if sysc == nil {
		return nil
	}

	return &adproto.SyscallNode{
		NodeBase: nodeBaseToProto(&sysc.NodeBase),
		Syscall:  int32(sysc.Syscall),
	}
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
		Cookie64:    p.Cookie,
		IsThread:    p.IsThread,
		IsExecChild: p.IsExecExec,
		File:        fileEventToProto(&p.FileEvent),
		ContainerId: string(p.ContainerID),
		Tty:         escape(p.TTYName),
		Comm:        escape(p.Comm),

		ForkTime: TimestampToProto(&p.ForkTime),
		ExitTime: TimestampToProto(&p.ExitTime),
		ExecTime: TimestampToProto(&p.ExecTime),

		Credentials: credentialsToProto(&p.Credentials),

		Args:          copyAndEscape(p.Argv),
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
		Hashes:            make([]string, len(fe.Hashes)),
		HashState:         adproto.HashState(fe.HashState),
	}
	copy(fi.Hashes, fe.Hashes)

	return fi
}

func fileActivityNodeToProto(fan *FileNode) *adproto.FileActivityNode {
	if fan == nil {
		return nil
	}

	pfan := adproto.FileActivityNodeFromVTPool()
	*pfan = adproto.FileActivityNode{
		MatchedRules:   make([]*adproto.MatchedRule, 0, len(fan.MatchedRules)),
		Name:           escape(fan.Name),
		File:           fileEventToProto(fan.File),
		GenerationType: adproto.GenerationType(fan.GenerationType),
		Open:           openNodeToProto(fan.Open),
		Children:       make([]*adproto.FileActivityNode, 0, len(fan.Children)),
		NodeBase:       nodeBaseToProto(&fan.NodeBase),
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

	pdn.NodeBase = nodeBaseToProto(&dn.NodeBase)

	return pdn
}

func dnsEventToProto(ev *model.DNSEvent) *adproto.DNSInfo {
	if ev == nil {
		return nil
	}

	return &adproto.DNSInfo{
		Name:  escape(ev.Question.Name),
		Type:  uint32(ev.Question.Type),
		Class: uint32(ev.Question.Class),
		Size:  uint32(ev.Question.Size),
		Count: uint32(ev.Question.Count),
	}
}

func imdsNodeToProto(in *IMDSNode) *adproto.IMDSNode {
	if in == nil {
		return nil
	}

	pin := &adproto.IMDSNode{
		MatchedRules: make([]*adproto.MatchedRule, 0, len(in.MatchedRules)),
		NodeBase:     nodeBaseToProto(&in.NodeBase),
		Event:        imdsEventToProto(in.Event),
	}

	return pin
}

func imdsEventToProto(event model.IMDSEvent) *adproto.IMDSEvent {
	return &adproto.IMDSEvent{
		Type:          event.Type,
		CloudProvider: event.CloudProvider,
		Url:           event.URL,
		Host:          event.Host,
		UserAgent:     event.UserAgent,
		Server:        event.Server,
		Aws:           awsIMDSEventToProto(event),
	}
}

func awsIMDSEventToProto(event model.IMDSEvent) *adproto.AWSIMDSEvent {
	if event.CloudProvider != model.IMDSAWSCloudProvider {
		return nil
	}
	return &adproto.AWSIMDSEvent{
		IsImdsV2: event.AWS.IsIMDSv2,
		SecurityCredentials: &adproto.AWSSecurityCredentials{
			Type:          event.AWS.SecurityCredentials.Type,
			Code:          event.AWS.SecurityCredentials.Code,
			AccessKeyId:   event.AWS.SecurityCredentials.AccessKeyID,
			LastUpdated:   event.AWS.SecurityCredentials.LastUpdated,
			ExpirationRaw: event.AWS.SecurityCredentials.ExpirationRaw,
		},
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
			Protocol:     uint32(bn.Protocol),
			NodeBase:     nodeBaseToProto(&bn.NodeBase),
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

// TimestampToProto encode a timestamp
func TimestampToProto(t *time.Time) uint64 {
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

func nodeBaseToProto(nb *NodeBase) *adproto.NodeBase {
	if nb == nil {
		return nil
	}

	pnb := &adproto.NodeBase{
		Seen: make(map[string]*adproto.ImageTagTimes, len(nb.Seen)),
	}

	for imageTag, times := range nb.Seen {
		pnb.Seen[imageTag] = &adproto.ImageTagTimes{
			FirstSeen: TimestampToProto(&times.FirstSeen),
			LastSeen:  TimestampToProto(&times.LastSeen),
		}
	}

	return pnb
}
