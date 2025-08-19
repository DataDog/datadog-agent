// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package activitytree holds activitytree related files
package activitytree

import (
	"net"
	"time"

	adproto "github.com/DataDog/agent-payload/v5/cws/dumpsv1"

	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
	"github.com/DataDog/datadog-agent/pkg/security/secl/containerutils"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
)

// ProtoDecodeActivityTree decodes an ActivityTree structure
func ProtoDecodeActivityTree(dest *ActivityTree, nodes []*adproto.ProcessActivityNode) {
	for _, node := range nodes {
		dest.ProcessNodes = append(dest.ProcessNodes, protoDecodeProcessActivityNode(dest, node))
	}
}

func protoDecodeProcessActivityNode(parent ProcessNodeParent, pan *adproto.ProcessActivityNode) *ProcessNode {
	if pan == nil {
		return nil
	}

	ppan := &ProcessNode{
		Process:        protoDecodeProcessNode(pan.Process),
		Parent:         parent,
		GenerationType: NodeGenerationType(pan.GenerationType),
		MatchedRules:   make([]*model.MatchedRule, 0, len(pan.MatchedRules)),
		Children:       make([]*ProcessNode, 0, len(pan.Children)),
		Files:          make(map[string]*FileNode, len(pan.Files)),
		DNSNames:       make(map[string]*DNSNode, len(pan.DnsNames)),
		IMDSEvents:     make(map[model.IMDSEvent]*IMDSNode, len(pan.ImdsEvents)),
		Sockets:        make([]*SocketNode, 0, len(pan.Sockets)),
		Syscalls:       make([]*SyscallNode, 0, len(pan.SyscallNodes)),
		NodeBase:       NewNodeBase(),
		NetworkDevices: make(map[model.NetworkDeviceContext]*NetworkDeviceNode, len(pan.NetworkDevices)),
	}

	if pan.NodeBase != nil {
		for tag, imageTagTimes := range pan.NodeBase.Seen {
			firstSeen := ProtoDecodeTimestamp(imageTagTimes.FirstSeen)
			lastSeen := ProtoDecodeTimestamp(imageTagTimes.LastSeen)
			ppan.RecordWithTimestamps(tag, firstSeen, lastSeen)
		}
	}

	for _, rule := range pan.MatchedRules {
		ppan.MatchedRules = append(ppan.MatchedRules, protoDecodeProtoMatchedRule(rule))
	}

	for _, child := range pan.Children {
		ppan.Children = append(ppan.Children, protoDecodeProcessActivityNode(ppan, child))
	}

	for _, fan := range pan.Files {
		protoDecodedFan := protoDecodeFileActivityNode(fan)
		ppan.Files[protoDecodedFan.Name] = protoDecodedFan
	}

	for _, dns := range pan.DnsNames {
		protoDecodedDNS := protoDecodeDNSNode(dns)
		if len(protoDecodedDNS.Requests) != 0 {
			name := protoDecodedDNS.Requests[0].Question.Name
			ppan.DNSNames[name] = protoDecodedDNS
		}
	}

	for _, imds := range pan.ImdsEvents {
		node := protoDecodeIMDSNode(imds)
		ppan.IMDSEvents[node.Event] = node
	}

	for _, socket := range pan.Sockets {
		ppan.Sockets = append(ppan.Sockets, protoDecodeProtoSocket(socket))
	}

	for _, sysc := range pan.SyscallNodes {
		ppan.Syscalls = append(ppan.Syscalls, protoDecodeSyscallNode(sysc))
	}

	for _, networkDevice := range pan.NetworkDevices {
		ppan.NetworkDevices[model.NetworkDeviceContext{
			NetNS:   networkDevice.Netns,
			IfIndex: networkDevice.Ifindex,
			IfName:  networkDevice.Ifname,
		}] = protoDecodeNetworkDevice(networkDevice)
	}

	return ppan
}

func protoDecodeSyscallNode(sysc *adproto.SyscallNode) *SyscallNode {
	if sysc == nil {
		return nil
	}

	syscallNode := &SyscallNode{
		NodeBase:       NewNodeBase(),
		GenerationType: Runtime,
		Syscall:        int(sysc.Syscall),
	}

	if sysc.NodeBase != nil {
		for tag, imageTagTimes := range sysc.NodeBase.Seen {
			firstSeen := ProtoDecodeTimestamp(imageTagTimes.FirstSeen)
			lastSeen := ProtoDecodeTimestamp(imageTagTimes.LastSeen)
			syscallNode.RecordWithTimestamps(tag, firstSeen, lastSeen)
		}
	}

	return syscallNode
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
		IsExecExec:  p.IsExecChild,
		FileEvent:   *protoDecodeFileEvent(p.File),
		ContainerID: containerutils.ContainerID(p.ContainerId),
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
		Open:           protoDecodeOpenNode(fan.Open),
		Children:       make(map[string]*FileNode, len(fan.Children)),
		NodeBase:       NewNodeBase(),
	}

	if fan.NodeBase != nil {
		for tag, imageTagTimes := range fan.NodeBase.Seen {
			firstSeen := ProtoDecodeTimestamp(imageTagTimes.FirstSeen)
			lastSeen := ProtoDecodeTimestamp(imageTagTimes.LastSeen)
			pfan.RecordWithTimestamps(tag, firstSeen, lastSeen)
		}
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
		NodeBase:     NewNodeBase(),
	}

	if dn.NodeBase != nil {
		for tag, imageTagTimes := range dn.NodeBase.Seen {
			firstSeen := ProtoDecodeTimestamp(imageTagTimes.FirstSeen)
			lastSeen := ProtoDecodeTimestamp(imageTagTimes.LastSeen)
			pdn.RecordWithTimestamps(tag, firstSeen, lastSeen)
		}
	}

	for _, rule := range dn.MatchedRules {
		pdn.MatchedRules = append(pdn.MatchedRules, protoDecodeProtoMatchedRule(rule))
	}

	for _, req := range dn.Requests {
		pdn.Requests = append(pdn.Requests, protoDecodeDNSInfo(req))
	}

	return pdn
}

func protoDecodeNetworkDevice(device *adproto.NetworkDeviceNode) *NetworkDeviceNode {
	if device == nil {
		return nil
	}
	ndn := &NetworkDeviceNode{
		MatchedRules: make([]*model.MatchedRule, 0, len(device.MatchedRules)),
		FlowNodes:    make(map[model.FiveTuple]*FlowNode, len(device.FlowNodes)),
		Context: model.NetworkDeviceContext{
			NetNS:   device.Netns,
			IfIndex: device.Ifindex,
			IfName:  device.Ifname,
		},
	}

	for _, rule := range device.MatchedRules {
		ndn.MatchedRules = append(ndn.MatchedRules, protoDecodeProtoMatchedRule(rule))
	}

	for _, flow := range device.FlowNodes {
		f := protoDecodeNetworkFlow(flow)
		_, ok := ndn.FlowNodes[f.GetFiveTuple()]
		if !ok {
			fn := &FlowNode{
				NodeBase:       NewNodeBase(),
				GenerationType: Runtime,
				Flow:           *f,
			}
			if flow.NodeBase != nil {
				for tag, imageTagTimes := range flow.NodeBase.Seen {
					firstSeen := ProtoDecodeTimestamp(imageTagTimes.FirstSeen)
					lastSeen := ProtoDecodeTimestamp(imageTagTimes.LastSeen)
					fn.RecordWithTimestamps(tag, firstSeen, lastSeen)
				}
			}
			ndn.FlowNodes[f.GetFiveTuple()] = fn
		}
	}

	return ndn
}

func protoDecodeNetworkFlow(flowNode *adproto.FlowNode) *model.Flow {
	return &model.Flow{
		Source:      protoDecodeIPPortContext(flowNode.Source),
		Destination: protoDecodeIPPortContext(flowNode.Destination),
		L3Protocol:  uint16(flowNode.L3Protocol),
		L4Protocol:  uint16(flowNode.L4Protocol),
		Ingress:     protoDecodeNetworkStats(flowNode.Ingress),
		Egress:      protoDecodeNetworkStats(flowNode.Egress),
	}
}

func protoDecodeIPPortContext(ipPort *adproto.IPPortContext) model.IPPortContext {
	ipc := model.IPPortContext{
		IPNet: *eval.IPNetFromIP(net.ParseIP(ipPort.Ip)),
		Port:  uint16(ipPort.Port),
	}
	return ipc
}

func protoDecodeNetworkStats(stats *adproto.NetworkStats) model.NetworkStats {
	ns := model.NetworkStats{
		DataSize:    stats.DataSize,
		PacketCount: stats.PacketCount,
	}
	return ns
}

func protoDecodeIMDSNode(in *adproto.IMDSNode) *IMDSNode {
	if in == nil {
		return nil
	}

	node := &IMDSNode{
		MatchedRules: make([]*model.MatchedRule, 0, len(in.MatchedRules)),
		NodeBase:     NewNodeBase(),
		Event:        protoDecodeIMDSEvent(in.Event),
	}

	if in.NodeBase != nil {
		for tag, imageTagTimes := range in.NodeBase.Seen {
			firstSeen := ProtoDecodeTimestamp(imageTagTimes.FirstSeen)
			lastSeen := ProtoDecodeTimestamp(imageTagTimes.LastSeen)
			node.RecordWithTimestamps(tag, firstSeen, lastSeen)
		}
	}

	for _, rule := range in.MatchedRules {
		node.MatchedRules = append(node.MatchedRules, protoDecodeProtoMatchedRule(rule))
	}

	return node
}

func protoDecodeDNSInfo(ev *adproto.DNSInfo) model.DNSEvent {
	if ev == nil {
		return model.DNSEvent{}
	}

	return model.DNSEvent{
		Question: model.DNSQuestion{
			Name:  ev.Name,
			Type:  uint16(ev.Type),
			Class: uint16(ev.Class),
			Size:  uint16(ev.Size),
			Count: uint16(ev.Count),
		},
	}
}

func protoDecodeIMDSEvent(ie *adproto.IMDSEvent) model.IMDSEvent {
	if ie == nil {
		return model.IMDSEvent{}
	}

	return model.IMDSEvent{
		Type:          ie.Type,
		CloudProvider: ie.CloudProvider,
		URL:           ie.Url,
		Host:          ie.Host,
		Server:        ie.Server,
		UserAgent:     ie.UserAgent,
		AWS:           protoDecodeAWSIMDSEvent(ie.Aws),
	}
}

func protoDecodeAWSIMDSEvent(aie *adproto.AWSIMDSEvent) model.AWSIMDSEvent {
	if aie == nil {
		return model.AWSIMDSEvent{}
	}

	return model.AWSIMDSEvent{
		IsIMDSv2:            aie.IsImdsV2,
		SecurityCredentials: protoDecodeAWSSecurityCredentials(aie.SecurityCredentials),
	}
}

func protoDecodeAWSSecurityCredentials(creds *adproto.AWSSecurityCredentials) model.AWSSecurityCredentials {
	if creds == nil {
		return model.AWSSecurityCredentials{}
	}

	expiration, _ := time.Parse(time.RFC3339, creds.ExpirationRaw)

	return model.AWSSecurityCredentials{
		Code:          creds.Code,
		Type:          creds.Type,
		AccessKeyID:   creds.AccessKeyId,
		LastUpdated:   creds.LastUpdated,
		ExpirationRaw: creds.ExpirationRaw,
		Expiration:    expiration,
	}
}

func protoDecodeProtoSocket(sn *adproto.SocketNode) *SocketNode {
	if sn == nil {
		return nil
	}

	socketNode := &SocketNode{
		Family: sn.Family,
	}
	socketNode.NodeBase = NewNodeBase()

	for _, bindNode := range sn.GetBind() {
		psn := &BindNode{
			MatchedRules: make([]*model.MatchedRule, 0, len(bindNode.MatchedRules)),
			Port:         uint16(bindNode.Port),
			IP:           bindNode.Ip,
			Protocol:     uint16(bindNode.Protocol),
			NodeBase:     NewNodeBase(),
		}

		if bindNode.NodeBase != nil {
			for tag, imageTagTimes := range bindNode.NodeBase.Seen {
				firstSeen := ProtoDecodeTimestamp(imageTagTimes.FirstSeen)
				lastSeen := ProtoDecodeTimestamp(imageTagTimes.LastSeen)
				psn.RecordWithTimestamps(tag, firstSeen, lastSeen)
			}
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
