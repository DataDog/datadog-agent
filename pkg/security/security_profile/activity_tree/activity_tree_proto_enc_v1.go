// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package activitytree holds activitytree related files
package activitytree

import (
	"slices"
	"time"
	"unicode/utf8"

	adproto "github.com/DataDog/agent-payload/v5/cws/dumpsv1"
	"golang.org/x/text/runes"

	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/utils"
)

// ToProto encodes an activity tree to its protobuf representation
func ToProto(at *ActivityTree) []*adproto.ProcessActivityNode {
	at.FlushPendingHashes()
	out := make([]*adproto.ProcessActivityNode, 0, len(at.ProcessNodes))

	for _, node := range at.ProcessNodes {
		out = append(out, processActivityNodeToProto(node, at.GetTagFromID))
	}
	return out
}

// UnattributedActivityToProto encodes the unattributed activity of an activity tree to its protobuf representation
func UnattributedActivityToProto(at *ActivityTree) *adproto.UnattributedActivity {
	if at == nil || at.UnattributedActivity == nil {
		return nil
	}
	at.FlushPendingHashes()

	// pan isn't returned to the pool: its activity nodes now belong to the output
	pan := processActivityNodeToProto(at.UnattributedActivity, at.GetTagFromID)
	return &adproto.UnattributedActivity{
		NodeBase:        pan.NodeBase,
		Files:           pan.Files,
		DnsNames:        pan.DnsNames,
		Sockets:         pan.Sockets,
		ImdsEvents:      pan.ImdsEvents,
		SyscallNodes:    pan.SyscallNodes,
		NetworkDevices:  pan.NetworkDevices,
		CapabilityNodes: pan.CapabilityNodes,
	}
}

// MountsToProto encodes the workload's deduplicated mount table to its protobuf representation
func MountsToProto(at *ActivityTree) []*adproto.MountNode {
	if len(at.Mounts) == 0 {
		return nil
	}

	out := make([]*adproto.MountNode, 0, len(at.Mounts))
	for _, mn := range at.Mounts {
		out = append(out, &adproto.MountNode{
			NodeBase:      nodeBaseToProto(&mn.NodeBase, at.GetTagFromID),
			MountPoint:    mn.MountPoint,
			MountRoot:     mn.MountRoot,
			Filesystem:    mn.Filesystem,
			MountFlags:    mn.MountFlags,
			BaseNamespace: mn.InBaseNamespace,
		})
	}
	return out
}

func processActivityNodeToProto(pan *ProcessNode, tagIDToImageTag func(id uint64) string) *adproto.ProcessActivityNode {
	if pan == nil {
		return nil
	}

	ppan := adproto.ProcessActivityNodeFromVTPool()
	*ppan = adproto.ProcessActivityNode{
		Process:         processNodeToProto(&pan.Process),
		GenerationType:  adproto.GenerationType(pan.GenerationType),
		MatchedRules:    make([]*adproto.MatchedRule, 0, len(pan.MatchedRules)),
		Children:        make([]*adproto.ProcessActivityNode, 0, len(pan.Children)),
		Files:           make([]*adproto.FileActivityNode, 0, len(pan.Files)),
		DnsNames:        make([]*adproto.DNSNode, 0, len(pan.DNSNames)),
		ImdsEvents:      make([]*adproto.IMDSNode, 0, len(pan.IMDSEvents)),
		Sockets:         make([]*adproto.SocketNode, 0, len(pan.Sockets)),
		NodeBase:        nodeBaseToProto(&pan.NodeBase, tagIDToImageTag),
		SyscallNodes:    make([]*adproto.SyscallNode, 0, len(pan.Syscalls)),
		NetworkDevices:  make([]*adproto.NetworkDeviceNode, 0, len(pan.NetworkDevices)),
		CapabilityNodes: make([]*adproto.CapabilityNode, 0, len(pan.Capabilities)),
	}

	for _, rule := range pan.MatchedRules {
		ppan.MatchedRules = append(ppan.MatchedRules, matchedRuleToProto(rule))
	}

	for _, child := range pan.Children {
		ppan.Children = append(ppan.Children, processActivityNodeToProto(child, tagIDToImageTag))
	}

	for _, fan := range pan.Files {
		ppan.Files = append(ppan.Files, fileActivityNodeToProto(fan, tagIDToImageTag))
	}

	for _, dns := range pan.DNSNames {
		ppan.DnsNames = append(ppan.DnsNames, dnsNodeToProto(dns, tagIDToImageTag))
	}

	for _, imds := range pan.IMDSEvents {
		ppan.ImdsEvents = append(ppan.ImdsEvents, imdsNodeToProto(imds, tagIDToImageTag))
	}

	for _, socket := range pan.Sockets {
		ppan.Sockets = append(ppan.Sockets, socketNodeToProto(socket, tagIDToImageTag))
	}

	syscallIDs := make([]int, 0, len(pan.Syscalls))
	for id := range pan.Syscalls {
		syscallIDs = append(syscallIDs, id)
	}
	slices.Sort(syscallIDs)
	for _, id := range syscallIDs {
		ppan.SyscallNodes = append(ppan.SyscallNodes, syscallNodeToProto(pan.Syscalls[id], tagIDToImageTag))
	}

	for _, networkDevice := range pan.NetworkDevices {
		ppan.NetworkDevices = append(ppan.NetworkDevices, networkDeviceToProto(networkDevice, tagIDToImageTag))
	}

	for _, capNode := range pan.Capabilities {
		ppan.CapabilityNodes = append(ppan.CapabilityNodes, capabilityNodeToProto(capNode, tagIDToImageTag))
	}

	return ppan
}

func networkDeviceToProto(device *NetworkDeviceNode, tagIDToImageTag func(id uint64) string) *adproto.NetworkDeviceNode {
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
		ndn.FlowNodes = append(ndn.FlowNodes, flowNodeToProto(flowNode.Flow, &flowNode.NodeBase, tagIDToImageTag))
	}

	return ndn
}

func flowNodeToProto(flow model.Flow, nodeBase *NodeBase, tagIDToImageTag func(id uint64) string) *adproto.FlowNode {
	return &adproto.FlowNode{
		NodeBase:    nodeBaseToProto(nodeBase, tagIDToImageTag),
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
		Ip:   utils.GetIPStringFromIPNet(ipPort.IPNet),
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

func syscallNodeToProto(sysc *SyscallNode, tagIDToImageTag func(id uint64) string) *adproto.SyscallNode {
	if sysc == nil {
		return nil
	}

	return &adproto.SyscallNode{
		NodeBase: nodeBaseToProto(&sysc.NodeBase, tagIDToImageTag),
		Syscall:  int32(sysc.Syscall),
	}
}

func processNodeToProto(p *ProcessInfo) *adproto.ProcessInfo {
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
		File:        processFileToProto(p),
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

// fileInfoToProto encodes a slim FileInfo directly into the proto, without round-tripping
// through a reconstructed model.FileEvent.
func fileInfoToProto(fi *FileInfo) *adproto.FileInfo {
	if fi == nil {
		return nil
	}

	hashState, hashes := fi.GetHashes()
	pkgFields := struct {
		epoch, srcEpoch     uint32
		release, srcRelease string
	}{
		epoch:      uint32(fi.PkgEpoch),
		srcEpoch:   uint32(fi.PkgSrcEpoch),
		release:    fi.PkgRelease,
		srcRelease: fi.PkgSrcRelease,
	}
	var hashesCopy []string
	if len(hashes) > 0 {
		hashesCopy = make([]string, len(hashes))
		copy(hashesCopy, hashes)
	}
	pfi := adproto.FileInfoFromVTPool()
	*pfi = adproto.FileInfo{
		Uid:               fi.UID,
		User:              fi.User,
		Gid:               fi.GID,
		Group:             fi.Group,
		Mode:              uint32(fi.Mode), // yeah sorry
		Ctime:             fi.CTime,
		Mtime:             fi.MTime,
		MountId:           fi.MountID,
		Inode:             fi.Inode,
		InUpperLayer:      fi.InUpperLayer,
		Path:              escape(fi.PathnameStr),
		Basename:          escape(fi.BasenameStr),
		Filesystem:        escape(fi.Filesystem),
		PackageName:       fi.PkgName,
		PackageVersion:    fi.PkgVersion,
		PackageEpoch:      &pkgFields.epoch,
		PackageRelease:    &pkgFields.release,
		PackageSrcVersion: fi.PkgSrcVersion,
		PackageSrcEpoch:   &pkgFields.srcEpoch,
		PackageSrcRelease: &pkgFields.srcRelease,
		Hashes:            hashesCopy,
		HashState:         adproto.HashState(hashState),
	}

	return pfi
}

func processFileToProto(p *ProcessInfo) *adproto.FileInfo {
	if p == nil {
		return nil
	}
	hashState, hashes := p.GetHashes()
	fe := &p.FileEvent
	fi := FileInfo{
		FileFields:    fe.FileFields,
		PathnameStr:   fe.PathnameStr,
		BasenameStr:   fe.BasenameStr,
		Filesystem:    fe.Filesystem,
		PkgName:       fe.PkgName,
		PkgVersion:    fe.PkgVersion,
		PkgEpoch:      fe.PkgEpoch,
		PkgRelease:    fe.PkgRelease,
		PkgSrcVersion: fe.PkgSrcVersion,
		PkgSrcEpoch:   fe.PkgSrcEpoch,
		PkgSrcRelease: fe.PkgSrcRelease,
		HashState:     hashState,
		Hashes:        hashes,
	}
	return fileInfoToProto(&fi)
}

func fileActivityNodeToProto(fan *FileNode, tagIDToImageTag func(id uint64) string) *adproto.FileActivityNode {
	if fan == nil {
		return nil
	}

	pfan := adproto.FileActivityNodeFromVTPool()
	*pfan = adproto.FileActivityNode{
		MatchedRules:   make([]*adproto.MatchedRule, 0, len(fan.MatchedRules)),
		Name:           escape(fan.Name),
		IsPattern:      fan.IsPattern,
		File:           fileInfoToProto(fan.File),
		GenerationType: adproto.GenerationType(fan.GenerationType),
		Open:           openNodeToProto(fan.Open),
		Children:       make([]*adproto.FileActivityNode, 0, len(fan.Children)),
		NodeBase:       nodeBaseToProto(&fan.NodeBase, tagIDToImageTag),
	}

	for _, rule := range fan.MatchedRules {
		pfan.MatchedRules = append(pfan.MatchedRules, matchedRuleToProto(rule))
	}

	for _, child := range fan.Children {
		pfan.Children = append(pfan.Children, fileActivityNodeToProto(child, tagIDToImageTag))
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

func dnsNodeToProto(dn *DNSNode, tagIDToImageTag func(id uint64) string) *adproto.DNSNode {
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
		pdn.Requests = append(pdn.Requests, dnsRequestNodeToProto(&req))
	}

	pdn.NodeBase = nodeBaseToProto(&dn.NodeBase, tagIDToImageTag)

	return pdn
}

func dnsRequestNodeToProto(req *DNSRequestNode) *adproto.DNSInfo {
	if req == nil {
		return nil
	}

	return &adproto.DNSInfo{
		Name:     escape(req.Question.Name),
		Type:     uint32(req.Question.Type),
		Class:    uint32(req.Question.Class),
		Size:     uint32(req.Question.Size),
		Count:    uint32(req.Question.Count),
		Response: dnsResponseToProto(req.Response),
	}
}

func dnsResponseToProto(resp *DNSResponseAggregate) *adproto.DNSResponseInfo {
	if resp == nil {
		return nil
	}

	presp := &adproto.DNSResponseInfo{
		Ips:    make([]string, 0, len(resp.IPs)),
		Cnames: make([]string, 0, len(resp.CNames)),
	}

	for _, ip := range resp.IPs {
		if str := utils.GetIPStringFromIPNet(ip); str != "" {
			presp.Ips = append(presp.Ips, str)
		}
	}

	for _, cname := range resp.CNames {
		presp.Cnames = append(presp.Cnames, escape(cname))
	}

	return presp
}

func imdsNodeToProto(in *IMDSNode, tagIDToImageTag func(id uint64) string) *adproto.IMDSNode {
	if in == nil {
		return nil
	}

	pin := &adproto.IMDSNode{
		MatchedRules: make([]*adproto.MatchedRule, 0, len(in.MatchedRules)),
		NodeBase:     nodeBaseToProto(&in.NodeBase, tagIDToImageTag),
		Event:        imdsEventToProto(in.Event),
	}

	return pin
}

func imdsEventToProto(event IMDSInfo) *adproto.IMDSEvent {
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

func awsIMDSEventToProto(event IMDSInfo) *adproto.AWSIMDSEvent {
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

func socketNodeToProto(sn *SocketNode, tagIDToImageTag func(id uint64) string) *adproto.SocketNode {
	if sn == nil {
		return nil
	}

	bindLen := len(sn.Bind)
	connectLen := len(sn.Connect)
	psn := &adproto.SocketNode{
		Family: sn.Family,
	}
	if bindLen == 0 && connectLen == 0 {
		return psn
	}

	totalSeen := 0
	for _, bn := range sn.Bind {
		totalSeen += bn.SeenLen()
	}
	for _, cn := range sn.Connect {
		totalSeen += cn.SeenLen()
	}

	nodeBases := make([]adproto.NodeBase, bindLen+connectLen)
	var timesBuf []adproto.ImageTagTimes
	if totalSeen > 0 {
		timesBuf = make([]adproto.ImageTagTimes, totalSeen)
	}
	nbIdx := 0

	if bindLen > 0 {
		psn.Bind = make([]*adproto.BindNode, bindLen)
		bindVals := make([]adproto.BindNode, bindLen)
		for i, bn := range sn.Bind {
			pnb := &nodeBases[nbIdx]
			nbIdx++
			timesBuf = fillNodeBaseToProto(pnb, &bn.NodeBase, tagIDToImageTag, timesBuf)

			pbn := &bindVals[i]
			pbn.Port = uint32(bn.Port)
			pbn.Ip = bn.IP
			pbn.Protocol = uint32(bn.Protocol)
			pbn.NodeBase = pnb
			if len(bn.MatchedRules) > 0 {
				pbn.MatchedRules = make([]*adproto.MatchedRule, 0, len(bn.MatchedRules))
				for _, rule := range bn.MatchedRules {
					pbn.MatchedRules = append(pbn.MatchedRules, matchedRuleToProto(rule))
				}
			}
			psn.Bind[i] = pbn
		}
	}

	if connectLen > 0 {
		psn.Connect = make([]*adproto.ConnectNode, connectLen)
		connectVals := make([]adproto.ConnectNode, connectLen)
		for i, cn := range sn.Connect {
			pnb := &nodeBases[nbIdx]
			nbIdx++
			timesBuf = fillNodeBaseToProto(pnb, &cn.NodeBase, tagIDToImageTag, timesBuf)

			pcn := &connectVals[i]
			pcn.Port = uint32(cn.Port)
			pcn.Ip = cn.IP
			pcn.Protocol = uint32(cn.Protocol)
			pcn.NodeBase = pnb
			if len(cn.MatchedRules) > 0 {
				pcn.MatchedRules = make([]*adproto.MatchedRule, 0, len(cn.MatchedRules))
				for _, rule := range cn.MatchedRules {
					pcn.MatchedRules = append(pcn.MatchedRules, matchedRuleToProto(rule))
				}
			}
			psn.Connect[i] = pcn
		}
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
	for _, value := range in {
		out = append(out, escape(value))
	}
	return out
}

func escape(in string) string {
	if utf8.ValidString(in) {
		return in
	}
	transformer := runes.ReplaceIllFormed()
	return transformer.String(in)
}

func fillNodeBaseToProto(pnb *adproto.NodeBase, nb *NodeBase, tagIDToImageTag func(id uint64) string, timesBuf []adproto.ImageTagTimes) []adproto.ImageTagTimes {
	pnb.LastAnomalySent = uint64(nb.lastAnomalySent)
	seenLen := nb.SeenLen()
	if seenLen == 0 {
		return timesBuf
	}
	pnb.Seen = make(map[string]*adproto.ImageTagTimes, seenLen)
	if len(timesBuf) < seenLen {
		timesBuf = make([]adproto.ImageTagTimes, seenLen)
	}
	nb.EachSeen(func(id uint64, firstSeen, lastSeen int64) {
		tag := tagIDToImageTag(id)
		if tag == "" {
			// ID is stale (slot was freed before this node was evicted); skip to avoid
			// writing an empty key into the proto map.
			return
		}
		t := &timesBuf[0]
		timesBuf = timesBuf[1:]
		t.FirstSeen = uint64(firstSeen)
		t.LastSeen = uint64(lastSeen)
		pnb.Seen[tag] = t
	})
	return timesBuf
}

func nodeBaseToProto(nb *NodeBase, tagIDToImageTag func(id uint64) string) *adproto.NodeBase {
	if nb == nil {
		return nil
	}

	pnb := &adproto.NodeBase{}
	fillNodeBaseToProto(pnb, nb, tagIDToImageTag, nil)
	return pnb
}

func capabilityNodeToProto(cap *CapabilityNode, tagIDToImageTag func(id uint64) string) *adproto.CapabilityNode {
	if cap == nil {
		return nil
	}

	return &adproto.CapabilityNode{
		NodeBase:              nodeBaseToProto(&cap.NodeBase, tagIDToImageTag),
		Capability:            cap.Capability,
		IsCapable:             cap.Capable,
		IsAttemptedHostUserns: cap.AttemptedHostUserNS,
		IsCapableHostUserns:   cap.CapableHostUserNS,
	}
}
