// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package activitytree holds activitytree related files
package activitytree

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"unsafe"

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

	journalHeaderSize            = 17 // 1B kind + 4B seq + 4B procID + 4B subID + 4B payloadLen
	journalDefaultBufSize        = 16 * 1024
	journalFlushCompactThreshold = 64 * 1024
	journalSyncBatchBytes        = 64 * 1024
	journalMaxAsyncPendingBytes  = 2 << 20  // 2 MiB max in-flight async buffer
	journalDefaultMaxSize        = 16 << 20 // 16 MiB default cap per profile journal
)

var journalReaderPool = sync.Pool{
	New: func() any {
		return bufio.NewReaderSize(nil, journalDefaultBufSize)
	},
}

var journalWriteBufPool = sync.Pool{
	New: func() any {
		b := make([]byte, 0, journalDefaultBufSize)
		return &b
	},
}

// TreeJournal spills cold, high-footprint node payloads (process argv/envs/hashes/credentials,
// FileInfo structs and strings, DNS response IP/CNAME aggregates, and MatchedRules) to a
// compact binary protobuf journal file on disk while keeping the hot deduplication skeleton,
// pattern-matching trie, image-tag timestamps, and eBPF sample cookie targets in RAM.
type TreeJournal struct {
	path                    string
	file                    *os.File
	seq                     uint32
	nextProcID              uint32
	currentProcID           uint32
	writtenBytes            int64
	lastCheckpointBytes     int64
	fragmentedBytes         int64
	maxBytes                int64
	dirty                   bool
	failed                  bool
	permanentlyMaterialized bool
	materializedDepth       int
	syncOnly                bool

	ioMu          sync.Mutex
	ioCond        *sync.Cond
	diskBytes     int64
	pendingBuf    []byte
	flushingBuf   []byte
	fallbackBuf   []byte
	asyncErr      error
	writerRunning bool
	writerStop    bool
	flushing      bool

	scratchBuf       []byte
	enrichmentHashes map[uint64]struct{}
	mergedFileSeqs   map[uint32]uint32
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

	j := &TreeJournal{
		path:             journalPath,
		file:             f,
		maxBytes:         maxBytes,
		enrichmentHashes: make(map[uint64]struct{}),
		mergedFileSeqs:   make(map[uint32]uint32),
	}
	j.ioCond = sync.NewCond(&j.ioMu)
	runtime.SetFinalizer(j, (*TreeJournal).Close)
	return j, nil
}

func sanitizeJournalKey(key string) string {
	h := fnvHashString(fnvOffset64, key)
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
		return fmt.Sprintf("profile-%016x", h)
	}
	return fmt.Sprintf("%s-%016x", b.String(), h)
}

// EnableJournal attaches an on-disk binary journal to the ActivityTree.
func (at *ActivityTree) EnableJournal(dir, key string, maxBytes int64) error {
	if at == nil || at.Stats == nil {
		return nil
	}
	if at.Stats.journal != nil {
		_ = at.Stats.journal.Materialize(at)
	}
	_ = at.CloseJournal()
	j, err := newTreeJournal(dir, key, maxBytes)
	if err != nil {
		seclog.Debugf("couldn't enable activity tree journal for %s: %v", key, err)
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
	return at != nil && at.Stats != nil && at.Stats.journal != nil && !at.Stats.journal.failed && !at.Stats.journal.permanentlyMaterialized
}

// JournalSize returns the current size in bytes of the on-disk journal.
func (at *ActivityTree) JournalSize() int64 {
	if !at.HasJournal() {
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
	if err := at.Stats.journal.Materialize(at); err != nil {
		at.Stats.journal.fail(fmt.Sprintf("materialize failed: %v", err))
	}
}

// WithMaterializedTree runs fn while the ActivityTree's journaled fields are hydrated in RAM,
// then strips them back out once the outermost WithMaterializedTree call finishes.
func (at *ActivityTree) WithMaterializedTree(fn func()) {
	if at == nil || at.Stats == nil || at.Stats.journal == nil {
		fn()
		return
	}
	at.Stats.journal.withMaterializedTree(at, fn)
}

func (j *TreeJournal) heapSize() int64 {
	if j == nil {
		return 0
	}
	j.ioMu.Lock()
	ioBufCap := int64(cap(j.pendingBuf) + cap(j.flushingBuf) + cap(j.fallbackBuf))
	j.ioMu.Unlock()
	s := int64(unsafe.Sizeof(*j)) + int64(cap(j.scratchBuf)) + ioBufCap
	s += fixedKeyMapBytes(j.enrichmentHashes)
	s += fixedKeyMapBytes(j.mergedFileSeqs)
	return s
}

func (j *TreeJournal) stopAndRemoveFile() error {
	if j == nil {
		return nil
	}
	j.ioMu.Lock()
	j.writerStop = true
	if j.ioCond != nil {
		j.ioCond.Broadcast()
	}
	for j.writerRunning {
		j.ioCond.Wait()
	}
	if j.pendingBuf != nil {
		b := j.pendingBuf[:0]
		journalWriteBufPool.Put(&b)
		j.pendingBuf = nil
	}
	j.fallbackBuf = nil
	j.diskBytes = 0
	f := j.file
	j.file = nil
	path := j.path
	j.ioMu.Unlock()
	j.writtenBytes = 0

	var closeErr error
	if f != nil {
		closeErr = f.Close()
	}
	if path != "" {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) && closeErr == nil {
			closeErr = err
		}
		_ = os.Remove(path + ".tmp")
	}
	return closeErr
}

// Close closes and removes the backing journal file.
func (j *TreeJournal) Close() error {
	if j == nil {
		return nil
	}
	runtime.SetFinalizer(j, nil)
	return j.stopAndRemoveFile()
}

func (j *TreeJournal) asyncWriterLoop() {
	for {
		j.ioMu.Lock()
		for len(j.pendingBuf) == 0 && !j.writerStop {
			j.ioCond.Wait()
		}
		if len(j.pendingBuf) == 0 && j.writerStop {
			j.writerRunning = false
			j.ioCond.Broadcast()
			j.ioMu.Unlock()
			return
		}
		buf := j.pendingBuf
		j.pendingBuf = nil
		j.flushingBuf = buf
		j.flushing = true
		f := j.file
		prevDiskBytes := j.diskBytes
		hasErr := j.asyncErr != nil
		j.ioMu.Unlock()

		var err error
		if hasErr || f == nil {
			err = errors.New("journal writer disabled")
		} else {
			_, err = f.Write(buf)
			if err != nil {
				_ = f.Truncate(prevDiskBytes)
				_, _ = f.Seek(prevDiskBytes, io.SeekStart)
			}
		}

		j.ioMu.Lock()
		if err != nil {
			if j.asyncErr == nil {
				j.asyncErr = err
			}
			j.fallbackBuf = append(j.fallbackBuf, buf...)
		} else {
			j.diskBytes += int64(len(buf))
		}
		j.flushingBuf = nil
		b := buf[:0]
		journalWriteBufPool.Put(&b)
		j.flushing = false
		j.ioCond.Broadcast()
		j.ioMu.Unlock()
	}
}

func (j *TreeJournal) flushAsync() error {
	if j == nil {
		return nil
	}
	if j.syncOnly {
		return j.flushSyncBuf()
	}
	j.ioMu.Lock()
	defer j.ioMu.Unlock()
	if j.ioCond != nil {
		j.ioCond.Broadcast()
	}
	for (len(j.pendingBuf) > 0 || j.flushing) && j.writerRunning {
		j.ioCond.Wait()
	}
	if len(j.pendingBuf) > 0 {
		if j.asyncErr == nil && j.file != nil {
			if _, err := j.file.Write(j.pendingBuf); err != nil {
				_ = j.file.Truncate(j.diskBytes)
				_, _ = j.file.Seek(j.diskBytes, io.SeekStart)
				j.asyncErr = err
				j.fallbackBuf = append(j.fallbackBuf, j.pendingBuf...)
			} else {
				j.diskBytes += int64(len(j.pendingBuf))
			}
		} else {
			j.fallbackBuf = append(j.fallbackBuf, j.pendingBuf...)
		}
		b := j.pendingBuf[:0]
		journalWriteBufPool.Put(&b)
		j.pendingBuf = nil
	}
	return j.asyncErr
}

func (j *TreeJournal) flushSyncBuf() error {
	if j == nil || len(j.pendingBuf) == 0 {
		return nil
	}
	if j.file == nil {
		return errors.New("nil journal file")
	}
	if _, err := j.file.Write(j.pendingBuf); err != nil {
		_ = j.file.Truncate(j.diskBytes)
		_, _ = j.file.Seek(j.diskBytes, io.SeekStart)
		j.fail(fmt.Sprintf("write error: %v", err))
		return err
	}
	j.diskBytes += int64(len(j.pendingBuf))
	j.pendingBuf = j.pendingBuf[:0]
	return nil
}

func (j *TreeJournal) fail(reason string) {
	if j == nil || j.failed {
		return
	}
	j.failed = true
	seclog.Debugf("activity tree journal [%s] disabled, falling back to in-memory payloads: %s", j.path, reason)
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

	if j.syncOnly {
		j.pendingBuf = append(j.pendingBuf, j.scratchBuf...)
		if len(j.pendingBuf) >= journalSyncBatchBytes {
			if err := j.flushSyncBuf(); err != nil {
				return false
			}
		}
		if cap(j.scratchBuf) > journalDefaultBufSize {
			j.scratchBuf = nil
		}
		j.writtenBytes += frameBytes
		return true
	}

	j.ioMu.Lock()
	if j.asyncErr != nil {
		err := j.asyncErr
		j.ioMu.Unlock()
		j.fail(fmt.Sprintf("async write error: %v", err))
		return false
	}
	if j.writerStop || j.file == nil {
		j.ioMu.Unlock()
		return false
	}
	if int64(len(j.pendingBuf))+frameBytes > journalMaxAsyncPendingBytes && j.flushing {
		j.ioMu.Unlock()
		j.fail(fmt.Sprintf("async pending buffer cap (%d bytes) reached", journalMaxAsyncPendingBytes))
		return false
	}
	if j.pendingBuf == nil {
		bp := journalWriteBufPool.Get().(*[]byte)
		j.pendingBuf = (*bp)[:0]
	}
	j.pendingBuf = append(j.pendingBuf, j.scratchBuf...)
	if !j.writerRunning {
		j.writerRunning = true
		go j.asyncWriterLoop()
	} else {
		j.ioCond.Signal()
	}
	j.ioMu.Unlock()

	if cap(j.scratchBuf) > journalDefaultBufSize {
		j.scratchBuf = nil
	}
	j.writtenBytes += frameBytes
	return true
}

const (
	fnvOffset64 uint64 = 14695981039346656037
	fnvPrime64  uint64 = 1099511628211
)

func fnvHashByte(h uint64, b byte) uint64 {
	return (h ^ uint64(b)) * fnvPrime64
}

func fnvHashUint32(h uint64, v uint32) uint64 {
	h = fnvHashByte(h, byte(v))
	h = fnvHashByte(h, byte(v>>8))
	h = fnvHashByte(h, byte(v>>16))
	h = fnvHashByte(h, byte(v>>24))
	return h
}

func fnvHashString(h uint64, s string) uint64 {
	for i := 0; i < len(s); i++ {
		h = (h ^ uint64(s[i])) * fnvPrime64
	}
	return h
}

func computeEnrichmentHash(procID, minSeq uint32, category, key, subKey string) uint64 {
	h := fnvOffset64
	h = fnvHashUint32(h, procID)
	h = fnvHashUint32(h, minSeq)
	h = fnvHashString(h, category)
	h = fnvHashByte(h, 0)
	h = fnvHashString(h, key)
	h = fnvHashByte(h, 0)
	h = fnvHashString(h, subKey)
	return h
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
	procID := pn.journalID
	hadJournalID := procID != 0
	if !hadJournalID {
		procID = j.allocProcID()
	}
	seq := j.nextSeq()
	minSeq := pn.minJournalSeq
	if minSeq == 0 {
		minSeq = seq
	}
	newRules, hashes := j.filterNewRules(procID, minSeq, "proc_rule", "", pn.MatchedRules)
	procProto := processNodeToProto(&pn.Process)
	msg := adproto.ProcessActivityNode{
		Process:      procProto,
		MatchedRules: newRules,
	}
	prevBytes := j.writtenBytes
	ok := j.writeFrame(journalKindProcess, seq, procID, 0, &msg)
	if procProto != nil {
		procProto.ReturnToVTPool()
	}
	if ok {
		pn.journalID = procID
		if hadJournalID {
			j.fragmentedBytes += j.writtenBytes - prevBytes
		}
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
	prevBytes := j.writtenBytes
	if j.writeFrame(journalKindProcess, seq, pn.journalID, 0, &msg) {
		j.fragmentedBytes += j.writtenBytes - prevBytes
		j.commitEnrichments(hashes)
	} else {
		pn.MatchedRules = model.AppendMatchedRule(pn.MatchedRules, rules)
	}
}

func (j *TreeJournal) recordFileNode(fn *FileNode, reducedPath string) {
	if j == nil || fn == nil {
		return
	}
	hadJournalFile := fn.hasJournalFile
	if fn.minJournalSeq == 0 {
		fn.minJournalSeq = j.nextSeq()
	}
	if j.failed || j.currentProcID == 0 {
		return
	}
	if fn.File == nil && len(fn.MatchedRules) == 0 {
		return
	}
	seq := j.nextSeq()
	minSeq := fn.minJournalSeq
	subID := minSeq << 1
	if fn.isDir || (fn.File != nil && fn.File.IsDir()) || len(fn.Children) > 0 {
		fn.isDir = true
		subID |= 1
	}
	newRules, hashes := j.filterNewRules(j.currentProcID, minSeq, "file_rule", reducedPath, fn.MatchedRules)
	fileProto := fileInfoToProto(fn.File)
	msg := adproto.ProcessActivityNode{
		Files: []*adproto.FileActivityNode{
			{
				Name:         reducedPath,
				File:         fileProto,
				MatchedRules: newRules,
			},
		},
	}
	prevBytes := j.writtenBytes
	ok := j.writeFrame(journalKindFile, seq, j.currentProcID, subID, &msg)
	if fileProto != nil {
		fileProto.ReturnToVTPool()
	}
	if ok {
		if hadJournalFile {
			j.fragmentedBytes += j.writtenBytes - prevBytes
		}
		j.commitEnrichments(hashes)
		if fn.File != nil {
			fn.hasJournalFile = true
			fn.File = nil
		}
		fn.MatchedRules = nil
	}
}

func (j *TreeJournal) recordFileRules(fn *FileNode, reducedPath string, rules []*model.MatchedRule) {
	if fn != nil && fn.minJournalSeq == 0 && j != nil {
		fn.minJournalSeq = j.nextSeq()
	}
	if j == nil || j.failed || fn == nil || j.currentProcID == 0 || len(rules) == 0 {
		if fn != nil && len(rules) > 0 {
			fn.MatchedRules = model.AppendMatchedRule(fn.MatchedRules, rules)
		}
		return
	}
	minSeq := fn.minJournalSeq
	candidateRules := fn.MatchedRules
	if len(candidateRules) == 0 {
		candidateRules = rules
	}
	newRules, hashes := j.filterNewRules(j.currentProcID, minSeq, "file_rule", reducedPath, candidateRules)
	if len(newRules) == 0 {
		fn.MatchedRules = nil
		return
	}
	seq := j.nextSeq()
	subID := minSeq << 1
	if fn.isDir || (fn.File != nil && fn.File.IsDir()) || len(fn.Children) > 0 {
		fn.isDir = true
		subID |= 1
	}
	msg := adproto.ProcessActivityNode{
		Files: []*adproto.FileActivityNode{
			{
				Name:         reducedPath,
				MatchedRules: newRules,
			},
		},
	}
	prevBytes := j.writtenBytes
	if j.writeFrame(journalKindFile, seq, j.currentProcID, subID, &msg) {
		j.fragmentedBytes += j.writtenBytes - prevBytes
		j.commitEnrichments(hashes)
		fn.MatchedRules = nil
	} else {
		fn.MatchedRules = model.AppendMatchedRule(fn.MatchedRules, rules)
	}
}

func (j *TreeJournal) recordMergedFileNode(dst, src *FileNode) {
	if j == nil || dst == nil || src == nil {
		return
	}
	j.dirty = true
	if dst.minJournalSeq == 0 {
		dst.minJournalSeq = src.minJournalSeq
		return
	}
	if src.minJournalSeq != 0 && src.minJournalSeq != dst.minJournalSeq {
		j.mergedFileSeqs[src.minJournalSeq] = dst.minJournalSeq
	}
}

func (j *TreeJournal) shouldCompact() bool {
	if j == nil || j.failed || (!j.dirty && j.fragmentedBytes < j.maxBytes/4) {
		return false
	}
	return j.writtenBytes >= j.maxBytes*3/4 && j.writtenBytes-j.lastCheckpointBytes >= j.maxBytes/4
}

func (j *TreeJournal) shouldCheckpointOnFlush() bool {
	if j == nil || j.failed {
		return false
	}
	if j.dirty {
		return true
	}
	threshold := int64(journalFlushCompactThreshold)
	if quarter := j.maxBytes / 4; quarter > 0 && quarter < threshold {
		threshold = quarter
	}
	return j.fragmentedBytes >= threshold
}

func (j *TreeJournal) maybeCompactBeforeInsert(at *ActivityTree) {
	if at != nil && j.shouldCompact() {
		j.withMaterializedTree(at, func() {})
	}
}

func (j *TreeJournal) onEviction(at *ActivityTree) {
	if j == nil {
		return
	}
	j.dirty = true
	if at != nil && len(at.ProcessNodes) == 0 {
		j.enrichmentHashes = make(map[uint64]struct{})
		j.mergedFileSeqs = make(map[uint32]uint32)
	}
}

func (j *TreeJournal) recordDNSEnrichment(pn *ProcessNode, dn *DNSNode, qIdx int, resp *model.DNSResponse, rules []*model.MatchedRule) bool {
	if dn != nil && dn.minJournalSeq == 0 && j != nil {
		dn.minJournalSeq = j.nextSeq()
	}
	if j == nil || j.failed || pn == nil || dn == nil || qIdx < 0 || qIdx >= len(dn.Requests) {
		return false
	}
	j.setCurrentProc(pn)
	if j.failed || pn.journalID == 0 {
		return false
	}
	minSeq := dn.minJournalSeq
	req := &dn.Requests[qIdx]
	newRules, hashes := j.filterNewRules(pn.journalID, minSeq, "dns_rule", req.Question.Name, rules)

	var filteredResp *DNSResponseAggregate
	var addedIPs, addedCNames uint8
	if resp != nil && (len(resp.IPs) > 0 || len(resp.CNames) > 0) {
		qKey := fmt.Sprintf("%s:%d", req.Question.Name, req.Question.Type)
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
	hadPriorDNS := req.journalIPCount > 0 || req.journalCNameCount > 0
	prevBytes := j.writtenBytes
	if !j.writeFrame(journalKindDNS, seq, pn.journalID, uint32(qIdx), &msg) {
		return false
	}
	if hadPriorDNS || len(newRules) > 0 {
		j.fragmentedBytes += j.writtenBytes - prevBytes
	}
	j.commitEnrichments(hashes)
	req.journalIPCount += addedIPs
	req.journalCNameCount += addedCNames
	req.Response = nil
	dn.MatchedRules = nil
	return true
}

func (j *TreeJournal) recordIMDSRules(pn *ProcessNode, in *IMDSNode, rules []*model.MatchedRule) {
	if in != nil && in.minJournalSeq == 0 && j != nil {
		in.minJournalSeq = j.nextSeq()
	}
	if j == nil || j.failed || pn == nil || in == nil {
		if in != nil && len(rules) > 0 {
			in.MatchedRules = model.AppendMatchedRule(in.MatchedRules, rules)
		}
		return
	}
	if len(rules) == 0 {
		return
	}
	j.setCurrentProc(pn)
	if j.failed || pn.journalID == 0 {
		in.MatchedRules = model.AppendMatchedRule(in.MatchedRules, rules)
		return
	}
	minSeq := in.minJournalSeq
	key := in.Event.URL + "|" + in.Event.AWS.SecurityCredentials.AccessKeyID
	newRules, hashes := j.filterNewRules(pn.journalID, minSeq, "imds_rule", key, rules)
	if len(newRules) == 0 {
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
	prevBytes := j.writtenBytes
	if j.writeFrame(journalKindIMDS, seq, pn.journalID, 0, &msg) {
		j.fragmentedBytes += j.writtenBytes - prevBytes
		j.commitEnrichments(hashes)
		in.MatchedRules = nil
	} else {
		in.MatchedRules = model.AppendMatchedRule(in.MatchedRules, rules)
	}
}

func (j *TreeJournal) recordSocketRules(family string, isConnect bool, port uint16, ip string, protocol uint16, minSeq *uint32, rules []*model.MatchedRule) bool {
	if minSeq != nil && *minSeq == 0 && j != nil {
		*minSeq = j.nextSeq()
	}
	if j == nil || j.failed || minSeq == nil {
		return false
	}
	if len(rules) == 0 {
		return true
	}
	if j.currentProcID == 0 {
		return false
	}
	curMinSeq := *minSeq
	key := fmt.Sprintf("%s:%v:%s:%d:%d", family, isConnect, ip, port, protocol)
	newRules, hashes := j.filterNewRules(j.currentProcID, curMinSeq, "sock_rule", key, rules)
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
	prevBytes := j.writtenBytes
	if !j.writeFrame(journalKindSocket, seq, j.currentProcID, subID, &msg) {
		return false
	}
	j.fragmentedBytes += j.writtenBytes - prevBytes
	j.commitEnrichments(hashes)
	return true
}

func (j *TreeJournal) recordNetworkDeviceRules(ndn *NetworkDeviceNode, rules []*model.MatchedRule) {
	if ndn != nil && ndn.minJournalSeq == 0 && j != nil {
		ndn.minJournalSeq = j.nextSeq()
	}
	if j == nil || j.failed || ndn == nil {
		if ndn != nil && len(rules) > 0 {
			ndn.MatchedRules = model.AppendMatchedRule(ndn.MatchedRules, rules)
		}
		return
	}
	if len(rules) == 0 {
		return
	}
	if j.currentProcID == 0 {
		ndn.MatchedRules = model.AppendMatchedRule(ndn.MatchedRules, rules)
		return
	}
	minSeq := ndn.minJournalSeq
	key := fmt.Sprintf("%d:%d:%s", ndn.Context.NetNS, ndn.Context.IfIndex, ndn.Context.IfName)
	newRules, hashes := j.filterNewRules(j.currentProcID, minSeq, "netdev_rule", key, rules)
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
				Ifname:       ndn.Context.IfName,
				MatchedRules: newRules,
			},
		},
	}
	prevBytes := j.writtenBytes
	if j.writeFrame(journalKindNetworkDevice, seq, j.currentProcID, 0, &msg) {
		j.fragmentedBytes += j.writtenBytes - prevBytes
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

func (pn *ProcessNode) restoreFromJournalProto(pi *adproto.ProcessInfo) {
	if pn == nil || pi == nil {
		return
	}
	decoded := protoDecodeProcessNode(pi)
	pn.Process.Argv = decoded.Argv
	pn.Process.Envs = decoded.Envs
	pn.Process.EnvsTruncated = decoded.EnvsTruncated
	pn.Process.ArgsTruncated = decoded.ArgsTruncated
	pn.Process.Credentials = decoded.Credentials
	pn.Process.CGroup = decoded.CGroup
	pn.Process.TTYName = decoded.TTYName
	pn.Process.Comm = decoded.Comm
	if pn.Process.Argv0 == "" {
		pn.Process.Argv0 = decoded.Argv0
	}
	livePath := pn.Process.FileEvent.PathnameStr
	liveBase := pn.Process.FileEvent.BasenameStr
	livePathKey := pn.Process.FileEvent.PathKey
	pn.Process.FileEvent = decoded.FileEvent
	if livePath != "" {
		pn.Process.FileEvent.PathnameStr = livePath
	}
	if liveBase != "" {
		pn.Process.FileEvent.BasenameStr = liveBase
	}
	if livePathKey.Inode != 0 {
		pn.Process.FileEvent.PathKey = livePathKey
	}
}

// withMaterializedTree hydrates the stripped fields of at from the on-disk journal, runs fn,
// compacts the journal if dirty or fragmented, and strips the heavy fields back out of RAM.
func (j *TreeJournal) withMaterializedTree(at *ActivityTree, fn func()) {
	if j == nil || at == nil || j.permanentlyMaterialized {
		fn()
		return
	}
	if j.materializedDepth > 0 {
		j.materializedDepth++
		defer func() {
			j.materializedDepth--
		}()
		fn()
		return
	}
	if matErr := j.Materialize(at); matErr != nil {
		j.fail(fmt.Sprintf("materialize failed: %v", matErr))
		j.permanentlyMaterialized = true
		_ = j.stopAndRemoveFile()
		at.recomputeSizeBytes()
		fn()
		return
	}
	j.materializedDepth = 1
	defer func() {
		j.materializedDepth = 0
		if j.shouldCheckpointOnFlush() && !j.failed {
			if err := j.Checkpoint(at); err != nil {
				j.fail(fmt.Sprintf("checkpoint failed: %v", err))
				j.permanentlyMaterialized = true
				_ = j.stopAndRemoveFile()
				at.recomputeSizeBytes()
				return
			}
		}
		if !j.failed {
			j.StripTree(at)
		} else {
			j.permanentlyMaterialized = true
			_ = j.stopAndRemoveFile()
			at.recomputeSizeBytes()
		}
	}()
	fn()
}

// Materialize replays the on-disk journal into the live nodes of at.
func (j *TreeJournal) Materialize(at *ActivityTree) error {
	if j == nil || at == nil || j.permanentlyMaterialized {
		return nil
	}
	if err := j.flushAsync(); err != nil {
		j.fail(fmt.Sprintf("flush before materialize: %v", err))
	}

	j.ioMu.Lock()
	file := j.file
	diskBytes := j.diskBytes
	var fallbackCopy []byte
	if len(j.fallbackBuf) > 0 {
		fallbackCopy = make([]byte, len(j.fallbackBuf))
		copy(fallbackCopy, j.fallbackBuf)
	}
	j.ioMu.Unlock()

	if file == nil && len(fallbackCopy) == 0 {
		return nil
	}

	liveProcs := make(map[uint32]*ProcessNode)
	liveFileSeqs := make(map[uint32]struct{})
	var collectFiles func(fn *FileNode)
	collectFiles = func(fn *FileNode) {
		if fn == nil {
			return
		}
		if fn.minJournalSeq != 0 {
			liveFileSeqs[fn.minJournalSeq] = struct{}{}
		}
		for _, child := range fn.Children {
			collectFiles(child)
		}
	}
	var collectProcs func(nodes []*ProcessNode)
	collectProcs = func(nodes []*ProcessNode) {
		for _, pn := range nodes {
			if pn == nil {
				continue
			}
			if pn.journalID != 0 {
				liveProcs[pn.journalID] = pn
			}
			for _, fn := range pn.Files {
				collectFiles(fn)
			}
			collectProcs(pn.Children)
		}
	}
	collectProcs(at.ProcessNodes)
	for len(j.mergedFileSeqs) > 0 {
		added := false
		for srcSeq, dstSeq := range j.mergedFileSeqs {
			if _, srcIn := liveFileSeqs[srcSeq]; srcIn {
				continue
			}
			if _, dstIn := liveFileSeqs[dstSeq]; dstIn {
				liveFileSeqs[srcSeq] = struct{}{}
				added = true
			}
		}
		if !added {
			break
		}
	}

	type dnsReqKey struct {
		dn   *DNSNode
		qIdx int
	}
	var savedDNSResponses map[dnsReqKey]*DNSResponseAggregate
	if j.failed {
		for _, pn := range liveProcs {
			for _, dn := range pn.DNSNames {
				if dn == nil {
					continue
				}
				for i := range dn.Requests {
					if dn.Requests[i].Response != nil {
						if savedDNSResponses == nil {
							savedDNSResponses = make(map[dnsReqKey]*DNSResponseAggregate)
						}
						savedDNSResponses[dnsReqKey{dn: dn, qIdx: i}] = dn.Requests[i].Response
						dn.Requests[i].Response = nil
					}
				}
			}
		}
	}
	defer func() {
		for k, saved := range savedDNSResponses {
			if k.dn.Requests[k.qIdx].Response == nil {
				k.dn.Requests[k.qIdx].Response = saved
			} else {
				mergeDNSResponseAggregate(k.dn.Requests[k.qIdx].Response, saved)
			}
		}
	}()

	var stream io.Reader
	if file != nil && diskBytes > 0 {
		stream = io.NewSectionReader(file, 0, diskBytes)
	}
	if len(fallbackCopy) > 0 {
		fbReader := bytes.NewReader(fallbackCopy)
		if stream != nil {
			stream = io.MultiReader(stream, fbReader)
		} else {
			stream = fbReader
		}
	}
	if stream == nil {
		return nil
	}

	reader := journalReaderPool.Get().(*bufio.Reader)
	reader.Reset(stream)
	defer func() {
		reader.Reset(nil)
		journalReaderPool.Put(reader)
	}()

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
				pn.restoreFromJournalProto(protoNode.Process)
			}
			for _, r := range protoNode.MatchedRules {
				pn.MatchedRules = model.AppendMatchedRule(pn.MatchedRules, []*model.MatchedRule{protoDecodeProtoMatchedRule(r)})
			}

		case journalKindFile:
			for _, fan := range protoNode.Files {
				if fan == nil {
					continue
				}
				birthSeq := subID >> 1
				if birthSeq != 0 {
					if _, alive := liveFileSeqs[birthSeq]; !alive {
						continue
					}
				}
				isDir := (subID&1) == 1 || (fan.File != nil && (fan.File.Mode&0170000) == 0040000)
				fn, resolvedPath := resolveLiveFileNode(pn.Files, fan.Name, !isDir)
				if fn == nil || (birthSeq == 0 && fn.minJournalSeq > 0 && seq < fn.minJournalSeq) {
					continue
				}
				if fan.File != nil && fn.hasJournalFile {
					if fn.File == nil || birthSeq == fn.minJournalSeq {
						fn.File = newFileInfo(protoDecodeFileEvent(fan.File))
						fn.File.PathnameStr = resolvedPath
						fn.File.BasenameStr = fn.Name
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
					if ctx.NetNS == ndProto.Netns && ctx.IfIndex == ndProto.Ifindex && ctx.IfName == ndProto.Ifname {
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

	return nil
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
	if err := j.flushAsync(); err != nil {
		j.fail(fmt.Sprintf("flush before checkpoint: %v", err))
		return err
	}
	tmpPath := j.path + ".tmp"
	tmpFile, err := os.OpenFile(tmpPath, os.O_CREATE|os.O_RDWR|os.O_TRUNC, 0600)
	if err != nil {
		return err
	}

	staged := &TreeJournal{
		path:             tmpPath,
		file:             tmpFile,
		maxBytes:         j.maxBytes,
		scratchBuf:       j.scratchBuf,
		syncOnly:         true,
		enrichmentHashes: make(map[uint64]struct{}),
	}
	var commits []func()

	var writeProc func(pn *ProcessNode)
	writeProc = func(pn *ProcessNode) {
		if pn == nil || staged.failed {
			return
		}
		procID := staged.allocProcID()
		seq := staged.nextSeq()
		newRules, hashes := staged.filterNewRules(procID, seq, "proc_rule", "", pn.MatchedRules)
		procProto := processNodeToProto(&pn.Process)
		msg := adproto.ProcessActivityNode{
			Process:      procProto,
			MatchedRules: newRules,
		}
		ok := staged.writeFrame(journalKindProcess, seq, procID, 0, &msg)
		if procProto != nil {
			procProto.ReturnToVTPool()
		}
		if !ok {
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
				subID := fSeq << 1
				if fn.isDir || (fn.File != nil && fn.File.IsDir()) || len(fn.Children) > 0 {
					fn.isDir = true
					subID |= 1
				}
				fRules, fHashes := staged.filterNewRules(procID, fSeq, "file_rule", fullPath, fn.MatchedRules)
				fileProto := fileInfoToProto(fn.File)
				fMsg := adproto.ProcessActivityNode{
					Files: []*adproto.FileActivityNode{
						{
							Name:         fullPath,
							File:         fileProto,
							MatchedRules: fRules,
						},
					},
				}
				okFile := staged.writeFrame(journalKindFile, fSeq, procID, subID, &fMsg)
				if fileProto != nil {
					fileProto.ReturnToVTPool()
				}
				if !okFile {
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
				key := fmt.Sprintf("%d:%d:%s", ndn.Context.NetNS, ndn.Context.IfIndex, ndn.Context.IfName)
				ndRules, ndHashes := staged.filterNewRules(procID, ndSeq, "netdev_rule", key, ndn.MatchedRules)
				if len(ndRules) > 0 {
					rSeq := staged.nextSeq()
					ndMsg := adproto.ProcessActivityNode{
						NetworkDevices: []*adproto.NetworkDeviceNode{
							{
								Netns:        ndn.Context.NetNS,
								Ifindex:      ndn.Context.IfIndex,
								Ifname:       ndn.Context.IfName,
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
	if !staged.failed {
		_ = staged.flushSyncBuf()
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
	j.ioMu.Lock()
	oldFile := j.file
	j.file = tmpFile
	j.diskBytes = staged.diskBytes
	j.pendingBuf = nil
	j.fallbackBuf = nil
	j.asyncErr = nil
	j.ioMu.Unlock()
	if oldFile != nil {
		_ = oldFile.Close()
	}
	j.seq = staged.seq
	j.nextProcID = staged.nextProcID
	j.currentProcID = staged.currentProcID
	j.writtenBytes = staged.writtenBytes
	j.lastCheckpointBytes = staged.writtenBytes
	j.fragmentedBytes = 0
	j.enrichmentHashes = staged.enrichmentHashes
	clear(j.mergedFileSeqs)
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
			if fn.File.IsDir() {
				fn.isDir = true
			}
			fn.hasJournalFile = true
			fn.File = nil
		}
		if len(fn.Children) > 0 {
			fn.isDir = true
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

func hashProcessArgs(argv []string) [16]byte {
	h := sha256.New()
	var lenBuf [4]byte
	binary.LittleEndian.PutUint32(lenBuf[:], uint32(len(argv)))
	_, _ = h.Write(lenBuf[:])
	for _, arg := range argv {
		binary.LittleEndian.PutUint32(lenBuf[:], uint32(len(arg)))
		_, _ = h.Write(lenBuf[:])
		_, _ = io.WriteString(h, arg)
	}
	var sum [sha256.Size]byte
	h.Sum(sum[:0])
	var out [16]byte
	copy(out[:], sum[:16])
	return out
}
