// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package activitytree holds activitytree related files
package activitytree

import (
	"bufio"
	"encoding/binary"
	"errors"
	"fmt"
	"hash/fnv"
	"io"
	"os"
	"path/filepath"
	"strings"

	adproto "github.com/DataDog/agent-payload/v5/cws/dumpsv1"

	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
)

const (
	journalKindProcess uint8 = 1 + iota
	journalKindFile
	journalKindDNS
	journalKindIMDS
	journalKindSocket
	journalKindNetworkDevice

	journalHeaderSize     = 17 // 1B kind + 4B seq + 4B procID + 4B subID + 4B payloadLen
	journalDefaultBufSize = 16 * 1024
	journalDefaultMaxSize = 64 << 20 // 64 MiB default cap per profile journal
)

// TreeJournal spills cold, high-footprint node payloads (process argv/envs/hashes/credentials,
// FileInfo structs and strings, DNS response IP/CNAME aggregates, and MatchedRules) to a
// compact binary protobuf journal file on disk while keeping the hot deduplication skeleton,
// pattern-matching trie, image-tag timestamps, and eBPF sample cookie targets in RAM.
type TreeJournal struct {
	path          string
	file          *os.File
	buf           *bufio.Writer
	seq           uint32
	nextProcID    uint32
	currentProcID uint32
	writtenBytes  int64
	maxBytes      int64
	dirty         bool
	failed        bool

	scratchBuf       []byte
	enrichmentHashes map[uint64]struct{}
}

func newTreeJournal(dir, key string, maxBytes int64) (*TreeJournal, error) {
	if dir == "" {
		return nil, errors.New("empty journal directory")
	}
	if err := os.MkdirAll(dir, 0750); err != nil {
		return nil, fmt.Errorf("couldn't create journal directory [%s]: %w", dir, err)
	}
	if maxBytes <= 0 {
		maxBytes = journalDefaultMaxSize
	}

	safeName := sanitizeJournalKey(key)
	journalPath := filepath.Join(dir, safeName+".journal")
	f, err := os.OpenFile(journalPath, os.O_CREATE|os.O_RDWR|os.O_TRUNC, 0600)
	if err != nil {
		return nil, fmt.Errorf("couldn't open journal file [%s]: %w", journalPath, err)
	}

	return &TreeJournal{
		path:             journalPath,
		file:             f,
		buf:              bufio.NewWriterSize(f, journalDefaultBufSize),
		maxBytes:         maxBytes,
		scratchBuf:       make([]byte, 0, 4096),
		enrichmentHashes: make(map[uint64]struct{}),
	}, nil
}

func sanitizeJournalKey(key string) string {
	h := fnv.New64a()
	_, _ = h.Write([]byte(key))
	var b strings.Builder
	for i := 0; i < len(key) && b.Len() < 48; i++ {
		c := key[i]
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '-' || c == '_' || c == '.' {
			b.WriteByte(c)
		} else {
			b.WriteByte('_')
		}
	}
	if b.Len() == 0 {
		return fmt.Sprintf("profile-%016x", h.Sum64())
	}
	return fmt.Sprintf("%s-%016x", b.String(), h.Sum64())
}

// EnableJournal attaches an on-disk binary journal to the ActivityTree.
func (at *ActivityTree) EnableJournal(dir, key string, maxBytes int64) error {
	if at == nil || at.Stats == nil {
		return nil
	}
	_ = at.CloseJournal()
	j, err := newTreeJournal(dir, key, maxBytes)
	if err != nil {
		return err
	}
	at.Stats.journal = j
	if len(at.ProcessNodes) > 0 {
		j.Checkpoint(at)
		j.StripTree(at)
	}
	return nil
}

// CloseJournal closes and removes the on-disk journal file if one is attached.
func (at *ActivityTree) CloseJournal() error {
	if at == nil || at.Stats == nil || at.Stats.journal == nil {
		return nil
	}
	err := at.Stats.journal.Close()
	at.Stats.journal = nil
	return err
}

// HasJournal reports whether an active on-disk journal is attached to the ActivityTree.
func (at *ActivityTree) HasJournal() bool {
	return at != nil && at.Stats != nil && at.Stats.journal != nil && !at.Stats.journal.failed
}

// JournalSize returns the current size in bytes of the on-disk journal.
func (at *ActivityTree) JournalSize() int64 {
	if at == nil || at.Stats == nil || at.Stats.journal == nil {
		return 0
	}
	return at.Stats.journal.writtenBytes
}

// MaterializeFromJournal replays the on-disk journal into the in-memory ActivityTree nodes
// without re-stripping them afterwards. Useful for tests that inspect leaf fields in place.
func (at *ActivityTree) MaterializeFromJournal() {
	if at == nil || at.Stats == nil || at.Stats.journal == nil {
		return
	}
	at.Stats.journal.Materialize(at)
}

// Close closes and removes the backing journal file.
func (j *TreeJournal) Close() error {
	if j == nil {
		return nil
	}
	var closeErr error
	if j.file != nil {
		closeErr = j.file.Close()
		j.file = nil
	}
	if j.path != "" {
		if err := os.Remove(j.path); err != nil && !os.IsNotExist(err) && closeErr == nil {
			closeErr = err
		}
	}
	return closeErr
}

func (j *TreeJournal) nextSeq() uint32 {
	j.seq++
	if j.seq == 0 {
		j.seq = 1
	}
	return j.seq
}

func (j *TreeJournal) allocProcID() uint32 {
	j.nextProcID++
	if j.nextProcID == 0 {
		j.nextProcID = 1
	}
	return j.nextProcID
}

func (j *TreeJournal) setCurrentProc(pn *ProcessNode) {
	if j == nil || pn == nil {
		return
	}
	if pn.journalID == 0 {
		j.recordProcessNode(pn)
	}
	j.currentProcID = pn.journalID
}

func (j *TreeJournal) writeFrame(kind uint8, seq, procID, subID uint32, msg *adproto.ProcessActivityNode) bool {
	if j == nil || j.failed || j.file == nil {
		return false
	}
	size := msg.SizeVT()
	frameBytes := int64(journalHeaderSize + size)
	if j.writtenBytes+frameBytes > j.maxBytes {
		j.failed = true
		return false
	}
	if cap(j.scratchBuf) < journalHeaderSize+size {
		j.scratchBuf = make([]byte, journalHeaderSize+size)
	} else {
		j.scratchBuf = j.scratchBuf[:journalHeaderSize+size]
	}
	j.scratchBuf[0] = kind
	binary.LittleEndian.PutUint32(j.scratchBuf[1:5], seq)
	binary.LittleEndian.PutUint32(j.scratchBuf[5:9], procID)
	binary.LittleEndian.PutUint32(j.scratchBuf[9:13], subID)
	binary.LittleEndian.PutUint32(j.scratchBuf[13:17], uint32(size))

	if _, err := msg.MarshalToSizedBufferVT(j.scratchBuf[journalHeaderSize:]); err != nil {
		j.failed = true
		return false
	}
	if _, err := j.buf.Write(j.scratchBuf); err != nil {
		j.failed = true
		return false
	}
	j.writtenBytes += frameBytes
	j.dirty = true
	return true
}

func (j *TreeJournal) markEnrichment(procID, minSeq uint32, category, key, subKey string) bool {
	h := fnv.New64a()
	var b [8]byte
	binary.LittleEndian.PutUint32(b[0:4], procID)
	binary.LittleEndian.PutUint32(b[4:8], minSeq)
	_, _ = h.Write(b[:])
	_, _ = h.Write([]byte(category))
	_, _ = h.Write([]byte{0})
	_, _ = h.Write([]byte(key))
	_, _ = h.Write([]byte{0})
	_, _ = h.Write([]byte(subKey))
	sum := h.Sum64()
	if _, exists := j.enrichmentHashes[sum]; exists {
		return false
	}
	j.enrichmentHashes[sum] = struct{}{}
	return true
}

func (j *TreeJournal) filterNewRules(procID, minSeq uint32, category, key string, rules []*model.MatchedRule) []*adproto.MatchedRule {
	if len(rules) == 0 {
		return nil
	}
	var out []*adproto.MatchedRule
	for _, r := range rules {
		if r == nil {
			continue
		}
		if j.markEnrichment(procID, minSeq, category, key, r.RuleID) {
			out = append(out, matchedRuleToProto(r))
		}
	}
	return out
}

func (j *TreeJournal) recordProcessNode(pn *ProcessNode) {
	if j == nil || j.failed || pn == nil {
		return
	}
	if pn.journalID == 0 {
		pn.journalID = j.allocProcID()
	}
	seq := j.nextSeq()
	if pn.minJournalSeq == 0 {
		pn.minJournalSeq = seq
	}
	msg := adproto.ProcessActivityNode{
		Process:      processNodeToProto(&pn.Process),
		MatchedRules: j.filterNewRules(pn.journalID, pn.minJournalSeq, "proc_rule", "", pn.MatchedRules),
	}
	if j.writeFrame(journalKindProcess, seq, pn.journalID, 0, &msg) {
		pn.stripForJournal()
	}
}

func (j *TreeJournal) recordProcessRules(pn *ProcessNode, rules []*model.MatchedRule) {
	if j == nil || j.failed || pn == nil || len(rules) == 0 {
		if pn != nil {
			pn.MatchedRules = model.AppendMatchedRule(pn.MatchedRules, rules)
		}
		return
	}
	if pn.journalID == 0 {
		j.recordProcessNode(pn)
	}
	newRules := j.filterNewRules(pn.journalID, pn.minJournalSeq, "proc_rule", "", rules)
	if len(newRules) == 0 {
		return
	}
	seq := j.nextSeq()
	msg := adproto.ProcessActivityNode{
		MatchedRules: newRules,
	}
	if !j.writeFrame(journalKindProcess, seq, pn.journalID, 0, &msg) {
		pn.MatchedRules = model.AppendMatchedRule(pn.MatchedRules, rules)
	}
}

func (j *TreeJournal) recordFileNode(fn *FileNode, reducedPath string) {
	if j == nil || j.failed || fn == nil || j.currentProcID == 0 {
		return
	}
	if fn.File == nil && len(fn.MatchedRules) == 0 {
		return
	}
	seq := j.nextSeq()
	if fn.minJournalSeq == 0 {
		fn.minJournalSeq = seq
	}
	var subID uint32
	if fn.File != nil && fn.File.IsDir() {
		subID = 1
	}
	msg := adproto.ProcessActivityNode{
		Files: []*adproto.FileActivityNode{
			{
				Name:         reducedPath,
				File:         fileInfoToProto(fn.File),
				MatchedRules: j.filterNewRules(j.currentProcID, fn.minJournalSeq, "file_rule", reducedPath, fn.MatchedRules),
			},
		},
	}
	if j.writeFrame(journalKindFile, seq, j.currentProcID, subID, &msg) {
		if fn.File != nil {
			fn.hasJournalFile = true
			fn.File = nil
		}
		fn.MatchedRules = nil
	}
}

func (j *TreeJournal) recordFileRules(fn *FileNode, reducedPath string, rules []*model.MatchedRule) {
	if j == nil || j.failed || fn == nil || j.currentProcID == 0 || len(rules) == 0 {
		if fn != nil && len(rules) > 0 {
			fn.MatchedRules = model.AppendMatchedRule(fn.MatchedRules, rules)
		}
		return
	}
	if fn.minJournalSeq == 0 {
		fn.minJournalSeq = j.nextSeq()
	}
	newRules := j.filterNewRules(j.currentProcID, fn.minJournalSeq, "file_rule", reducedPath, rules)
	if len(newRules) == 0 {
		return
	}
	seq := j.nextSeq()
	msg := adproto.ProcessActivityNode{
		Files: []*adproto.FileActivityNode{
			{
				Name:         reducedPath,
				MatchedRules: newRules,
			},
		},
	}
	if !j.writeFrame(journalKindFile, seq, j.currentProcID, 0, &msg) {
		fn.MatchedRules = model.AppendMatchedRule(fn.MatchedRules, rules)
	}
}

func (j *TreeJournal) recordDNSEnrichment(pn *ProcessNode, dn *DNSNode, qIdx int, resp *model.DNSResponse, rules []*model.MatchedRule) {
	if j == nil || j.failed || pn == nil || dn == nil || qIdx < 0 || qIdx >= len(dn.Requests) {
		return
	}
	j.setCurrentProc(pn)
	if dn.minJournalSeq == 0 {
		dn.minJournalSeq = j.nextSeq()
	}
	req := &dn.Requests[qIdx]
	qKey := fmt.Sprintf("%s:%d", req.Question.Name, req.Question.Type)
	newRules := j.filterNewRules(pn.journalID, dn.minJournalSeq, "dns_rule", req.Question.Name, rules)

	var filteredResp *DNSResponseAggregate
	if resp != nil {
		for _, ip := range resp.IPs {
			if req.journalIPCount >= maxDNSResponseIPs {
				break
			}
			if j.markEnrichment(pn.journalID, dn.minJournalSeq, "dns_ip", qKey, ip.String()) {
				if filteredResp == nil {
					filteredResp = &DNSResponseAggregate{}
				}
				filteredResp.IPs = append(filteredResp.IPs, copyIPNet(ip))
				req.journalIPCount++
			}
		}
		for _, cname := range resp.CNames {
			if req.journalCNameCount >= maxDNSResponseCNames {
				break
			}
			if j.markEnrichment(pn.journalID, dn.minJournalSeq, "dns_cname", qKey, cname) {
				if filteredResp == nil {
					filteredResp = &DNSResponseAggregate{}
				}
				filteredResp.CNames = append(filteredResp.CNames, cname)
				req.journalCNameCount++
			}
		}
	}

	if filteredResp == nil && len(newRules) == 0 {
		req.Response = nil
		dn.MatchedRules = nil
		return
	}

	seq := j.nextSeq()
	msg := adproto.ProcessActivityNode{
		DnsNames: []*adproto.DNSNode{
			{
				MatchedRules: newRules,
				Requests: []*adproto.DNSInfo{
					{
						Name:     req.Question.Name,
						Type:     uint32(req.Question.Type),
						Class:    uint32(req.Question.Class),
						Size:     uint32(req.Question.Size),
						Count:    uint32(req.Question.Count),
						Response: dnsResponseToProto(filteredResp),
					},
				},
			},
		},
	}
	if j.writeFrame(journalKindDNS, seq, pn.journalID, uint32(qIdx), &msg) {
		req.Response = nil
		dn.MatchedRules = nil
	}
}

func (j *TreeJournal) recordIMDSRules(pn *ProcessNode, in *IMDSNode, rules []*model.MatchedRule) {
	if j == nil || j.failed || pn == nil || in == nil || len(rules) == 0 {
		if in != nil && len(rules) > 0 {
			in.MatchedRules = model.AppendMatchedRule(in.MatchedRules, rules)
		}
		return
	}
	j.setCurrentProc(pn)
	if in.minJournalSeq == 0 {
		in.minJournalSeq = j.nextSeq()
	}
	key := in.Event.URL + "|" + in.Event.AWS.SecurityCredentials.AccessKeyID
	newRules := j.filterNewRules(pn.journalID, in.minJournalSeq, "imds_rule", key, rules)
	if len(newRules) == 0 {
		in.MatchedRules = nil
		return
	}
	seq := j.nextSeq()
	protoIMDS := imdsNodeToProto(in, nil)
	protoIMDS.MatchedRules = newRules
	msg := adproto.ProcessActivityNode{
		ImdsEvents: []*adproto.IMDSNode{protoIMDS},
	}
	if j.writeFrame(journalKindIMDS, seq, pn.journalID, 0, &msg) {
		in.MatchedRules = nil
	}
}

func (j *TreeJournal) recordSocketRules(family string, isConnect bool, port uint16, ip string, protocol uint16, minSeq *uint32, rules []*model.MatchedRule) bool {
	if j == nil || j.failed || j.currentProcID == 0 || len(rules) == 0 {
		return false
	}
	if *minSeq == 0 {
		*minSeq = j.nextSeq()
	}
	key := fmt.Sprintf("%s:%v:%s:%d:%d", family, isConnect, ip, port, protocol)
	newRules := j.filterNewRules(j.currentProcID, *minSeq, "sock_rule", key, rules)
	if len(newRules) == 0 {
		return true
	}
	seq := j.nextSeq()
	sockProto := &adproto.SocketNode{Family: family}
	var subID uint32
	if isConnect {
		subID = 1
		sockProto.Connect = []*adproto.ConnectNode{
			{
				Port:         uint32(port),
				Ip:           ip,
				Protocol:     uint32(protocol),
				MatchedRules: newRules,
			},
		}
	} else {
		sockProto.Bind = []*adproto.BindNode{
			{
				Port:         uint32(port),
				Ip:           ip,
				Protocol:     uint32(protocol),
				MatchedRules: newRules,
			},
		}
	}
	msg := adproto.ProcessActivityNode{
		Sockets: []*adproto.SocketNode{sockProto},
	}
	return j.writeFrame(journalKindSocket, seq, j.currentProcID, subID, &msg)
}

func (j *TreeJournal) recordNetworkDeviceRules(ndn *NetworkDeviceNode, rules []*model.MatchedRule) {
	if j == nil || j.failed || ndn == nil || j.currentProcID == 0 || len(rules) == 0 {
		if ndn != nil && len(rules) > 0 {
			ndn.MatchedRules = model.AppendMatchedRule(ndn.MatchedRules, rules)
		}
		return
	}
	if ndn.minJournalSeq == 0 {
		ndn.minJournalSeq = j.nextSeq()
	}
	key := fmt.Sprintf("%d:%d", ndn.Context.NetNS, ndn.Context.IfIndex)
	newRules := j.filterNewRules(j.currentProcID, ndn.minJournalSeq, "netdev_rule", key, rules)
	if len(newRules) == 0 {
		ndn.MatchedRules = nil
		return
	}
	seq := j.nextSeq()
	msg := adproto.ProcessActivityNode{
		NetworkDevices: []*adproto.NetworkDeviceNode{
			{
				Netns:        ndn.Context.NetNS,
				Ifindex:      ndn.Context.IfIndex,
				MatchedRules: newRules,
			},
		},
	}
	if j.writeFrame(journalKindNetworkDevice, seq, j.currentProcID, 0, &msg) {
		ndn.MatchedRules = nil
	}
}

func (pn *ProcessNode) stripForJournal() {
	pn.Process.Argv = nil
	pn.Process.Envs = nil
	pn.Process.Credentials = model.Credentials{}
	pn.Process.CGroup = model.CGroupContext{}
	pn.Process.TTYName = ""
	pn.Process.Comm = ""
	pn.Process.FileEvent = model.FileEvent{
		PathnameStr: stringInterner.Deduplicate(pn.Process.FileEvent.PathnameStr),
		BasenameStr: stringInterner.Deduplicate(pn.Process.FileEvent.BasenameStr),
		FileFields: model.FileFields{
			PathKey: pn.Process.FileEvent.PathKey,
		},
	}
	pn.MatchedRules = nil
}

// withMaterializedTree hydrates the stripped fields of at from the on-disk journal, runs fn,
// compacts the journal if dirty, and strips the heavy fields back out of RAM.
func (j *TreeJournal) withMaterializedTree(at *ActivityTree, fn func()) {
	if j == nil || at == nil {
		fn()
		return
	}
	j.Materialize(at)
	defer func() {
		if j.dirty && !j.failed {
			j.Checkpoint(at)
		}
		if !j.failed {
			j.StripTree(at)
		}
	}()
	fn()
}

// Materialize replays the on-disk journal into the live nodes of at.
func (j *TreeJournal) Materialize(at *ActivityTree) {
	if j == nil || at == nil || j.file == nil {
		return
	}
	if err := j.buf.Flush(); err != nil {
		return
	}
	if _, err := j.file.Seek(0, io.SeekStart); err != nil {
		return
	}

	liveProcs := make(map[uint32]*ProcessNode)
	var collectProcs func(nodes []*ProcessNode)
	collectProcs = func(nodes []*ProcessNode) {
		for _, pn := range nodes {
			if pn == nil || pn.SeenIsEmpty() {
				continue
			}
			if pn.journalID != 0 {
				liveProcs[pn.journalID] = pn
			}
			collectProcs(pn.Children)
		}
	}
	collectProcs(at.ProcessNodes)

	bestOrigPath := make(map[*FileNode]string)
	reader := bufio.NewReaderSize(j.file, journalDefaultBufSize)
	var hdr [journalHeaderSize]byte
	var payloadBuf []byte
	var protoNode adproto.ProcessActivityNode

	for {
		if _, err := io.ReadFull(reader, hdr[:]); err != nil {
			break
		}
		kind := hdr[0]
		seq := binary.LittleEndian.Uint32(hdr[1:5])
		procID := binary.LittleEndian.Uint32(hdr[5:9])
		subID := binary.LittleEndian.Uint32(hdr[9:13])
		payloadLen := int(binary.LittleEndian.Uint32(hdr[13:17]))

		pn, ok := liveProcs[procID]
		if !ok || (pn.minJournalSeq > 0 && seq < pn.minJournalSeq) {
			if _, err := io.CopyN(io.Discard, reader, int64(payloadLen)); err != nil {
				break
			}
			continue
		}

		if cap(payloadBuf) < payloadLen {
			payloadBuf = make([]byte, payloadLen)
		} else {
			payloadBuf = payloadBuf[:payloadLen]
		}
		if _, err := io.ReadFull(reader, payloadBuf); err != nil {
			break
		}

		protoNode.Reset()
		if err := protoNode.UnmarshalVT(payloadBuf); err != nil {
			continue
		}

		switch kind {
		case journalKindProcess:
			if protoNode.Process != nil {
				liveExitTime := pn.Process.ExitTime
				liveIsExecExec := pn.Process.IsExecExec
				liveIsThread := pn.Process.IsThread
				livePid := pn.Process.Pid
				liveTid := pn.Process.Tid
				decoded := protoDecodeProcessNode(protoNode.Process)
				if !liveExitTime.IsZero() {
					decoded.ExitTime = liveExitTime
				}
				decoded.IsExecExec = decoded.IsExecExec || liveIsExecExec
				decoded.IsThread = liveIsThread
				if livePid != 0 {
					decoded.Pid = livePid
				}
				if liveTid != 0 {
					decoded.Tid = liveTid
				}
				pn.Process = decoded
			}
			for _, r := range protoNode.MatchedRules {
				pn.MatchedRules = model.AppendMatchedRule(pn.MatchedRules, []*model.MatchedRule{protoDecodeProtoMatchedRule(r)})
			}

		case journalKindFile:
			for _, fan := range protoNode.Files {
				if fan == nil {
					continue
				}
				isDir := subID == 1 || (fan.File != nil && (fan.File.Mode&0170000) == 0040000)
				fn, resolvedPath := resolveLiveFileNode(pn.Files, fan.Name, !isDir)
				if fn == nil || fn.SeenIsEmpty() || (fn.minJournalSeq > 0 && seq < fn.minJournalSeq) {
					continue
				}
				if fan.File != nil && fn.hasJournalFile {
					prevBest, hasBest := bestOrigPath[fn]
					if fn.File == nil || !hasBest || fan.Name < prevBest {
						fn.File = newFileInfo(protoDecodeFileEvent(fan.File))
						fn.File.PathnameStr = resolvedPath
						fn.File.BasenameStr = fn.Name
						bestOrigPath[fn] = fan.Name
					}
				}
				for _, r := range fan.MatchedRules {
					fn.MatchedRules = model.AppendMatchedRule(fn.MatchedRules, []*model.MatchedRule{protoDecodeProtoMatchedRule(r)})
				}
			}

		case journalKindDNS:
			for _, dProto := range protoNode.DnsNames {
				if dProto == nil || len(dProto.Requests) == 0 {
					continue
				}
				reqProto := dProto.Requests[0]
				dn, ok := pn.DNSNames[reqProto.Name]
				if !ok || dn == nil || dn.SeenIsEmpty() || (dn.minJournalSeq > 0 && seq < dn.minJournalSeq) {
					continue
				}
				for _, r := range dProto.MatchedRules {
					dn.MatchedRules = model.AppendMatchedRule(dn.MatchedRules, []*model.MatchedRule{protoDecodeProtoMatchedRule(r)})
				}
				if reqProto.Response != nil {
					decodedResp := protoDecodeDNSResponse(reqProto.Response)
					if decodedResp != nil {
						for i := range dn.Requests {
							if uint32(dn.Requests[i].Question.Type) == reqProto.Type {
								if dn.Requests[i].Response == nil {
									dn.Requests[i].Response = &DNSResponseAggregate{}
								}
								mergeDNSResponseAggregate(dn.Requests[i].Response, decodedResp)
								break
							}
						}
					}
				}
			}

		case journalKindIMDS:
			for _, imdsProto := range protoNode.ImdsEvents {
				if imdsProto == nil {
					continue
				}
				decoded := protoDecodeIMDSNode(imdsProto, nil)
				if decoded == nil {
					continue
				}
				if in, ok := pn.IMDSEvents[decoded.Event]; ok && in != nil && !in.SeenIsEmpty() {
					if in.minJournalSeq == 0 || seq >= in.minJournalSeq {
						in.MatchedRules = model.AppendMatchedRule(in.MatchedRules, decoded.MatchedRules)
					}
				}
			}

		case journalKindSocket:
			for _, sProto := range protoNode.Sockets {
				if sProto == nil {
					continue
				}
				var sock *SocketNode
				for _, s := range pn.Sockets {
					if s.Family == sProto.Family {
						sock = s
						break
					}
				}
				if sock == nil {
					continue
				}
				for _, bProto := range sProto.Bind {
					for _, bn := range sock.Bind {
						if bn.Port == uint16(bProto.Port) && bn.IP == bProto.Ip && bn.Protocol == uint16(bProto.Protocol) {
							if bn.minJournalSeq == 0 || seq >= bn.minJournalSeq {
								for _, r := range bProto.MatchedRules {
									bn.MatchedRules = model.AppendMatchedRule(bn.MatchedRules, []*model.MatchedRule{protoDecodeProtoMatchedRule(r)})
								}
							}
							break
						}
					}
				}
				for _, cProto := range sProto.Connect {
					for _, cn := range sock.Connect {
						if cn.Port == uint16(cProto.Port) && cn.IP == cProto.Ip && cn.Protocol == uint16(cProto.Protocol) {
							if cn.minJournalSeq == 0 || seq >= cn.minJournalSeq {
								for _, r := range cProto.MatchedRules {
									cn.MatchedRules = model.AppendMatchedRule(cn.MatchedRules, []*model.MatchedRule{protoDecodeProtoMatchedRule(r)})
								}
							}
							break
						}
					}
				}
			}

		case journalKindNetworkDevice:
			for _, ndProto := range protoNode.NetworkDevices {
				if ndProto == nil {
					continue
				}
				for ctx, ndn := range pn.NetworkDevices {
					if ctx.NetNS == ndProto.Netns && ctx.IfIndex == ndProto.Ifindex {
						if ndn.minJournalSeq == 0 || seq >= ndn.minJournalSeq {
							for _, r := range ndProto.MatchedRules {
								ndn.MatchedRules = model.AppendMatchedRule(ndn.MatchedRules, []*model.MatchedRule{protoDecodeProtoMatchedRule(r)})
							}
						}
						break
					}
				}
			}
		}
	}

	_, _ = j.file.Seek(0, io.SeekEnd)
}

func mergeDNSResponseAggregate(dst, src *DNSResponseAggregate) {
	if dst == nil || src == nil {
		return
	}
	for _, ip := range src.IPs {
		if len(dst.IPs) >= maxDNSResponseIPs {
			break
		}
		if !containsIPNet(dst.IPs, ip) {
			dst.IPs = append(dst.IPs, copyIPNet(ip))
		}
	}
	for _, cname := range src.CNames {
		if len(dst.CNames) >= maxDNSResponseCNames {
			break
		}
		if !containsString(dst.CNames, cname) {
			dst.CNames = append(dst.CNames, cname)
		}
	}
}

func containsString(list []string, target string) bool {
	for _, s := range list {
		if s == target {
			return true
		}
	}
	return false
}

// resolveLiveFileNode walks origPath through the live FileNode trie, falling back to
// pattern children (or broader covering patterns) without mutating lookup-hit stats.
func resolveLiveFileNode(roots map[string]*FileNode, origPath string, isLeafFile bool) (*FileNode, string) {
	if len(roots) == 0 || origPath == "" {
		return nil, ""
	}
	currentMap := roots
	currentPath := origPath
	var currentFn *FileNode
	var resolved strings.Builder
	resolved.Grow(len(origPath))

	for {
		comp, nextIdx := ExtractFirstParent(currentPath)
		if nextIdx == 0 {
			break
		}
		lastComponent := len(currentPath) <= nextIdx+1
		child := matchChildForReplay(currentMap, comp, lastComponent && isLeafFile)
		if child == nil {
			return nil, ""
		}
		resolved.WriteByte('/')
		resolved.WriteString(child.Name)
		currentFn = child
		if lastComponent {
			break
		}
		currentMap = currentFn.Children
		currentPath = currentPath[nextIdx:]
	}
	return currentFn, resolved.String()
}

func matchChildForReplay(children map[string]*FileNode, name string, isLeafFile bool) *FileNode {
	if len(children) == 0 {
		return nil
	}
	if child, ok := children[name]; ok {
		return child
	}
	isTemplate := strings.IndexByte(name, '<') >= 0
	var narrowTmp *FileNode
	if isTemplate {
		narrowTmp = &FileNode{Name: name, IsPattern: true}
	}

	var best *FileNode
	var bestSpec int
	for _, c := range children {
		if c == nil || !c.IsPattern {
			continue
		}
		m := c.matcher()
		if isLeafFile && m.directoryOnly {
			continue
		}
		matched := m.matches(name)
		if !matched && narrowTmp != nil {
			matched = patternCovers(c, narrowTmp)
		}
		if !matched {
			continue
		}
		if best == nil || m.specificity > bestSpec || (m.specificity == bestSpec && c.Name < best.Name) {
			best = c
			bestSpec = m.specificity
		}
	}
	return best
}

// Checkpoint atomically rewrites the journal file from the currently materialized ActivityTree,
// discarding any evicted nodes and compacting merged pattern siblings into single records.
func (j *TreeJournal) Checkpoint(at *ActivityTree) {
	if j == nil || at == nil || j.path == "" {
		return
	}
	tmpPath := j.path + ".tmp"
	tmpFile, err := os.OpenFile(tmpPath, os.O_CREATE|os.O_RDWR|os.O_TRUNC, 0600)
	if err != nil {
		return
	}

	oldFile := j.file
	j.file = tmpFile
	j.buf.Reset(tmpFile)
	j.writtenBytes = 0
	j.seq = 0
	j.failed = false
	clear(j.enrichmentHashes)

	var writeProc func(pn *ProcessNode)
	writeProc = func(pn *ProcessNode) {
		if pn == nil || pn.SeenIsEmpty() {
			return
		}
		if pn.journalID == 0 {
			pn.journalID = j.allocProcID()
		}
		seq := j.nextSeq()
		pn.minJournalSeq = seq
		msg := adproto.ProcessActivityNode{
			Process:      processNodeToProto(&pn.Process),
			MatchedRules: j.filterNewRules(pn.journalID, pn.minJournalSeq, "proc_rule", "", pn.MatchedRules),
		}
		j.writeFrame(journalKindProcess, seq, pn.journalID, 0, &msg)

		var writeFiles func(fn *FileNode, prefix string)
		writeFiles = func(fn *FileNode, prefix string) {
			if fn == nil || (fn.SeenIsEmpty() && len(fn.Children) == 0) {
				return
			}
			fullPath := prefix + "/" + fn.Name
			fSeq := j.nextSeq()
			fn.minJournalSeq = fSeq
			if fn.File != nil || len(fn.MatchedRules) > 0 {
				var subID uint32
				if fn.File != nil && fn.File.IsDir() {
					subID = 1
				}
				fMsg := adproto.ProcessActivityNode{
					Files: []*adproto.FileActivityNode{
						{
							Name:         fullPath,
							File:         fileInfoToProto(fn.File),
							MatchedRules: j.filterNewRules(pn.journalID, fn.minJournalSeq, "file_rule", fullPath, fn.MatchedRules),
						},
					},
				}
				if j.writeFrame(journalKindFile, fSeq, pn.journalID, subID, &fMsg) && fn.File != nil {
					fn.hasJournalFile = true
				}
			}
			for _, child := range fn.Children {
				writeFiles(child, fullPath)
			}
		}
		for _, fn := range pn.Files {
			writeFiles(fn, "")
		}

		for _, dn := range pn.DNSNames {
			if dn == nil || dn.SeenIsEmpty() {
				continue
			}
			dn.minJournalSeq = j.nextSeq()
			for i := range dn.Requests {
				req := &dn.Requests[i]
				req.journalIPCount = 0
				req.journalCNameCount = 0
				var rules []*model.MatchedRule
				if i == 0 {
					rules = dn.MatchedRules
				}
				var resp *model.DNSResponse
				if req.Response != nil {
					resp = &model.DNSResponse{
						IPs:    req.Response.IPs,
						CNames: req.Response.CNames,
					}
				}
				j.recordDNSEnrichment(pn, dn, i, resp, rules)
			}
		}

		for _, in := range pn.IMDSEvents {
			if in == nil || in.SeenIsEmpty() {
				continue
			}
			in.minJournalSeq = j.nextSeq()
			if len(in.MatchedRules) > 0 {
				j.recordIMDSRules(pn, in, in.MatchedRules)
			}
		}

		j.currentProcID = pn.journalID
		for _, sock := range pn.Sockets {
			if sock == nil {
				continue
			}
			for _, bn := range sock.Bind {
				if bn != nil && len(bn.MatchedRules) > 0 {
					j.recordSocketRules(sock.Family, false, bn.Port, bn.IP, bn.Protocol, &bn.minJournalSeq, bn.MatchedRules)
				}
			}
			for _, cn := range sock.Connect {
				if cn != nil && len(cn.MatchedRules) > 0 {
					j.recordSocketRules(sock.Family, true, cn.Port, cn.IP, cn.Protocol, &cn.minJournalSeq, cn.MatchedRules)
				}
			}
		}

		for _, ndn := range pn.NetworkDevices {
			if ndn != nil && len(ndn.MatchedRules) > 0 {
				j.recordNetworkDeviceRules(ndn, ndn.MatchedRules)
			}
		}

		for _, child := range pn.Children {
			writeProc(child)
		}
	}

	for _, pn := range at.ProcessNodes {
		writeProc(pn)
	}

	if err := j.buf.Flush(); err != nil || j.failed {
		_ = tmpFile.Close()
		_ = os.Remove(tmpPath)
		j.file = oldFile
		if oldFile != nil {
			j.buf.Reset(oldFile)
		}
		return
	}
	if oldFile != nil {
		_ = oldFile.Close()
	}
	if err := os.Rename(tmpPath, j.path); err != nil {
		return
	}
	j.dirty = false
}

// StripTree strips all journaled heavy fields from the live nodes of at and recomputes Stats.SizeBytes.
func (j *TreeJournal) StripTree(at *ActivityTree) {
	if j == nil || j.failed || at == nil {
		return
	}
	var stripFiles func(fn *FileNode)
	stripFiles = func(fn *FileNode) {
		if fn == nil {
			return
		}
		if fn.File != nil {
			fn.hasJournalFile = true
			fn.File = nil
		}
		fn.MatchedRules = nil
		for _, child := range fn.Children {
			stripFiles(child)
		}
	}

	var stripProc func(pn *ProcessNode)
	stripProc = func(pn *ProcessNode) {
		if pn == nil {
			return
		}
		pn.stripForJournal()
		for _, fn := range pn.Files {
			stripFiles(fn)
		}
		for _, dn := range pn.DNSNames {
			if dn == nil {
				continue
			}
			dn.MatchedRules = nil
			for i := range dn.Requests {
				dn.Requests[i].Response = nil
			}
		}
		for _, in := range pn.IMDSEvents {
			if in != nil {
				in.MatchedRules = nil
			}
		}
		for _, sock := range pn.Sockets {
			if sock == nil {
				continue
			}
			for _, bn := range sock.Bind {
				if bn != nil {
					bn.MatchedRules = nil
				}
			}
			for _, cn := range sock.Connect {
				if cn != nil {
					cn.MatchedRules = nil
				}
			}
		}
		for _, ndn := range pn.NetworkDevices {
			if ndn != nil {
				ndn.MatchedRules = nil
			}
		}
		for _, child := range pn.Children {
			stripProc(child)
		}
	}

	for _, pn := range at.ProcessNodes {
		stripProc(pn)
	}
	at.recomputeSizeBytes()
}

func hashProcessArgs(argv []string) uint64 {
	h := fnv.New64a()
	var lenBuf [4]byte
	binary.LittleEndian.PutUint32(lenBuf[:], uint32(len(argv)))
	_, _ = h.Write(lenBuf[:])
	for _, arg := range argv {
		binary.LittleEndian.PutUint32(lenBuf[:], uint32(len(arg)))
		_, _ = h.Write(lenBuf[:])
		_, _ = h.Write([]byte(arg))
	}
	return h.Sum64()
}
