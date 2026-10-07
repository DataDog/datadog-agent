// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package smb tails log files on an SMB share (Windows, Samba, Azure Files).
//
// A Tailer reads one file identity (client.Identity: the server's 64-bit
// FileId and the file's creation time) from the path it was created for and,
// after a rotation, from the name the file was renamed to. It never holds a file handle: every read opens the file, reads a byte
// range and closes it before the bytes reach the decoder, so the log writer can
// always rename, truncate or delete the file. Reads are driven by the SMB
// launcher's scan loop through Poll; the tailer has no polling goroutine of its
// own.
package smb

import (
	"context"
	"fmt"
	"path"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/benbjohnson/clock"
	"go.uber.org/atomic"

	"github.com/DataDog/datadog-agent/comp/logs-library/metrics"
	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
	"github.com/DataDog/datadog-agent/pkg/logs/internal/decoder"
	"github.com/DataDog/datadog-agent/pkg/logs/internal/smb/client"
	"github.com/DataDog/datadog-agent/pkg/logs/internal/tag"
	"github.com/DataDog/datadog-agent/pkg/logs/internal/util"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
	status "github.com/DataDog/datadog-agent/pkg/logs/status/utils"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	// DefaultChunkSize is the most bytes requested by one ReadAt call.
	DefaultChunkSize = 256 * 1024
	// DefaultPollBudget bounds the bytes one Poll reads, so a file far behind
	// does not hold up the other files of its source for long.
	DefaultPollBudget = 4 * 1024 * 1024
	// DefaultForceReadEvery makes a tailer whose listing shows no new data read
	// anyway every that many polls: directory listings can report a stale size
	// for a file another client holds open for writing.
	DefaultForceReadEvery = 10

	// offsetVersion prefixes the registry offsets this tailer writes, which
	// hold the file's FileId and creation time. offsetVersionNoCreation
	// prefixes those of earlier versions, without the creation time.
	offsetVersion           = "v2"
	offsetVersionNoCreation = "v1"
)

// Outcome tells the launcher what a Poll found.
type Outcome int

const (
	// OutcomeUnchanged means no read was needed: the listing shows no new data.
	OutcomeUnchanged Outcome = iota
	// OutcomeRead means the file was read, whether or not it had new data.
	OutcomeRead
	// OutcomeIdentityChanged means the file now at the tailer's read path is
	// not the one it tails (a different FileId or creation time). Nothing was
	// forwarded from it.
	OutcomeIdentityChanged
	// OutcomeTruncated means the file is shorter than the tailer's offset
	// (copytruncate, or a delete and recreate that reused the FileId on a
	// server that reports no creation times). Nothing was read; the tailer
	// should be replaced by one reading from offset 0.
	OutcomeTruncated
)

// String implements fmt.Stringer.
func (o Outcome) String() string {
	switch o {
	case OutcomeUnchanged:
		return "unchanged"
	case OutcomeRead:
		return "read"
	case OutcomeIdentityChanged:
		return "identity changed"
	case OutcomeTruncated:
		return "truncated"
	default:
		return "outcome(" + strconv.Itoa(int(o)) + ")"
	}
}

// Identifier returns the registry identifier of path on a share. It names the
// file by path, so a rotated file's replacement keeps the same key. It never
// contains credentials.
func Identifier(host, share, filePath string) string {
	return "smb://" + strings.ToLower(host) + "/" + strings.ToLower(share) + "/" + filePath
}

// EncodeOffset returns the registry offset string for offset in the file
// file: "v2:<FileId>:<creation time>:<offset>", the creation time in Unix
// nanoseconds (0 when unknown).
func EncodeOffset(file client.Identity, offset int64) string {
	return offsetVersion + ":" + strconv.FormatUint(file.FileID, 10) + ":" + strconv.FormatInt(file.Created, 10) + ":" + strconv.FormatInt(offset, 10)
}

// DecodeOffset parses a registry offset written by EncodeOffset. It also
// accepts the offsets of earlier versions: "v1:<FileId>:<offset>", which has
// no creation time, and a bare decimal offset, which has no FileId either. The
// parts of file that an offset does not hold are 0 (unknown).
func DecodeOffset(s string) (file client.Identity, offset int64, ok bool) {
	parts := strings.Split(s, ":")
	switch {
	case len(parts) == 4 && parts[0] == offsetVersion:
	case len(parts) == 3 && parts[0] == offsetVersionNoCreation:
		parts = []string{parts[0], parts[1], "0", parts[2]}
	case len(parts) == 1 && s != "":
		parts = []string{"", "0", "0", s}
	default:
		return client.Identity{}, 0, false
	}
	fileID, err := strconv.ParseUint(parts[1], 10, 64)
	if err != nil {
		return client.Identity{}, 0, false
	}
	created, err := strconv.ParseInt(parts[2], 10, 64)
	if err != nil {
		return client.Identity{}, 0, false
	}
	offset, err = strconv.ParseInt(parts[3], 10, 64)
	if err != nil || offset < 0 {
		return client.Identity{}, 0, false
	}
	return client.Identity{FileID: fileID, Created: created}, offset, true
}

// TailerOptions holds the parameters of NewTailer.
type TailerOptions struct {
	Source          *sources.ReplaceableSource // Required
	Client          client.Client              // Required
	Host            string                     // Required: names the share in the identifier and tags
	Share           string                     // Required
	Path            string                     // Required: path relative to the share root, as client.CleanPath returns it
	File            client.Identity            // Optional: the parts it does not know are adopted from the first read
	OutputChan      chan *message.Message      // Required
	CapacityMonitor *metrics.CapacityMonitor   // Required
	Decoder         decoder.Decoder            // Required
	Info            *status.InfoRegistry       // Required
	Rotated         bool                       // Optional: the tailer replaces one whose file rotated
	ChunkSize       int                        // Optional: 0 means DefaultChunkSize
	PollBudget      int                        // Optional: 0 means DefaultPollBudget
	ForceReadEvery  int                        // Optional: 0 means DefaultForceReadEvery
	// ProcessingRules are the global processing rules, which the logs
	// processor applies before the source's own (see Committed).
	ProcessingRules []*config.ProcessingRule // Optional
}

// Tailer tails one file of an SMB share.
//
// # Operational Overview
//
// Poll, called by the launcher's scan loop, reads new bytes with
// client.ReadAt (which opens and closes the file each time) and sends them to
// the decoder. forwardMessages, the tailer's only goroutine besides the
// decoder's, turns decoded messages into log messages carrying the registry
// identifier and offset, and sends them to the pipeline.
//
// Poll and the methods that change the read state (Start, StartDraining,
// SetReadPath, CommitTo) must be called from a single goroutine.
type Tailer struct {
	source     *sources.ReplaceableSource
	client     client.Client
	identifier string
	path       string   // path the tailer was created for: identifier and tags
	readPath   string   // where the file is now: path, or its rotated name while draining
	tags       []string // filename and dirname tags

	// fileID and created are the file's identity (see client.Identity), each
	// 0 while unknown.
	fileID  *atomic.Uint64
	created *atomic.Int64
	// offset is the next byte to read, i.e. the bytes sent to the decoder.
	offset *atomic.Int64
	// decodedOffset is the offset at which the latest decoded message ends.
	decodedOffset *atomic.Int64
	// lastSeenSize is the largest size observed for the file, from a listing
	// or a read. Bytes between offset and lastSeenSize exist but were not
	// read; they are lost if the tailer stops before reading them.
	lastSeenSize *atomic.Int64
	// draining is set once the tailer only finishes a rotated file.
	draining *atomic.Bool
	// commitMu guards commitTo and committed: once StartDraining or CommitTo
	// returns, no message forwarded later commits under the identifier it
	// replaced, so Committed no longer changes for that identifier.
	commitMu sync.Mutex
	// commitTo is the identifier the tailer's messages commit their offsets
	// under: its own, or, while draining, none ("") since its identifier
	// belongs to the path's new file, or the one CommitTo set.
	commitTo string
	// committed is set once a message forwarded commits under the tailer's
	// own identifier and passes the processing rules.
	committed bool
	// globalRules are the global processing rules (see Committed).
	globalRules []*config.ProcessingRule

	// Read state, owned by the goroutine calling Poll.
	lastListedSize int64 // size from the previous listing, -1 before the first one
	skippedPolls   int   // polls since the last read

	chunkSize      int
	pollBudget     int
	forceReadEvery int

	outputChan      chan *message.Message
	capacityMonitor *metrics.CapacityMonitor
	decoder         decoder.Decoder
	tagProvider     tag.Provider

	info      *status.InfoRegistry
	bytesRead *status.CountInfo
	movingSum *util.MovingSum
	fileInfo  *status.MappedInfo

	// forwardContext ends forwarding even while blocked on the output channel.
	forwardContext context.Context
	stopForward    context.CancelFunc
	done           chan struct{}
	started        bool
	stopOnce       sync.Once
}

// NewTailer returns a Tailer ready to be started. It takes ownership of the
// decoder.
func NewTailer(opts *TailerOptions) *Tailer {
	forwardContext, stopForward := context.WithCancel(context.Background())

	bytesRead := status.NewCountInfo("Bytes Read")
	opts.Info.Register(bytesRead)
	timeWindow := 24 * time.Hour
	totalBucket := 24
	movingSum := util.NewMovingSum(timeWindow, timeWindow/time.Duration(totalBucket), clock.New())
	opts.Info.Register(movingSum)
	fileInfo := status.NewMappedInfo("SMB File")
	opts.Info.Register(fileInfo)

	t := &Tailer{
		source:          opts.Source,
		client:          opts.Client,
		identifier:      Identifier(opts.Host, opts.Share, opts.Path),
		path:            opts.Path,
		readPath:        opts.Path,
		fileID:          atomic.NewUint64(opts.File.FileID),
		created:         atomic.NewInt64(opts.File.Created),
		offset:          atomic.NewInt64(0),
		decodedOffset:   atomic.NewInt64(0),
		lastSeenSize:    atomic.NewInt64(0),
		draining:        atomic.NewBool(false),
		lastListedSize:  -1,
		chunkSize:       orDefault(opts.ChunkSize, DefaultChunkSize),
		pollBudget:      orDefault(opts.PollBudget, DefaultPollBudget),
		forceReadEvery:  orDefault(opts.ForceReadEvery, DefaultForceReadEvery),
		globalRules:     opts.ProcessingRules,
		outputChan:      opts.OutputChan,
		capacityMonitor: opts.CapacityMonitor,
		decoder:         opts.Decoder,
		tagProvider:     tag.NewLocalProvider([]string{}),
		info:            opts.Info,
		bytesRead:       bytesRead,
		movingSum:       movingSum,
		fileInfo:        fileInfo,
		forwardContext:  forwardContext,
		stopForward:     stopForward,
		done:            make(chan struct{}),
	}
	dir := Identifier(opts.Host, opts.Share, path.Dir(opts.Path))
	if path.Dir(opts.Path) == "." { // a file at the share root
		dir = strings.TrimSuffix(Identifier(opts.Host, opts.Share, ""), "/")
	}
	t.commitTo = t.identifier
	t.tags = []string{
		"filename:" + path.Base(opts.Path),
		"dirname:" + dir,
	}
	if opts.Rotated {
		rotation := status.NewMappedInfo("Last Rotation Date")
		rotation.SetMessage("Last Rotation Date", time.Now().UTC().Format("2006-01-02 15:04:05 UTC"))
		opts.Info.Register(rotation)
	}
	t.updateFileInfo()
	return t
}

// Start makes the tailer read from offset and starts forwarding.
func (t *Tailer) Start(offset int64) {
	t.offset.Store(offset)
	t.decodedOffset.Store(offset)
	if offset > t.lastSeenSize.Load() {
		t.lastSeenSize.Store(offset)
	}
	t.started = true
	log.Infof("Starting SMB tailer for %s at offset %d (FileId %d)", t.identifier, offset, t.fileID.Load())
	go t.forwardMessages()
	t.decoder.Start()
}

// AssumeSize records that the file held size bytes when another tailer of it
// last saw it, or when a listing Poll is not given showed it, so that
// UnreadBytes counts the bytes past the offset before this tailer sees the
// file itself.
func (t *Tailer) AssumeSize(size int64) {
	if size > t.lastSeenSize.Load() {
		t.lastSeenSize.Store(size)
	}
}

// Stop flushes the decoder and returns once every decoded message has been
// forwarded. It is safe to call more than once, and on a tailer never started.
func (t *Tailer) Stop() {
	t.stopOnce.Do(func() {
		if !t.started {
			t.stopForward()
			return
		}
		t.decoder.Stop()
		<-t.done
		t.stopForward()
		log.Infof("Closed SMB tailer for %s (read from %s): read %d bytes and %d lines", t.identifier, t.readPath, t.bytesRead.Get(), t.decoder.GetLineCount())
	})
}

// AssumeCommitted records that a previous tailer of the file, which this one
// continues, forwarded a message that commits under the tailer's identifier
// (see Committed): once the pipeline delivers it, the registry entry of the
// tailer's path holds an offset of the file, although this tailer may not
// forward anything there.
func (t *Tailer) AssumeCommitted() {
	t.commitMu.Lock()
	defer t.commitMu.Unlock()
	t.committed = true
}

// StartDraining turns the tailer into the drain of a rotated file: its next
// Poll reads whatever the listing shows, since the file may have grown since
// the last listing of its previous name, later ones read while bytes it saw
// remain unread (see shouldRead), and its messages stop committing
// offsets under the path's identifier, which now belongs to the path's new
// file (see CommitTo). GetID changes too, so a tailer container holding the
// tailer must remove it before this call.
func (t *Tailer) StartDraining() {
	if t.draining.Swap(true) {
		return
	}
	t.lastListedSize = -1
	t.CommitTo("")
	draining := status.NewMappedInfo("Draining Since")
	draining.SetMessage("Draining Since", time.Now().UTC().Format("2006-01-02 15:04:05 UTC"))
	t.info.Register(draining)
}

// IsDraining reports whether StartDraining was called.
func (t *Tailer) IsDraining() bool {
	return t.draining.Load()
}

// CommitTo makes the messages of a draining tailer commit their offsets under
// identifier, or under none when it is "". The launcher points it at the
// identifier of the path the drained file sits at, when no other tailer
// commits there, so that an Agent restart resumes the file there.
func (t *Tailer) CommitTo(identifier string) {
	t.commitMu.Lock()
	defer t.commitMu.Unlock()
	t.commitTo = identifier
}

// CommitIdentifier returns the identifier the tailer's messages commit their
// offsets under, "" for none.
func (t *Tailer) CommitIdentifier() string {
	t.commitMu.Lock()
	defer t.commitMu.Unlock()
	return t.commitTo
}

// Committed reports whether the tailer forwarded a message that commits an
// offset under its own identifier and that the processing rules keep: once
// the pipeline delivers it, the registry entry of the tailer's path holds an
// offset of the tailer's file. A message the rules drop (exclude_at_match,
// include_at_match, exclude_truncated) does not count, since the processor
// drops it before the auditor commits its offset. After StartDraining
// returns, it changes only if CommitTo points the tailer at its own
// identifier again.
func (t *Tailer) Committed() bool {
	t.commitMu.Lock()
	defer t.commitMu.Unlock()
	return t.committed
}

// nextCommit returns the identifier msg, being forwarded, commits its offset
// under, "" for none, and records whether it is the tailer's own and the
// processing rules keep msg. The rules are only evaluated until a message
// passes them.
func (t *Tailer) nextCommit(msg *message.Message) string {
	t.commitMu.Lock()
	defer t.commitMu.Unlock()
	if t.commitTo == t.identifier && !t.committed &&
		keptByRules(msg, t.globalRules, t.source.UnderlyingSource().Config.ProcessingRules) {
		t.committed = true
	}
	return t.commitTo
}

// keptByRules reports whether the logs processor sends msg on rather than
// dropping it, given the global processing rules and the source's: it applies
// them in the processor's order (comp/logs-library/processor's
// applyRedactingRules), on a copy of msg's content.
func keptByRules(msg *message.Message, globalRules, sourceRules []*config.ProcessingRule) bool {
	content := msg.GetContent()
	for _, rules := range [2][]*config.ProcessingRule{globalRules, sourceRules} {
		for _, rule := range rules {
			switch rule.Type {
			case config.ExcludeAtMatch:
				if rule.Regex.Match(content) {
					return false
				}
			case config.IncludeAtMatch:
				if !rule.Regex.Match(content) {
					return false
				}
			case config.MaskSequences:
				content, _ = config.ApplyMaskSequence(content, rule)
			case config.ExcludeTruncated:
				if msg.IsTruncated {
					return false
				}
			}
		}
	}
	return true
}

// SetReadPath points a draining tailer at the name its file was renamed to.
func (t *Tailer) SetReadPath(p string) {
	if p != t.readPath {
		t.readPath = p
		t.updateFileInfo()
	}
}

// Poll reads the bytes the file gained since the previous poll and sends them
// to the decoder, up to the poll budget. entry is the file's directory entry
// from the current scan; nil always reads. Without new data in the listing, a
// read is still made every ForceReadEvery polls, since listing sizes can be
// stale, and a draining tailer reads at every poll while bytes a read or a
// listing showed remain unread.
//
// Bytes are sent to the decoder only after ReadAt returned, so no file handle
// is open while Poll waits for the pipeline. Offsets only move past bytes the
// decoder accepted: after an error, the next Poll resumes where this one
// stopped, without sending anything twice.
func (t *Tailer) Poll(ctx context.Context, entry *client.Entry) (Outcome, error) {
	if entry != nil {
		if !t.Identity().Matches(entry.Identity()) {
			return OutcomeIdentityChanged, nil
		}
		// A listed size only counts as seen when the listing confirms the
		// file's identity: otherwise it may be a replacement's size.
		if entry.FileID != 0 && entry.FileID == t.fileID.Load() && entry.Size > t.lastSeenSize.Load() {
			t.lastSeenSize.Store(entry.Size)
		}
	}
	if !t.shouldRead(entry) {
		t.skippedPolls++
		return OutcomeUnchanged, nil
	}
	t.skippedPolls = 0

	budget := t.pollBudget
	for {
		offset := t.offset.Load()
		res, err := t.client.ReadAt(ctx, t.readPath, offset, min(t.chunkSize, budget))
		if err != nil {
			return OutcomeRead, err
		}
		if !t.Identity().Matches(res.Identity()) {
			return OutcomeIdentityChanged, nil
		}
		t.adopt(res.Identity())
		if res.Size < offset {
			log.Infof("SMB file %s shrank from %d bytes read to %d bytes: truncated", t.readPath, offset, res.Size)
			return OutcomeTruncated, nil
		}
		if res.Size > t.lastSeenSize.Load() {
			t.lastSeenSize.Store(res.Size)
		}
		n := len(res.Data)
		if n > 0 {
			select {
			case t.decoder.InputChan() <- decoder.NewInput(res.Data):
			case <-ctx.Done():
				return OutcomeRead, ctx.Err()
			}
			t.offset.Add(int64(n))
			t.recordBytes(int64(n))
			budget -= n
		}
		// Stop at the end of the file (as of this read) rather than paying
		// for another round trip to learn it.
		if n == 0 || offset+int64(n) >= res.Size || budget <= 0 {
			return OutcomeRead, nil
		}
	}
}

// adopt records the parts of the file's identity that the tailer did not know
// yet. Poll checked that file matches the tailer's identity.
func (t *Tailer) adopt(file client.Identity) {
	changed := false
	if t.fileID.Load() == 0 && file.FileID != 0 {
		t.fileID.Store(file.FileID)
		changed = true
	}
	if t.created.Load() == 0 && file.Created != 0 {
		t.created.Store(file.Created)
		changed = true
	}
	if changed {
		t.updateFileInfo()
	}
}

// shouldRead reports whether Poll needs to read. It records the listed size.
func (t *Tailer) shouldRead(entry *client.Entry) bool {
	if entry == nil {
		return true
	}
	listed := entry.Size
	changed := listed != t.lastListedSize
	t.lastListedSize = listed
	// A drain whose last read stopped at the poll budget, or failed, reads on
	// while bytes it saw remain unread, whatever the listing shows: its file
	// may go away at any time. Only drains do: lastSeenSize never shrinks, so
	// a file truncated to a size between offset and lastSeenSize keeps
	// UnreadBytes above 0. A drain's reads of it find no new data, and the
	// drain ends after close_timeout; an active tailer would read it at every
	// poll for as long as it runs.
	return listed > t.offset.Load() || changed ||
		(t.draining.Load() && t.UnreadBytes() > 0) ||
		t.skippedPolls+1 >= t.forceReadEvery
}

// UnreadBytes returns the bytes known to exist in the file that were not read:
// a lower bound, since the file may have grown since it was last observed.
func (t *Tailer) UnreadBytes() int64 {
	return max(0, t.lastSeenSize.Load()-t.offset.Load())
}

// RecordMissedBytes reports UnreadBytes as lost, in the missed-bytes metrics,
// the source's Bytes Missed status and the logs, and returns it. Call it when
// the tailer stops for good with its file still holding unread data: the file
// is gone, or its drain timed out.
func (t *Tailer) RecordMissedBytes(reason string) int64 {
	missed := t.UnreadBytes()
	if missed <= 0 {
		return 0
	}
	recordMissed(t.source.UnderlyingSource(), missed)
	log.Warnf("%s: %d bytes of SMB file %s (last read as %s) were not read and are lost", reason, missed, t.identifier, t.readPath)
	return missed
}

// RecordMissedBytesOf reports missed bytes of the file read at identifier as
// lost, as RecordMissedBytes does, for a file no tailer reads anymore.
func RecordMissedBytesOf(source *sources.LogSource, identifier string, missed int64, reason string) {
	if missed <= 0 {
		return
	}
	recordMissed(source, missed)
	log.Warnf("%s: %d bytes of SMB file %s were not read and are lost", reason, missed, identifier)
}

func recordMissed(source *sources.LogSource, missed int64) {
	metrics.BytesMissed.Add(missed)
	metrics.TlmBytesMissed.Add(float64(missed))
	missedSource, missedService := missedBytesIdentity(source.Config)
	metrics.RecordMissedBytes(missedSource, missedService, missed)
	source.RecordMissedBytes(missed)
}

// forwardMessages forwards decoded messages to the output channel until the
// decoder is stopped and flushed.
func (t *Tailer) forwardMessages() {
	defer close(t.done)
	for output := range t.decoder.OutputChan() {
		offset := t.decodedOffset.Load() + int64(output.RawDataLenForCheckpoint())
		t.decodedOffset.Store(offset)
		metrics.TlmLogLineSizes.Observe(float64(output.RawDataLen))
		if !output.HasContent() {
			continue
		}

		origin := message.NewOrigin(t.source.UnderlyingSource())
		// A draining tailer reads a file that no longer owns the tailer's
		// identifier: committing its offsets there would move the new file's
		// offset backwards or forwards, so its messages carry none (as for
		// rotated file tailers), unless CommitTo gave it another identifier.
		if id := t.nextCommit(output); id != "" {
			origin.Identifier = id
			origin.Offset = EncodeOffset(t.Identity(), offset)
		}
		tags := make([]string, 0, len(t.tags)+len(output.ParsingExtra.Tags))
		tags = append(tags, t.tags...)
		tags = append(tags, t.tagProvider.GetTags()...)
		tags = append(tags, output.ParsingExtra.Tags...)
		origin.SetTags(tags)
		output.Origin = origin
		select {
		case t.outputChan <- output:
			t.capacityMonitor.AddIngress(output)
		case <-t.forwardContext.Done():
		}
	}
}

func (t *Tailer) recordBytes(n int64) {
	t.source.UnderlyingSource().RecordBytes(n)
	t.bytesRead.Add(n)
	t.movingSum.Add(n)
}

func (t *Tailer) updateFileInfo() {
	t.fileInfo.SetMessage("FileId", fmt.Sprintf("FileId: %d", t.fileID.Load()))
	if t.readPath != t.path {
		t.fileInfo.SetMessage("Reading From", "Reading from: "+t.readPath)
	}
}

// Identifier returns the tailer's registry identifier.
func (t *Tailer) Identifier() string {
	return t.identifier
}

// Path returns the path the tailer was created for.
func (t *Tailer) Path() string {
	return t.path
}

// ReadPath returns where the tailer reads its file now.
func (t *Tailer) ReadPath() string {
	return t.readPath
}

// FileID returns the FileId of the tailed file, 0 while unknown.
func (t *Tailer) FileID() uint64 {
	return t.fileID.Load()
}

// Identity returns the identity of the tailed file, with the parts not known
// yet at 0.
func (t *Tailer) Identity() client.Identity {
	return client.Identity{FileID: t.fileID.Load(), Created: t.created.Load()}
}

// Offset returns the offset of the next byte to read.
func (t *Tailer) Offset() int64 {
	return t.offset.Load()
}

// Source returns the tailer's source.
func (t *Tailer) Source() *sources.LogSource {
	return t.source.UnderlyingSource()
}

// GetDetectedPattern returns the multiline pattern the decoder detected, so a
// replacement tailer can reuse it.
func (t *Tailer) GetDetectedPattern() *regexp.Regexp {
	return t.decoder.GetDetectedPattern()
}

// GetID implements tailers.Tailer. It is the registry identifier, except for a
// draining tailer: the path's new file has its own tailer under that ID, and
// agent status lists both.
func (t *Tailer) GetID() string {
	if t.draining.Load() {
		return t.identifier + " (rotated, FileId " + strconv.FormatUint(t.fileID.Load(), 10) + ")"
	}
	return t.identifier
}

// GetType implements tailers.Tailer.
func (t *Tailer) GetType() string {
	return "smb"
}

// GetInfo implements tailers.Tailer.
func (t *Tailer) GetInfo() *status.InfoRegistry {
	return t.info
}

// missedBytesIdentity resolves the tuple to report a loss under. It is a copy
// of the file tailer's helper (pkg/logs/tailers/file/missed_bytes.go), which
// is unexported, so both sources report losses under the same tuples.
func missedBytesIdentity(cfg *config.LogsConfig) (source string, service string) {
	const unknown = "unknown"
	if cfg == nil {
		return unknown, unknown
	}
	source = unknown
	switch {
	case cfg.Source != "":
		source = cfg.Source
	case cfg.IntegrationName != "":
		source = cfg.IntegrationName
	}
	service = unknown
	switch {
	case cfg.Service != "":
		service = cfg.Service
	case cfg.Source != "":
		service = cfg.Source
	case cfg.IntegrationName != "":
		service = cfg.IntegrationName
	}
	return source, service
}

func orDefault(v, def int) int {
	if v <= 0 {
		return def
	}
	return v
}
