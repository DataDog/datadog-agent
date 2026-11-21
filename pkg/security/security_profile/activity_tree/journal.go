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
	"github.com/DataDog/datadog-agent/pkg/security/seclog"
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
	journalDefaultMaxSize = 16 << 20 // 16 MiB default cap per profile journal
)

// TreeJournal spills cold, high-footprint node payloads (process argv/envs/hashes/credentials,
// FileInfo structs and strings, DNS response IP/CNAME aggregates, and MatchedRules) to a
// compact binary protobuf journal file on disk while keeping the hot deduplication skeleton,
// pattern-matching trie, image-tag timestamps, and eBPF sample cookie targets in RAM.
type TreeJournal struct {
	path          string
	file          *os.File
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
		seclog.Warnf("couldn't enable activity tree journal for %s: %v", key, err)
		return err
	}
	at.Stats.journal = j
	if len(at.ProcessNodes) > 0 {
		if err := j.Checkpoint(at); err != nil {
			_ = j.Close()
			at.Stats.journal = nil
			return err
		}
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
	_ = at.Stats.journal.Materialize(at)
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

func (j *TreeJournal) fail(reason string) {
	if j == nil || j.failed {
		return
	}
	j.failed = true
	seclog.Warnf("activity tree journal [%s] disabled, falling back to in-memory payloads: %s", j.path, reason)
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
		j.fail(fmt.Sprintf("journal size cap (%d bytes) reached", j.maxBytes))
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
		j.fail(fmt.Sprintf("marshal error: %v", err))
		return false
	}
	if _, err := j.file.Write(j.scratchBuf); err != nil {
		j.fail(fmt.Sprintf("write error: %v", err))
		return false
	}
	j.writtenBytes += frameBytes
	return true
}

func computeEnrichmentHash(procID, minSeq uint32, category, key, subKey string) uint64 {
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
	return h.Sum64()
}

func ruleSubKey(r *model.MatchedRule) string {
	return r.RuleID + "\x00" + r.RuleVersion + "\x00" + r.PolicyName + "\x00" + r.PolicyVersion
}

func (j *TreeJournal) hasEnrichment(sum uint64) bool {
	_, exists := j.enrichmentHashes[sum]
	return exists
}

func (j *TreeJournal) commitEnrichments(hashes []uint64) {
	for _, sum := range hashes {
		j.enrichmentHashes[sum] = struct{}{}
	}
}

func (j *TreeJournal) filterNewRules(procID, minSeq uint32, category, key string, rules []*model.MatchedRule) ([]*adproto.MatchedRule, []uint64) {
	if len(rules) == 0 {
		return nil, nil
	}
	var out []*adproto.MatchedRule
	var hashes []uint64
	for _, r := range rules {
		if r == nil {
			continue
		}
		sum := computeEnrichmentHash(procID, minSeq, category, key, ruleSubKey(r))
		if j.hasEnrichment(sum) || containsUint64(hashes, sum) {
			continue
		}
		hashes = append(hashes, sum)
		out = append(out, matchedRuleToProto(r))
	}
	return out, hashes
}

func containsUint64(list []uint64, target uint64) bool {
	for _, v := range list {
		if v == target {
			return true
		}
	}
	return false
}

func (j *TreeJournal) recordProcessNode(pn *ProcessNode) {
	if j == nil || j.failed || pn == nil {
		return
	}
	if pn.journalID == 0 {
		pn.journalID = j.allocProcID()
	}
	seq := j.nextSeq()
	minSeq := pn.minJournalSeq
	if minSeq == 0 {
		minSeq = seq
	}
	newRules, hashes := j.filterNewRules(pn.journalID, minSeq, "proc_rule", "", pn.MatchedRules)
	msg := adproto.ProcessActivityNode{
		Process:      processNodeToProto(&pn.Process),
		MatchedRules: newRules,
	}
	if j.writeFrame(journalKindProcess, seq, pn.journalID, 0, &msg) {
		pn.minJournalSeq = minSeq
		j.commitEnrichments(hashes)
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
		if j.failed || pn.journalID == 0 {
			pn.MatchedRules = model.AppendMatchedRule(pn.MatchedRules, rules)
			return
		}
	}
	newRules, hashes := j.filterNewRules(pn.journalID, pn.minJournalSeq, "proc_rule", "", rules)
	if len(newRules) == 0 {
		return
	}
	seq := j.nextSeq()
	msg := adproto.ProcessActivityNode{
		MatchedRules: newRules,
	}
	if j.writeFrame(journalKindProcess, seq, pn.journalID, 0, &msg) {
		j.commitEnrichments(hashes)
	} else {
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
	minSeq := fn.minJournalSeq
	if minSeq == 0 {
		minSeq = seq
	}
	var subID uint32
	if fn.File != nil && fn.File.IsDir() {
		subID = 1
	}
	newRules, hashes := j.filterNewRules(j.currentProcID, minSeq, "file_rule", reducedPath, fn.MatchedRules)
	msg := adproto.ProcessActivityNode{
		Files: []*adproto.FileActivityNode{
			{
				Name:         reducedPath,
				File:         fileInfoToProto(fn.File),
				MatchedRules: newRules,
			},
		},
	}
	if j.writeFrame(journalKindFile, seq, j.currentProcID, subID, &msg) {
		fn.minJournalSeq = minSeq
		j.commitEnrichments(hashes)
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
	minSeq := fn.minJournalSeq
	if minSeq == 0 {
		minSeq = j.nextSeq()
	}
	newRules, hashes := j.filterNewRules(j.currentProcID, minSeq, "file_rule", reducedPath, rules)
	if len(newRules) == 0 {
		fn.minJournalSeq = minSeq
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
	if j.writeFrame(journalKindFile, seq, j.currentProcID, 0, &msg) {
		fn.minJournalSeq = minSeq
		j.commitEnrichments(hashes)
	} else {
		fn.MatchedRules = model.AppendMatchedRule(fn.MatchedRules, rules)
	}
}

func (j *TreeJournal) recordDNSEnrichment(pn *ProcessNode, dn *DNSNode, qIdx int, resp *model.DNSResponse, rules []*model.MatchedRule) bool {
	if j == nil || j.failed || pn == nil || dn == nil || qIdx < 0 || qIdx >= len(dn.Requests) {
		return false
	}
	j.setCurrentProc(pn)
	if j.failed || pn.journalID == 0 {
		return false
	}
	minSeq := dn.minJournalSeq
	if minSeq == 0 {
		minSeq = j.nextSeq()
	}
	req := &dn.Requests[qIdx]
	qKey := fmt.Sprintf("%s:%d", req.Question.Name, req.Question.Type)
	newRules, hashes := j.filterNewRules(pn.journalID, minSeq, "dns_rule", req.Question.Name, rules)

	var filteredResp *DNSResponseAggregate
	var addedIPs, addedCNames uint8
	if resp != nil {
		for _, ip := range resp.IPs {
			if req.journalIPCount+addedIPs >= maxDNSResponseIPs {
				break
			}
			sum := computeEnrichmentHash(pn.journalID, minSeq, "dns_ip", qKey, ip.String())
			if !j.hasEnrichment(sum) && !containsUint64(hashes, sum) {
				hashes = append(hashes, sum)
				if filteredResp == nil {
					filteredResp = &DNSResponseAggregate{}
				}
				filteredResp.IPs = append(filteredResp.IPs, copyIPNet(ip))
				addedIPs++
			}
		}
		for _, cname := range resp.CNames {
			if req.journalCNameCount+addedCNames >= maxDNSResponseCNames {
				break
			}
			sum := computeEnrichmentHash(pn.journalID, minSeq, "dns_cname", qKey, cname)
			if !j.hasEnrichment(sum) && !containsUint64(hashes, sum) {
				hashes = append(hashes, sum)
				if filteredResp == nil {
					filteredResp = &DNSResponseAggregate{}
				}
				filteredResp.CNames = append(filteredResp.CNames, cname)
				addedCNames++
			}
		}
	}

	if filteredResp == nil && len(newRules) == 0 {
		dn.minJournalSeq = minSeq
		req.Response = nil
		dn.MatchedRules = nil
		return true
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
	if !j.writeFrame(journalKindDNS, seq, pn.journalID, uint32(qIdx), &msg) {
		return false
	}
	dn.minJournalSeq = minSeq
	j.commitEnrichments(hashes)
	req.journalIPCount += addedIPs
	req.journalCNameCount += addedCNames
	req.Response = nil
	dn.MatchedRules = nil
	return true
}

func (j *TreeJournal) recordIMDSRules(pn *ProcessNode, in *IMDSNode, rules []*model.MatchedRule) {
	if j == nil || j.failed || pn == nil || in == nil || len(rules) == 0 {
		if in != nil && len(rules) > 0 {
			in.MatchedRules = model.AppendMatchedRule(in.MatchedRules, rules)
		}
		return
	}
	j.setCurrentProc(pn)
	if j.failed || pn.journalID == 0 {
		in.MatchedRules = model.AppendMatchedRule(in.MatchedRules, rules)
		return
	}
	minSeq := in.minJournalSeq
	if minSeq == 0 {
		minSeq = j.nextSeq()
	}
	key := in.Event.URL + "|" + in.Event.AWS.SecurityCredentials.AccessKeyID
	newRules, hashes := j.filterNewRules(pn.journalID, minSeq, "imds_rule", key, rules)
	if len(newRules) == 0 {
		in.minJournalSeq = minSeq
		in.MatchedRules = nil
		return
	}
	seq := j.nextSeq()
	msg := adproto.ProcessActivityNode{
		ImdsEvents: []*adproto.IMDSNode{
			{
				Event:        imdsEventToProto(in.Event),
				MatchedRules: newRules,
			},
		},
	}
	if j.writeFrame(journalKindIMDS, seq, pn.journalID, 0, &msg) {
		in.minJournalSeq = minSeq
		j.commitEnrichments(hashes)
		in.MatchedRules = nil
	} else {
		in.MatchedRules = model.AppendMatchedRule(in.MatchedRules, rules)
	}
}

func (j *TreeJournal) recordSocketRules(family string, isConnect bool, port uint16, ip string, protocol uint16, minSeq *uint32, rules []*model.MatchedRule) bool {
	if j == nil || j.failed || j.currentProcID == 0 || len(rules) == 0 {
		return false
	}
	curMinSeq := *minSeq
	if curMinSeq == 0 {
		curMinSeq = j.nextSeq()
	}
	key := fmt.Sprintf("%s:%v:%s:%d:%d", family, isConnect, ip, port, protocol)
	newRules, hashes := j.filterNewRules(j.currentProcID, curMinSeq, "sock_rule", key, rules)
	if len(newRules) == 0 {
		*minSeq = curMinSeq
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
	if !j.writeFrame(journalKindSocket, seq, j.currentProcID, subID, &msg) {
		return false
	}
	*minSeq = curMinSeq
	j.commitEnrichments(hashes)
	return true
}

func (j *TreeJournal) recordNetworkDeviceRules(ndn *NetworkDeviceNode, rules []*model.MatchedRule) {
	if j == nil || j.failed || ndn == nil || j.currentProcID == 0 || len(rules) == 0 {
		if ndn != nil && len(rules) > 0 {
			ndn.MatchedRules = model.AppendMatchedRule(ndn.MatchedRules, rules)
		}
		return
	}
	minSeq := ndn.minJournalSeq
	if minSeq == 0 {
		minSeq = j.nextSeq()
	}
	key := fmt.Sprintf("%d:%d", ndn.Context.NetNS, ndn.Context.IfIndex)
	newRules, hashes := j.filterNewRules(j.currentProcID, minSeq, "netdev_rule", key, rules)
	if len(newRules) == 0 {
		ndn.minJournalSeq = minSeq
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
		ndn.minJournalSeq = minSeq
		j.commitEnrichments(hashes)
		ndn.MatchedRules = nil
	} else {
		ndn.MatchedRules = model.AppendMatchedRule(ndn.MatchedRules, rules)
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
	matErr := j.Materialize(at)
	defer func() {
		if matErr != nil {
			j.fail(fmt.Sprintf("materialize failed: %v", matErr))
			at.recomputeSizeBytes()
			return
		}
		if j.dirty && !j.failed {
			if err := j.Checkpoint(at); err != nil {
				j.fail(fmt.Sprintf("checkpoint failed: %v", err))
				at.recomputeSizeBytes()
				return
			}
		}
		if !j.failed {
			j.StripTree(at)
		} else {
			at.recomputeSizeBytes()
		}
	}()
	fn()
}

// Materialize replays the on-disk journal into the live nodes of at.
func (j *TreeJournal) Materialize(at *ActivityTree) error {
	if j == nil || at == nil || j.file == nil {
		return nil
	}
	if _, err := j.file.Seek(0, io.SeekStart); err != nil {
		return err
	}

	liveProcs := make(map[uint32]*ProcessNode)
	var collectProcs func(nodes []*ProcessNode)
	collectProcs = func(nodes []*ProcessNode) {
		for _, pn := range nodes {
			if pn == nil {
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
			if errors.Is(err, io.EOF) {
				break
			}
			return err
		}
		kind := hdr[0]
		seq := binary.LittleEndian.Uint32(hdr[1:5])
		procID := binary.LittleEndian.Uint32(hdr[5:9])
		subID := binary.LittleEndian.Uint32(hdr[9:13])
		payloadLen := int(binary.LittleEndian.Uint32(hdr[13:17]))
		if payloadLen < 0 || int64(payloadLen) > j.maxBytes {
			return fmt.Errorf("corrupt journal frame length: %d", payloadLen)
		}

		pn, ok := liveProcs[procID]
		if !ok || (pn.minJournalSeq > 0 && seq < pn.minJournalSeq) {
			if _, err := io.CopyN(io.Discard, reader, int64(payloadLen)); err != nil {
				return err
			}
			continue
		}

		if cap(payloadBuf) < payloadLen {
			payloadBuf = make([]byte, payloadLen)
		} else {
			payloadBuf = payloadBuf[:payloadLen]
		}
		if _, err := io.ReadFull(reader, payloadBuf); err != nil {
			return err
		}

		protoNode.Reset()
		if err := protoNode.UnmarshalVT(payloadBuf); err != nil {
			return err
		}

		switch kind {
		case journalKindProcess:
			if protoNode.Process != nil {
				liveExitTime := pn.Process.ExitTime
				liveIsExecExec := pn.Process.IsExecExec
				liveIsThread := pn.Process.IsThread
				livePid := pn.Process.Pid
				liveTid := pn.Process.Tid
				livePath := pn.Process.FileEvent.PathnameStr
				liveBase := pn.Process.FileEvent.BasenameStr
				livePathKey := pn.Process.FileEvent.PathKey
				liveArgv0 := pn.Process.Argv0
				liveArgsHash := pn.Process.argsHash
				liveArgsHashed := pn.Process.argsHashed
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
				if livePath != "" {
					decoded.FileEvent.PathnameStr = livePath
				}
				if liveBase != "" {
					decoded.FileEvent.BasenameStr = liveBase
				}
				if livePathKey.Inode != 0 {
					decoded.FileEvent.PathKey = livePathKey
				}
				if liveArgv0 != "" {
					decoded.Argv0 = liveArgv0
				}
				if liveArgsHashed {
					decoded.argsHash = liveArgsHash
					decoded.argsHashed = true
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
				if fn == nil || (fn.minJournalSeq > 0 && seq < fn.minJournalSeq) {
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
				if !ok || dn == nil || (dn.minJournalSeq > 0 && seq < dn.minJournalSeq) {
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
				decodedEvent := protoDecodeIMDSEvent(imdsProto.Event)
				if in, ok := pn.IMDSEvents[decodedEvent]; ok && in != nil {
					if in.minJournalSeq == 0 || seq >= in.minJournalSeq {
						for _, r := range imdsProto.MatchedRules {
							in.MatchedRules = model.AppendMatchedRule(in.MatchedRules, []*model.MatchedRule{protoDecodeProtoMatchedRule(r)})
						}
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

	_, err := j.file.Seek(0, io.SeekEnd)
	return err
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
func (j *TreeJournal) Checkpoint(at *ActivityTree) error {
	if j == nil || at == nil || j.path == "" {
		return nil
	}
	tmpPath := j.path + ".tmp"
	tmpFile, err := os.OpenFile(tmpPath, os.O_CREATE|os.O_RDWR|os.O_TRUNC, 0600)
	if err != nil {
		return err
	}

	staged := &TreeJournal{
		path:             tmpPath,
		file:             tmpFile,
		nextProcID:       j.nextProcID,
		maxBytes:         j.maxBytes,
		scratchBuf:       j.scratchBuf,
		enrichmentHashes: make(map[uint64]struct{}),
	}
	var commits []func()

	var writeProc func(pn *ProcessNode)
	writeProc = func(pn *ProcessNode) {
		if pn == nil || staged.failed {
			return
		}
		procID := pn.journalID
		if procID == 0 {
			procID = staged.allocProcID()
		}
		seq := staged.nextSeq()
		newRules, hashes := staged.filterNewRules(procID, seq, "proc_rule", "", pn.MatchedRules)
		msg := adproto.ProcessActivityNode{
			Process:      processNodeToProto(&pn.Process),
			MatchedRules: newRules,
		}
		if !staged.writeFrame(journalKindProcess, seq, procID, 0, &msg) {
			return
		}
		staged.commitEnrichments(hashes)
		pNode := pn
		commits = append(commits, func() {
			pNode.journalID = procID
			pNode.minJournalSeq = seq
		})

		var writeFiles func(fn *FileNode, prefix string)
		writeFiles = func(fn *FileNode, prefix string) {
			if fn == nil || staged.failed {
				return
			}
			fullPath := prefix + "/" + fn.Name
			fSeq := staged.nextSeq()
			hasFile := fn.File != nil
			if hasFile || len(fn.MatchedRules) > 0 {
				var subID uint32
				if fn.File != nil && fn.File.IsDir() {
					subID = 1
				}
				fRules, fHashes := staged.filterNewRules(procID, fSeq, "file_rule", fullPath, fn.MatchedRules)
				fMsg := adproto.ProcessActivityNode{
					Files: []*adproto.FileActivityNode{
						{
							Name:         fullPath,
							File:         fileInfoToProto(fn.File),
							MatchedRules: fRules,
						},
					},
				}
				if !staged.writeFrame(journalKindFile, fSeq, procID, subID, &fMsg) {
					return
				}
				staged.commitEnrichments(fHashes)
			}
			fNode := fn
			commits = append(commits, func() {
				fNode.minJournalSeq = fSeq
				if hasFile {
					fNode.hasJournalFile = true
				}
			})
			for _, child := range fn.Children {
				writeFiles(child, fullPath)
			}
		}
		for _, fn := range pn.Files {
			writeFiles(fn, "")
		}

		for _, dn := range pn.DNSNames {
			if dn == nil || staged.failed {
				continue
			}
			dSeq := staged.nextSeq()
			dNode := dn
			commits = append(commits, func() {
				dNode.minJournalSeq = dSeq
			})
			for i := range dn.Requests {
				req := &dn.Requests[i]
				qKey := fmt.Sprintf("%s:%d", req.Question.Name, req.Question.Type)
				var rules []*model.MatchedRule
				if i == 0 {
					rules = dn.MatchedRules
				}
				dRules, dHashes := staged.filterNewRules(procID, dSeq, "dns_rule", req.Question.Name, rules)
				var filteredResp *DNSResponseAggregate
				var addedIPs, addedCNames uint8
				if req.Response != nil {
					for _, ip := range req.Response.IPs {
						if addedIPs >= maxDNSResponseIPs {
							break
						}
						sum := computeEnrichmentHash(procID, dSeq, "dns_ip", qKey, ip.String())
						if !staged.hasEnrichment(sum) && !containsUint64(dHashes, sum) {
							dHashes = append(dHashes, sum)
							if filteredResp == nil {
								filteredResp = &DNSResponseAggregate{}
							}
							filteredResp.IPs = append(filteredResp.IPs, copyIPNet(ip))
							addedIPs++
						}
					}
					for _, cname := range req.Response.CNames {
						if addedCNames >= maxDNSResponseCNames {
							break
						}
						sum := computeEnrichmentHash(procID, dSeq, "dns_cname", qKey, cname)
						if !staged.hasEnrichment(sum) && !containsUint64(dHashes, sum) {
							dHashes = append(dHashes, sum)
							if filteredResp == nil {
								filteredResp = &DNSResponseAggregate{}
							}
							filteredResp.CNames = append(filteredResp.CNames, cname)
							addedCNames++
						}
					}
				}
				if filteredResp != nil || len(dRules) > 0 {
					rSeq := staged.nextSeq()
					dMsg := adproto.ProcessActivityNode{
						DnsNames: []*adproto.DNSNode{
							{
								MatchedRules: dRules,
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
					if !staged.writeFrame(journalKindDNS, rSeq, procID, uint32(i), &dMsg) {
						return
					}
					staged.commitEnrichments(dHashes)
				}
				rPtr := req
				ipCnt := addedIPs
				cnCnt := addedCNames
				commits = append(commits, func() {
					rPtr.journalIPCount = ipCnt
					rPtr.journalCNameCount = cnCnt
				})
			}
		}

		for _, in := range pn.IMDSEvents {
			if in == nil || staged.failed {
				continue
			}
			iSeq := staged.nextSeq()
			if len(in.MatchedRules) > 0 {
				key := in.Event.URL + "|" + in.Event.AWS.SecurityCredentials.AccessKeyID
				iRules, iHashes := staged.filterNewRules(procID, iSeq, "imds_rule", key, in.MatchedRules)
				if len(iRules) > 0 {
					rSeq := staged.nextSeq()
					iMsg := adproto.ProcessActivityNode{
						ImdsEvents: []*adproto.IMDSNode{
							{
								Event:        imdsEventToProto(in.Event),
								MatchedRules: iRules,
							},
						},
					}
					if !staged.writeFrame(journalKindIMDS, rSeq, procID, 0, &iMsg) {
						return
					}
					staged.commitEnrichments(iHashes)
				}
			}
			iNode := in
			commits = append(commits, func() {
				iNode.minJournalSeq = iSeq
			})
		}

		staged.currentProcID = procID
		for _, sock := range pn.Sockets {
			if sock == nil || staged.failed {
				continue
			}
			for _, bn := range sock.Bind {
				if bn == nil {
					continue
				}
				var bSeq uint32
				if len(bn.MatchedRules) > 0 {
					if !staged.recordSocketRules(sock.Family, false, bn.Port, bn.IP, bn.Protocol, &bSeq, bn.MatchedRules) {
						return
					}
				} else {
					bSeq = staged.nextSeq()
				}
				bNode := bn
				commits = append(commits, func() {
					bNode.minJournalSeq = bSeq
				})
			}
			for _, cn := range sock.Connect {
				if cn == nil {
					continue
				}
				var cSeq uint32
				if len(cn.MatchedRules) > 0 {
					if !staged.recordSocketRules(sock.Family, true, cn.Port, cn.IP, cn.Protocol, &cSeq, cn.MatchedRules) {
						return
					}
				} else {
					cSeq = staged.nextSeq()
				}
				cNode := cn
				commits = append(commits, func() {
					cNode.minJournalSeq = cSeq
				})
			}
		}

		for _, ndn := range pn.NetworkDevices {
			if ndn == nil || staged.failed {
				continue
			}
			ndSeq := staged.nextSeq()
			if len(ndn.MatchedRules) > 0 {
				key := fmt.Sprintf("%d:%d", ndn.Context.NetNS, ndn.Context.IfIndex)
				ndRules, ndHashes := staged.filterNewRules(procID, ndSeq, "netdev_rule", key, ndn.MatchedRules)
				if len(ndRules) > 0 {
					rSeq := staged.nextSeq()
					ndMsg := adproto.ProcessActivityNode{
						NetworkDevices: []*adproto.NetworkDeviceNode{
							{
								Netns:        ndn.Context.NetNS,
								Ifindex:      ndn.Context.IfIndex,
								MatchedRules: ndRules,
							},
						},
					}
					if !staged.writeFrame(journalKindNetworkDevice, rSeq, procID, 0, &ndMsg) {
						return
					}
					staged.commitEnrichments(ndHashes)
				}
			}
			ndNode := ndn
			commits = append(commits, func() {
				ndNode.minJournalSeq = ndSeq
			})
		}

		for _, child := range pn.Children {
			writeProc(child)
		}
	}

	for _, pn := range at.ProcessNodes {
		writeProc(pn)
	}

	if staged.failed {
		_ = tmpFile.Close()
		_ = os.Remove(tmpPath)
		return errors.New("checkpoint journal write failed")
	}
	if err := os.Rename(tmpPath, j.path); err != nil {
		_ = tmpFile.Close()
		_ = os.Remove(tmpPath)
		return err
	}
	if j.file != nil {
		_ = j.file.Close()
	}
	j.file = tmpFile
	j.seq = staged.seq
	j.nextProcID = staged.nextProcID
	j.currentProcID = staged.currentProcID
	j.writtenBytes = staged.writtenBytes
	j.enrichmentHashes = staged.enrichmentHashes
	j.scratchBuf = staged.scratchBuf
	j.dirty = false
	j.failed = false
	for _, commit := range commits {
		commit()
	}
	return nil
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
