// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build smb && !goexperiment.systemcrypto && !goexperiment.boringcrypto && !requirefips

package smb

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"path"
	"regexp"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
	"github.com/DataDog/datadog-agent/pkg/logs/internal/decoder"
	"github.com/DataDog/datadog-agent/pkg/logs/internal/smb/client"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
	status "github.com/DataDog/datadog-agent/pkg/logs/status/utils"
	tailer "github.com/DataDog/datadog-agent/pkg/logs/tailers/smb"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/startstop"
)

// scanner tails the files of one smb source. Its goroutine lists the share,
// polls the tailers and handles rotations; every field below cancel is only
// used on that goroutine.
//
// A tailer follows one file identity (client.Identity: FileId and creation
// time). On each scan, for every path with an active tailer:
//   - same identity in the listing: poll it;
//   - another identity (rotation, or a new file that reuses the FileId of the
//     one it replaced): the tailer becomes a drain of its file, which it looks
//     up by identity under any listed name, and a new tailer reads the path
//     from offset 0 right away;
//   - path no longer listed: the tailer becomes a drain the same way;
//   - shorter than the offset (truncation): a new tailer reads from offset 0.
//
// A drain ends when two consecutive polls find no new data in the file, when
// closeTimeout elapses, or when its file is no longer listed; bytes known to
// exist but not read are then reported as missed, unless the file sits at a
// path the pattern matches: the file's next tailer then resumes where the
// drain ended, whatever name the file has by the time it starts. Until then,
// the drain commits its offsets under the identifier of that path, so that an
// Agent restart resumes the file there too (see commitDrainAt). While a drain
// lasts, agent status lists it next to the path's new tailer. A source that
// stops for good while a drain lasts reports the drain's unread bytes as
// missed, unless a restart resumes its file (see stopTailers).
//
// A path tailed for the first time can hold another file than its stored
// position names (where the scanner this one replaces stopped reading it, or
// else the registry offset): the file rotated away while the path was not
// tailed, during an Agent restart or a source replacement. The rest of that
// file is read from the stored offset wherever it is now, as for a rotation
// (see resumeRotatedAway).
//
// start_position applies to the files that were there when the source
// started, i.e. the files matched until the first scan that lists every
// directory of the pattern. A file that appears later is new: it is read from
// the beginning, as file sources do.
type scanner struct {
	l         *Launcher
	source    *sources.LogSource
	client    client.Client
	clientKey clientKey
	host      string
	share     string
	target    string // smb://host/share, for logs and status
	password  string // only to scrub status messages
	pattern   string // clean path pattern, relative to the share root
	excludes  []string
	mode      config.TailingMode
	interval  time.Duration

	cancel context.CancelFunc
	done   chan struct{}
	// replaced is set before the scanner stops for a scanner of the same
	// configuration that continues its work (see Launcher.replace); any other
	// stop is final. It is atomic since the scanner also stops when the
	// launcher's context ends, while Stop runs.
	replaced atomic.Bool

	active    map[string]*tailer.Tailer // by path
	committed map[string]int64          // by path: the committed offset of each active tailer's file when it started (see handoff.committed)
	draining  []*drain
	stopping  sync.WaitGroup            // drains being stopped
	fromStart map[string]bool           // paths this scanner read from with a tailer, with no active tailer now: their next tailer reads from offset 0, unless it resumes a file (see startPosition)
	drainedAt map[string]bool           // paths a drain committed its offsets under, with no tailer since: their registry offset is no position from before this scanner started (see commitDrainAt)
	resume    map[uint64]handoff        // files a drain finished reading, by FileId: the file's next tailer resumes there, whatever its path
	inherited map[string]handoff        // where the tailers of the scanner this one replaces stopped, by path (see resumeFrom)
	patterns  map[string]*regexp.Regexp // multiline patterns of the paths' previous tailers
	conflicts map[string]bool           // paths tailed by another source, already logged
	mismatch  map[string]int            // consecutive identity changes the listing does not show
	blocked   map[string]time.Time      // paths whose file could not be opened (locked or missing), since then
	listedAll bool                      // a scan listed every directory of the pattern
	initial   map[string]bool           // paths matched until then, not tailed yet: they start at start_position
	failing   bool                      // the previous scan reported an error
	lastErr   string

	// stoppedAt is where the tailers stopped reading, set once the scanner
	// stopped, for a scanner that replaces this one (see Launcher.replace).
	stoppedAt map[string]handoff
}

type drain struct {
	t        *tailer.Tailer
	deadline time.Time
	caughtUp int    // consecutive polls that found no new data
	commitAt string // path whose identifier the drain commits its offsets under, "" for none (see commitDrainAt)
	// committed is an offset of the file the registry can hold for it without
	// skipping a byte that was not delivered: the offset last committed for
	// the file before it rotated, or the committed offset its tailer started
	// with.
	committed int64
	// resumePaths holds the paths whose registry entry holds an offset of the
	// drain's file, so that an Agent restart that lists another file at one
	// of them resumes the drain's file from there (see resumeRotatedAway):
	// the drain's own path, when the file's tailer committed offsets there or
	// started from that entry, and every path the drain committed its offsets
	// under (see commitDrainAt). A path leaves it once a tailer of another
	// file commits there, or resumes another file there (see pathHolds).
	resumePaths map[string]bool
}

// handoff is where the scanner stopped reading a file: the file's next tailer
// resumes there instead of reading it again.
type handoff struct {
	file   client.Identity
	offset int64
	size   int64 // the largest size seen for the file, 0 when unknown: the bytes past offset were not read
	// committed is an offset of the file, at most offset, up to which every
	// byte was delivered as far as the scanner knew when it stored the
	// handoff: the bytes up to offset were read, but some may still be on
	// their way through the pipeline. A drain of the file's next tailer
	// records no more than that under a new path (see commitDrainAt).
	committed int64
	// held tells that the file's tailer forwarded messages committing under
	// the path the handoff is stored for (see tailer.Tailer.Committed): once
	// the pipeline delivers them, the path's registry entry holds an offset
	// of the file, even if the next tailer of the file forwards nothing
	// there.
	held bool
}

// newScanner returns a scanner of source, which passed validation.
func newScanner(l *Launcher, source *sources.LogSource, c client.Client, key clientKey) (*scanner, error) {
	cfg := source.Config.SMB
	s := &scanner{
		l:         l,
		source:    source,
		client:    c,
		clientKey: key,
		host:      cfg.Host,
		share:     cfg.Share,
		target:    "smb://" + cfg.Host + "/" + cfg.Share,
		password:  cfg.Password,
		interval:  defaultPollInterval,
		active:    make(map[string]*tailer.Tailer),
		committed: make(map[string]int64),
		fromStart: make(map[string]bool),
		drainedAt: make(map[string]bool),
		resume:    make(map[uint64]handoff),
		inherited: make(map[string]handoff),
		patterns:  make(map[string]*regexp.Regexp),
		conflicts: make(map[string]bool),
		mismatch:  make(map[string]int),
		blocked:   make(map[string]time.Time),
		initial:   make(map[string]bool),
		done:      make(chan struct{}),
	}
	if cfg.PollInterval > 0 {
		// Validate bounds poll_interval; bounding it again keeps the ticker
		// from panicking whatever the value. Clamp in seconds before
		// converting: a float too large for a Duration converts to an
		// arbitrary value (negative on amd64).
		seconds := min(max(cfg.PollInterval, config.SMBMinPollInterval.Seconds()), config.SMBMaxPollInterval.Seconds())
		s.interval = time.Duration(seconds * float64(time.Second))
	}
	s.mode, _ = config.TailingModeFromString(source.Config.TailingMode)
	// Validate rejects the patterns CleanPath refuses; CleanPath normalizes
	// them.
	var err error
	if s.pattern, err = client.CleanPath(source.Config.Path); err != nil {
		return nil, fmt.Errorf("invalid smb path: %w", err)
	}
	for _, exclude := range source.Config.ExcludePaths {
		p, err := client.CleanPath(exclude)
		if err != nil {
			return nil, fmt.Errorf("invalid smb exclude_paths entry: %w", err)
		}
		s.excludes = append(s.excludes, p)
	}
	return s, nil
}

// resumeFrom makes s, not started yet, continue the work of prev, a stopped
// scanner of the same configuration: s's tailers resume prev's files where
// prev's tailers stopped reading them, so nothing is read twice, the registry
// offsets prev's drains recorded are not taken for positions stored before s
// started (see drainedAt), and start_position does not apply again to files
// prev found after it started.
func (s *scanner) resumeFrom(prev *scanner) {
	for p, h := range prev.stoppedAt {
		s.inherited[p] = h
	}
	for p := range prev.drainedAt {
		if _, ok := s.inherited[p]; !ok {
			s.drainedAt[p] = true
		}
	}
	for p, pattern := range prev.patterns {
		s.patterns[p] = pattern
	}
	s.listedAll = prev.listedAll
	for p := range prev.initial {
		s.initial[p] = true
	}
}

func (s *scanner) start(ctx context.Context) {
	ctx, s.cancel = context.WithCancel(ctx)
	go s.run(ctx)
}

// Stop stops the scanner and its tailers for good (the Agent stops, or the
// source is removed), and returns once they have flushed.
func (s *scanner) Stop() {
	s.stop(false)
}

// stopForReplacement stops the scanner and its tailers for a scanner of the
// same configuration that continues its work (see resumeFrom), and returns
// once they have flushed.
func (s *scanner) stopForReplacement() {
	s.stop(true)
}

func (s *scanner) stop(replaced bool) {
	if s.cancel == nil {
		return
	}
	s.replaced.Store(replaced)
	s.cancel()
	<-s.done
}

func (s *scanner) run(ctx context.Context) {
	defer close(s.done)
	defer s.stopTailers()
	log.Infof("Tailing %s matching %q every %s", s.target, s.pattern, s.interval)
	ticker := s.l.clock.Ticker(s.interval)
	defer ticker.Stop()
	for {
		s.scan(ctx)
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// stopTailers stops every tailer of the scanner and waits for them. It records
// where the active tailers stopped reading in stoppedAt.
//
// Unless the scanner is replaced, its stop is final: the bytes a drain did not
// read are reported missed, unless an Agent restart can resume its file from
// a registry entry that holds an offset of it (see drain.resumePaths). That is
// decided once every tailer stopped: stopping flushes the decoders, whose last
// messages may commit under the drain's paths.
//
// The decision rests on what the scanner knows when it stops, which is wrong
// in two cases:
//   - every path whose registry entry holds the file disappears for good
//     while the Agent is down: no scan reads those entries again, so the
//     bytes are lost without being reported missed;
//   - the logs processor drops every message of the path's new file for
//     another reason than the processing rules, such as a failure to render
//     or encode it: the new file's tailer counts them as committed (see
//     tailer.Tailer.Committed), so the drain's bytes are reported missed
//     although a restart resumes them from the path's registry entry.
func (s *scanner) stopTailers() {
	s.stoppedAt = make(map[string]handoff, len(s.active))
	stopper := startstop.NewParallelStopper()
	active := make([]*tailer.Tailer, 0, len(s.active))
	for p, t := range s.active {
		// Nothing polls t anymore: its offset is final, and stopping t
		// forwards everything it read.
		s.stoppedAt[p] = handoff{
			file:      t.Identity(),
			offset:    t.Offset(),
			size:      t.Offset() + t.UnreadBytes(),
			committed: s.committedOffset(t.Identifier(), t, s.committed[p]),
		}
		s.deactivate(p, t)
		stopper.Add(t)
		active = append(active, t)
	}
	for _, d := range s.draining {
		stopper.Add(d.t)
	}
	stopper.Stop()
	s.stopping.Wait()

	for _, t := range active {
		// Stopping t may have forwarded its first committing message.
		h := s.stoppedAt[t.Path()]
		h.held = t.Committed()
		s.stoppedAt[t.Path()] = h
		s.pathCommitted(t)
	}
	for _, d := range s.draining {
		resumed := len(d.resumePaths) > 0
		s.releaseCommitPath(d) // its last messages committed there
		s.l.tailers.Remove(d.t)
		if !s.replaced.Load() && !resumed {
			d.t.RecordMissedBytes("SMB source stopped while draining a rotated file")
		}
		// Nothing goes on with d: a scanner that replaces this one resumes
		// d's file from the registry entries that hold an offset of it (see
		// resumeRotatedAway), so their paths leave drainedAt.
		for p := range d.resumePaths {
			delete(s.drainedAt, p)
		}
	}
	s.draining = nil
}

// scan lists the share once, then polls every tailer.
func (s *scanner) scan(ctx context.Context) {
	var errs scanErrors
	v := s.list(ctx, &errs)
	if ctx.Err() != nil {
		return
	}
	if !s.listedAll {
		for p := range v.matches {
			if s.active[p] == nil {
				s.initial[p] = true
			}
		}
		s.listedAll = !v.failed
	}
	s.forgetUnlisted(v)

	for _, p := range sortedKeys(s.active) {
		t := s.active[p]
		entry, listed, known := v.lookup(p)
		switch {
		case !known:
			// Its directory could not be listed: nothing to conclude.
		case !listed:
			log.Infof("SMB file %s is no longer listed; reading the rest of %s wherever it was moved", t.Identifier(), t.Identity())
			committed := s.committed[p]
			s.deactivate(p, t)
			s.startDrain(t, committed)
			s.fromStart[p] = true
		case entry.FileID != 0 && t.FileID() != 0 && !t.Identity().Matches(entry.Identity()):
			log.Infof("SMB file %s rotated (%s, now %s); reading the new file from the beginning", t.Identifier(), t.Identity(), entry.Identity())
			committed := s.committed[p]
			s.deactivate(p, t)
			s.startDrain(t, committed)
			s.fromStart[p] = true // started below, in this scan
		default:
			s.poll(ctx, p, t, &entry, &errs)
		}
	}

	s.resumeRotatedAway(v)
	for _, p := range sortedKeys(v.matches) {
		if _, ok := s.active[p]; !ok {
			s.startTailer(ctx, p, v.matches[p], &errs)
		}
	}

	s.pollDrains(ctx, v, &errs)
	if ctx.Err() != nil {
		return
	}
	s.report(&errs)
}

// forgetUnlisted forgets the per-path state of the paths the listing v shows
// are gone, and where the drains of the files no longer listed ended. The
// bytes such a drain knew its file held past where it ended are lost: no
// tailer reads them anymore.
func (s *scanner) forgetUnlisted(v *view) {
	if !v.failed && len(s.resume) > 0 {
		// One lookup per resume point, of which there is one per drained
		// file still listed.
		listed := v.filesByID()
		for id, h := range s.resume {
			if !slices.ContainsFunc(listed[id], h.file.Matches) {
				tailer.RecordMissedBytesOf(s.source, s.target+" ("+h.file.String()+")", h.size-h.offset, "Rotated SMB file is no longer listed")
				delete(s.resume, id)
			}
		}
	}
	gone := func(p string) bool {
		_, listed, known := v.lookup(p)
		return known && !listed
	}
	for p := range s.blocked {
		if gone(p) {
			delete(s.blocked, p)
		}
	}
	for p := range s.initial {
		if gone(p) {
			delete(s.initial, p)
		}
	}
	for p := range s.mismatch {
		if gone(p) {
			delete(s.mismatch, p)
		}
	}
}

// poll polls the active tailer t of path p and handles what it found.
func (s *scanner) poll(ctx context.Context, p string, t *tailer.Tailer, entry *client.Entry, errs *scanErrors) {
	outcome, err := t.Poll(ctx, entry)
	if err != nil {
		s.fileErr(p, t.Identifier(), err, errs)
		return
	}
	if outcome != tailer.OutcomeUnchanged {
		delete(s.blocked, p) // the file could be opened
	}
	switch outcome {
	case tailer.OutcomeIdentityChanged:
		if entry.FileID != 0 {
			// The file was replaced between the listing and the read. The
			// next listing shows the new file and the rotation is handled
			// then, with a drain of the old file.
			s.mismatch[p]++
			if s.mismatch[p] == 3 {
				log.Warnf("SMB file %s: the directory listing keeps reporting %s but opening the file reports another file; the server's FileIds or creation times may be inconsistent", t.Identifier(), entry.Identity())
			}
			return
		}
		// The listing has no FileIds, so the old file cannot be found again.
		log.Infof("SMB file %s was replaced by another file; reading the new file from the beginning", t.Identifier())
		s.deactivate(p, t)
		t.RecordMissedBytes("SMB file replaced")
		t.StartDraining() // its last messages must not commit offsets
		s.pathCommitted(t)
		s.stopAsync(t)
		s.fromStart[p] = true // started by the caller's loop over new paths
	case tailer.OutcomeTruncated:
		log.Infof("SMB file %s was truncated; reading it again from the beginning", t.Identifier())
		s.deactivate(p, t)
		t.RecordMissedBytes("SMB file truncated")
		// Its last messages must not commit offsets past the new content.
		t.StartDraining()
		s.pathCommitted(t)
		s.stopAsync(t)
		s.fromStart[p] = true
	default:
		delete(s.mismatch, p)
	}
}

// resumeRotatedAway finds the files that rotated away from a path while this
// scanner did not tail it: the paths this scan starts tailing for the first
// time whose stored position (see storedPosition) names another file than the
// one now at the path, and the paths the scanner this one replaces tailed that
// are no longer listed. Each such file is read from its stored offset (see
// resumeElsewhere), and the path's new file from offset 0.
//
// It runs before any tailer of the scan starts, so that the tailer of the
// path a file was renamed to resumes it whatever the order of the paths.
//
// The registry can hold offsets of one file under several of those paths:
// the entry of the path the file rotated away from keeps its offset until the
// path's new file commits, while the file's offsets are committed under the
// matched name it rotated to. The file is read from the furthest of them, so
// that nothing delivered is sent again: resumeElsewhere ignores a file it
// already resumes from at least as far.
func (s *scanner) resumeRotatedAway(v *view) {
	type rotatedAway struct {
		p      string
		stored handoff
	}
	var away []rotatedAway
	for _, p := range sortedKeys(v.matches) {
		entry := v.matches[p]
		identifier := tailer.Identifier(s.host, s.share, p)
		if s.active[p] != nil || s.fromStart[p] || s.drainedAt[p] || entry.FileID == 0 || s.l.claims.ownedByAnother(identifier, s) {
			continue
		}
		stored, found := s.storedPosition(p, identifier)
		if !found || stored.file.FileID == 0 || stored.file.Matches(entry.Identity()) {
			continue
		}
		log.Infof("SMB file %s was replaced while it was not tailed (%s, now %s); reading the new file from the beginning", identifier, stored.file, entry.Identity())
		delete(s.inherited, p)
		s.fromStart[p] = true
		away = append(away, rotatedAway{p, stored})
	}
	for _, p := range sortedKeys(s.inherited) {
		if _, listed, known := v.lookup(p); known && !listed {
			stored := s.inherited[p]
			delete(s.inherited, p)
			if stored.file.FileID != 0 {
				log.Infof("SMB file %s is no longer listed; reading the rest of %s wherever it was moved", tailer.Identifier(s.host, s.share, p), stored.file)
				away = append(away, rotatedAway{p, stored})
			}
		}
	}
	slices.SortStableFunc(away, func(a, b rotatedAway) int { return cmp.Compare(b.stored.offset, a.stored.offset) })
	for _, a := range away {
		s.resumeElsewhere(a.p, a.stored, v)
	}
}

// storedPosition returns where the file at p was last read before this
// scanner tailed p: where the scanner this one replaces stopped reading it,
// or else the registry offset.
func (s *scanner) storedPosition(p, identifier string) (handoff, bool) {
	if h, ok := s.inherited[p]; ok {
		return h, true
	}
	file, offset, ok := tailer.DecodeOffset(s.l.registry.GetOffset(identifier))
	return handoff{file: file, offset: offset, committed: offset}, ok
}

// resumeElsewhere reads the rest of the file stored names, read at p up to
// stored.offset before it rotated away from p, wherever the file is now:
//   - at a path the pattern matches: the tailer of that path resumes it there
//     (see startPosition), unless that path's own stored position is further;
//   - at another path: a drain reads it, as after a rotation;
//   - nowhere: the bytes known to remain in it are reported as missed.
func (s *scanner) resumeElsewhere(p string, stored handoff, v *view) {
	identifier := tailer.Identifier(s.host, s.share, p)
	if s.tailingFile(stored.file) || s.drainingFile(stored.file) {
		return // this scanner reads it already
	}
	if h, found := s.resumePoint(stored.file); found && h.offset >= stored.offset {
		return
	}
	at, _, found := v.findFile(stored.file)
	if !found && !v.failed {
		if missed := stored.size - stored.offset; missed > 0 {
			tailer.RecordMissedBytesOf(s.source, identifier, missed, "Rotated SMB file is no longer listed")
		} else {
			log.Infof("SMB file %s: %s, read up to offset %d, rotated away while it was not tailed and is no longer listed", identifier, stored.file, stored.offset)
		}
		return
	}
	if _, matched := v.matches[at]; found && matched {
		atIdentifier := tailer.Identifier(s.host, s.share, at)
		if own, ok := s.storedPosition(at, atIdentifier); ok && !s.fromStart[at] && own.file.FileID == stored.file.FileID && own.file.Matches(stored.file) && own.offset >= stored.offset {
			return
		}
		log.Infof("SMB file %s rotated to %s while it was not tailed; reading %s from offset %d there", identifier, atIdentifier, stored.file, stored.offset)
		s.resume[stored.file.FileID] = stored
		return
	}
	log.Infof("SMB file %s rotated while it was not tailed; reading the rest of %s from offset %d wherever it was moved", identifier, stored.file, stored.offset)
	t := s.newTailer(p, stored.file, s.patterns[p], false)
	if found {
		t.SetReadPath(at)
	}
	if stored.held {
		t.AssumeCommitted() // what the scanner this one replaces sent under p
	}
	t.Start(stored.offset)
	s.startDrain(t, stored.committed)
}

// tailingFile reports whether an active tailer reads the file file.
func (s *scanner) tailingFile(file client.Identity) bool {
	for _, t := range s.active {
		if t.FileID() == file.FileID && t.Identity().Matches(file) {
			return true
		}
	}
	return false
}

// startTailer starts a tailer for the matched path p and polls it.
func (s *scanner) startTailer(ctx context.Context, p string, entry client.Entry, errs *scanErrors) {
	if s.drainingFile(entry.Identity()) {
		// A rotated file being drained, renamed to another matched path.
		// The path's tailer starts where the drain ends (see endDrain).
		return
	}
	identifier := tailer.Identifier(s.host, s.share, p)
	if !s.l.claims.claim(identifier, s) {
		if !s.conflicts[p] {
			s.conflicts[p] = true
			log.Warnf("SMB file %s is already tailed by another source; not tailing it for source %s", identifier, s.source.Name)
		}
		return
	}
	delete(s.conflicts, p)

	inherited, wasInherited := s.inherited[p]
	offset, committed, file, ok := s.startPosition(ctx, p, identifier, entry, errs)
	if !ok {
		s.l.claims.release(identifier, s)
		return
	}
	rotated := s.fromStart[p] || s.drainedAt[p]
	delete(s.fromStart, p)
	delete(s.drainedAt, p)
	delete(s.initial, p)
	pattern := s.patterns[p]
	delete(s.patterns, p)

	t := s.newTailer(p, file, pattern, rotated)
	if wasInherited && inherited.held && sameFile(inherited.file, file) {
		t.AssumeCommitted() // what the scanner this one replaces sent under p
	}
	t.Start(offset)
	s.active[p] = t
	s.committed[p] = committed
	s.l.tailers.Add(t)
	s.source.AddInput(identifier)
	s.l.registry.SetTailed(identifier, true)
	s.poll(ctx, p, t, &entry, errs)
}

// newTailer returns a tailer, not started, of file at p, whose decoder starts
// with the multiline pattern pattern. rotated tells that it replaces a tailer
// whose file rotated.
func (s *scanner) newTailer(p string, file client.Identity, pattern *regexp.Regexp, rotated bool) *tailer.Tailer {
	replaceable := sources.NewReplaceableSource(s.source)
	info := status.NewInfoRegistry()
	outputChan, monitor := s.l.pipelineProvider.NextPipelineChanWithMonitor()
	return tailer.NewTailer(&tailer.TailerOptions{
		Source:          replaceable,
		Client:          s.client,
		Host:            s.host,
		Share:           s.share,
		Path:            p,
		File:            file,
		OutputChan:      outputChan,
		CapacityMonitor: monitor,
		Decoder:         decoder.NewDecoderFromSourceWithPattern(replaceable, pattern, info),
		Info:            info,
		Rotated:         rotated,
		ChunkSize:       s.l.chunkSize,
		PollBudget:      s.l.pollBudget,
		ForceReadEvery:  s.l.forceReadEvery,
		ProcessingRules: s.l.processingRules,
	})
}

// startPosition returns the offset a new tailer of p starts at:
//   - where a drain of the same file ended, whatever the file's name was then;
//   - where the scanner this one replaces stopped reading p, when p still
//     holds the same file;
//   - 0 for a path whose previous file was replaced while being tailed, or
//     rotated away while the path was not tailed (see resumeRotatedAway);
//   - the registry offset, when it was recorded for the same file identity
//     and is not past the end of the file;
//   - 0 when the registry offset was recorded for another file identity or is
//     past the end of the file: the file was replaced or truncated while it
//     was not tailed, so all of it is new (as for file sources,
//     pkg/logs/launchers/file/position.go);
//   - for a file that was there when the source started (see scanner), 0
//     (start_position: beginning) or the end of the file (end);
//   - otherwise 0: the file appeared since, so all of it is new.
//
// The registry offset of a path a drain committed its offsets under (see
// drainedAt) is used the same way: it resumes the drained file, back at the
// path without a resume point, and any other file there is new.
//
// The drain's offset comes first: a path whose previous file rotated away can
// receive a file that was already drained, the previous file of another
// matched path (app.log.1 renamed to app.log.2 and app.log to app.log.1). The
// drained file may also have been renamed again since its drain ended, before
// any tailer of its previous path started: drains are found by file identity,
// not by path.
//
// It also returns committed, the offset up to which the file is known to be
// delivered: offset itself, except for a file resumed where a drain or the
// replaced scanner read it up to (see handoff.committed).
//
// The size and identity come from opening the file, since listing sizes can
// be stale.
func (s *scanner) startPosition(ctx context.Context, p, identifier string, entry client.Entry, errs *scanErrors) (offset, committed int64, file client.Identity, ok bool) {
	file = entry.Identity()
	if h, found := s.resumePoint(file); found {
		// The tailer may forward nothing, its file read to its end, while
		// the registry entry of the path the file left gets another file's
		// offsets: record where the file resumes under p, so that a restart
		// resumes it there too.
		s.seedOffset(identifier, h.file, h.committed)
		s.pathHolds(p, h.file)
		delete(s.resume, file.FileID)
		return h.offset, h.committed, h.file, true
	}
	if h, found := s.inherited[p]; found {
		delete(s.inherited, p)
		if sameFile(h.file, file) {
			return h.offset, h.committed, h.file, true
		}
	}
	if s.fromStart[p] {
		return 0, 0, file, true
	}
	res, err := s.client.ReadAt(ctx, p, 0, 0)
	if err != nil {
		s.fileErr(p, identifier, err, errs)
		return 0, 0, client.Identity{}, false
	}
	delete(s.blocked, p)
	file = merge(res.Identity(), file)
	if file.FileID == 0 && entry.FileID != 0 {
		// The file was replaced between the listing and the read, which
		// tells no FileId. The tailer starts from the next listing, which
		// shows the new file's FileId: without it, the tailer could not
		// tell the path's next rotation (see scan), nor its drain find the
		// rotated file (see view.findFile).
		s.mismatch[p]++
		if s.mismatch[p] == 3 {
			log.Warnf("SMB file %s: the directory listing keeps reporting %s but opening the file reports another file; the server's FileIds or creation times may be inconsistent", identifier, entry.Identity())
		}
		return 0, 0, client.Identity{}, false
	}
	if stored, offset, found := tailer.DecodeOffset(s.l.registry.GetOffset(identifier)); found {
		switch {
		case !stored.Matches(file):
			if !s.drainedAt[p] {
				log.Infof("SMB file %s was replaced while it was not tailed (%s, now %s); reading it from the beginning", identifier, stored, file)
			}
			return 0, 0, file, true
		case offset <= res.Size:
			return offset, offset, file, true
		default:
			log.Infof("Stored offset %d for SMB file %s is past its end (%d bytes): it was truncated while it was not tailed; reading it from the beginning", offset, identifier, res.Size)
			return 0, 0, file, true
		}
	}
	if s.mode == config.Beginning || !s.initial[p] {
		return 0, 0, file, true
	}
	return res.Size, res.Size, file, true
}

// resumePoint returns where a drain of file ended, if one did.
func (s *scanner) resumePoint(file client.Identity) (handoff, bool) {
	if file.FileID == 0 {
		return handoff{}, false
	}
	h, found := s.resume[file.FileID]
	if !found || !h.file.Matches(file) {
		// Another file, which reused the FileId of the drained one.
		return handoff{}, false
	}
	return h, true
}

// seedOffset records offset of file under identifier, unless the registry
// already holds that file there at least as far.
func (s *scanner) seedOffset(identifier string, file client.Identity, offset int64) {
	if stored, at, ok := tailer.DecodeOffset(s.l.registry.GetOffset(identifier)); ok && sameFile(stored, file) && at >= offset {
		return
	}
	s.l.registry.SetOffset(identifier, tailer.EncodeOffset(file, offset))
}

// sameFile reports whether a and b identify the same file, as far as they
// tell: the same FileId, known or not in both, and creation times that match.
func sameFile(a, b client.Identity) bool {
	return a.FileID == b.FileID && a.Matches(b)
}

// merge returns the identity read, with the parts it does not know taken from
// the listing when the listing can describe the same file.
func merge(read, listed client.Identity) client.Identity {
	if !read.Matches(listed) {
		return read
	}
	if read.FileID == 0 {
		read.FileID = listed.FileID
	}
	if read.Created == 0 {
		read.Created = listed.Created
	}
	return read
}

// startDrain turns t, already removed from the active tailers, into a drain.
// committed is the committed offset t's file had when t started (see
// startPosition). The drain is tracked under its own ID (see tailer.GetID),
// so agent status shows it until it ends.
func (s *scanner) startDrain(t *tailer.Tailer, committed int64) {
	// The registry still holds the offset last committed for the file under
	// t's identifier: the path's new file has not committed anything yet.
	committed = s.committedOffset(t.Identifier(), t, committed)
	t.StartDraining()
	s.pathCommitted(t)
	resumePaths := make(map[string]bool)
	if stored, _, ok := tailer.DecodeOffset(s.l.registry.GetOffset(t.Identifier())); t.Committed() || ok && sameFile(stored, t.Identity()) {
		resumePaths[t.Path()] = true
	}
	s.l.tailers.Add(t)
	s.draining = append(s.draining, &drain{
		t:           t,
		deadline:    s.l.clock.Now().Add(s.l.closeTimeout),
		committed:   committed,
		resumePaths: resumePaths,
	})
}

// pathCommitted is called once t no longer commits under its own identifier.
// If t did, the registry entry of t's path holds an offset of t's file, so
// that entry no longer resumes the drains that read another file.
func (s *scanner) pathCommitted(t *tailer.Tailer) {
	if t.Committed() {
		s.pathHolds(t.Path(), t.Identity())
	}
}

// pathHolds records that the registry entry of p holds, or will once the
// messages forwarded are delivered, an offset of file: a restart can resume
// from there the drain of file, and no other drain.
func (s *scanner) pathHolds(p string, file client.Identity) {
	for _, d := range s.draining {
		if sameFile(d.t.Identity(), file) {
			d.resumePaths[p] = true
		} else {
			delete(d.resumePaths, p)
		}
	}
}

// committedOffset returns the registry offset of identifier when it was
// committed for the file of t, is past floor and not past what t read, else
// floor.
func (s *scanner) committedOffset(identifier string, t *tailer.Tailer, floor int64) int64 {
	file, offset, ok := tailer.DecodeOffset(s.l.registry.GetOffset(identifier))
	if ok && sameFile(file, t.Identity()) && offset > floor && offset <= t.Offset() {
		return offset
	}
	return floor
}

// pollDrains reads the rest of each rotated file and ends the drains that are
// done.
func (s *scanner) pollDrains(ctx context.Context, v *view, errs *scanErrors) {
	kept := s.draining[:0]
	for _, d := range s.draining {
		if ctx.Err() != nil {
			kept = append(kept, d)
			continue
		}
		if s.pollDrain(ctx, d, v, errs) {
			kept = append(kept, d)
		}
	}
	clear(s.draining[len(kept):])
	s.draining = kept
}

// pollDrain polls one drain and reports whether it goes on.
//
// When a directory could not be listed and the listing does not show the
// drain's file, the drain goes on: the file may be in that directory. Past its
// deadline, it goes on only while the directory its file was found in last
// cannot be listed, so that it ends with a listing that shows where the file
// is: endDrain then leaves a resume point instead of reporting the file's
// unread bytes missed, and the file's next tailer resumes it instead of
// reading it again.
func (s *scanner) pollDrain(ctx context.Context, d *drain, v *view, errs *scanErrors) bool {
	t := d.t
	expired := !s.l.clock.Now().Before(d.deadline)
	at, entry, found := v.findFile(t.Identity())
	if !found && v.failed {
		if _, _, known := v.lookup(t.ReadPath()); !expired || !known {
			return true
		}
	}
	if expired {
		s.endDrain(d, v, t.Offset(), fmt.Sprintf("SMB rotation drain timed out after %s (logs_config.close_timeout)", s.l.closeTimeout))
		return false
	}
	if !found {
		s.endDrain(d, v, t.Offset(), "Rotated SMB file is no longer listed")
		return false
	}
	t.SetReadPath(at)
	s.commitDrainAt(d, at, v)
	offset := t.Offset()
	outcome, err := t.Poll(ctx, &entry)
	switch {
	case err != nil:
		errs.addFileErr(t.Identifier(), err)
		return true
	case outcome == tailer.OutcomeIdentityChanged:
		return true // moved again since the listing: found again next scan
	case outcome == tailer.OutcomeTruncated:
		// The bytes not read before the truncation are gone; whatever the
		// file holds now is new.
		t.RecordMissedBytes("Rotated SMB file was truncated")
		// Its last messages must not commit offsets past the new content.
		s.stopCommitting(d)
		s.endDrain(d, v, 0, "")
		return false
	case t.CaughtUp() && t.Offset() == offset:
		// Nothing new since the previous poll: the writer may be done.
		d.caughtUp++
		if d.caughtUp >= drainCaughtUpPolls {
			s.endDrain(d, v, t.Offset(), "")
			return false
		}
		return true
	default:
		d.caughtUp = 0
		return true
	}
}

// endDrain stops the drain d. While its file is listed, the file's next tailer
// resumes it at offset, whatever name the file has by the time that tailer
// starts, so nothing is read twice: the tailer of the path the file sits at
// when the pattern matches it (another one, or its own path again after a
// rename back), or of a matched path the file is renamed to later.
//
// Unless the pattern matches the path the file sits at, whose tailer reads
// them, the bytes the drain did not read are reported as missed, with reason.
// The file's next tailer, if it gets one, then starts after them, so that no
// byte is both reported missed and sent. The resume point records the bytes
// left unread otherwise, reported missed if the file is no longer listed
// before its next tailer starts (see forgetUnlisted).
func (s *scanner) endDrain(d *drain, v *view, offset int64, reason string) {
	t := d.t
	committed := min(s.drainCommitted(d), offset)
	s.releaseCommitPath(d) // its last messages still commit there
	at, _, found := v.findFile(t.Identity())
	if _, matched := v.matches[at]; !matched && reason != "" {
		if missed := t.RecordMissedBytes(reason); missed > 0 {
			offset = t.Offset() + missed
		}
	}
	if found {
		h := handoff{file: t.Identity(), offset: offset, committed: committed}
		if offset == t.Offset() {
			// Not after bytes reported missed above, nor at the start of a
			// truncated file.
			h.size = offset + t.UnreadBytes()
		}
		s.resume[t.FileID()] = h
	}
	s.l.tailers.Remove(t)
	s.stopAsync(t)
}

// commitDrainAt makes the drain d commit its offsets under the identifier of
// at, the path its file sits at now, when the pattern matches at and no other
// tailer of the scanner commits there: an Agent restart then resumes the file
// at that path (see startPosition) instead of reading it again from the
// beginning or skipping its rest, as start_position would. Otherwise d commits
// no offset.
//
// When d starts committing under a path whose registry offset is not already
// as far in the same file, it first records there d.committed, so that a
// restart resumes the file there even if d has not forwarded anything yet.
// From then on, until a tailer starts at the path, its registry offset is no
// position stored before this scanner started (see drainedAt):
// resumeRotatedAway does not resume the drained file from there once another
// file sits at the path. The path's next tailer still resumes from there the
// drained file itself, should the file come back without a resume point (see
// startPosition). A scanner that replaces this one keeps those paths (see
// resumeFrom), but for those it resumes the file of a drain this scanner
// stopped from (see stopTailers).
//
// An Agent restart does not. What d reads once its file has left the path for
// a name the pattern does not match is committed nowhere, so the path's
// registry offset stays where d left it: a restart that finds another file at
// the path resumes d's file from that offset (see resumeRotatedAway) and sends
// those lines again. They are delivered at least once, not lost.
func (s *scanner) commitDrainAt(d *drain, at string, v *view) {
	target := ""
	if _, matched := v.matches[at]; matched && s.active[at] == nil && !s.drainCommitsAt(at, d) {
		if at == d.commitAt || s.l.claims.claim(tailer.Identifier(s.host, s.share, at), s) {
			target = at
		}
	}
	if target == d.commitAt {
		return
	}
	d.committed = s.drainCommitted(d)
	s.stopCommitting(d)
	if target == "" {
		return
	}
	identifier := tailer.Identifier(s.host, s.share, target)
	file := d.t.Identity()
	s.seedOffset(identifier, file, d.committed)
	s.l.registry.SetTailed(identifier, true)
	s.pathHolds(target, file)
	d.commitAt = target
	s.drainedAt[target] = true
	d.t.CommitTo(identifier)
	log.Debugf("SMB rotation drain of %s commits its offsets under %s", file, identifier)
}

// drainCommitted returns d.committed, or the offset committed since under the
// path d commits its offsets under, when that one is further.
func (s *scanner) drainCommitted(d *drain) int64 {
	if d.commitAt == "" {
		return d.committed
	}
	return s.committedOffset(tailer.Identifier(s.host, s.share, d.commitAt), d.t, d.committed)
}

// drainCommitsAt reports whether a drain other than d commits its offsets
// under the identifier of p.
func (s *scanner) drainCommitsAt(p string, d *drain) bool {
	for _, other := range s.draining {
		if other != d && other.commitAt == p {
			return true
		}
	}
	return false
}

// stopCommitting makes the drain d commit no offset anymore.
func (s *scanner) stopCommitting(d *drain) {
	d.t.CommitTo("")
	s.releaseCommitPath(d)
}

// releaseCommitPath releases the path the drain d commits its offsets under,
// unless a tailer started there since. The messages d already decoded may
// still commit there.
func (s *scanner) releaseCommitPath(d *drain) {
	if d.commitAt == "" {
		return
	}
	if s.active[d.commitAt] == nil {
		identifier := tailer.Identifier(s.host, s.share, d.commitAt)
		s.l.registry.SetTailed(identifier, false)
		s.l.claims.release(identifier, s)
	}
	d.commitAt = ""
}

// drainingFile reports whether a drain reads the file file.
func (s *scanner) drainingFile(file client.Identity) bool {
	if file.FileID == 0 {
		return false
	}
	for _, d := range s.draining {
		if d.t.FileID() == file.FileID && d.t.Identity().Matches(file) {
			return true
		}
	}
	return false
}

// deactivate removes t, the active tailer of p, from the active tailers and
// releases its identifier. It does not stop t.
func (s *scanner) deactivate(p string, t *tailer.Tailer) {
	delete(s.active, p)
	delete(s.committed, p)
	delete(s.mismatch, p)
	if pattern := t.GetDetectedPattern(); pattern != nil {
		s.patterns[p] = pattern
	}
	s.l.tailers.Remove(t)
	s.source.RemoveInput(t.Identifier())
	s.l.registry.SetTailed(t.Identifier(), false)
	s.l.claims.release(t.Identifier(), s)
}

// stopAsync stops t without blocking the scan on the pipeline.
func (s *scanner) stopAsync(t *tailer.Tailer) {
	s.stopping.Add(1)
	go func() {
		defer s.stopping.Done()
		t.Stop()
	}()
}

// report sets the source status from the scan's errors.
func (s *scanner) report(errs *scanErrors) {
	if errs.err == nil {
		if s.failing {
			log.Infof("SMB source %s on %s recovered", s.source.Name, s.target)
		}
		s.failing, s.lastErr = false, ""
		s.source.Status().Success()
		return
	}
	err := s.statusError(errs)
	if !s.failing || err.Error() != s.lastErr {
		log.Warnf("SMB source %s: %v", s.source.Name, err)
	} else {
		log.Debugf("SMB source %s: %v", s.source.Name, err)
	}
	s.failing, s.lastErr = true, err.Error()
	s.source.Status().Error(err)
}

// statusError explains errs for the source status, which is shown in agent
// status and sent to Datadog with the inventory metadata. The client removes
// the password from its errors; it is removed again here in case.
func (s *scanner) statusError(errs *scanErrors) error {
	var msg string
	switch errs.kind {
	case client.ErrAuth:
		msg = fmt.Sprintf("cannot read %s: the server rejected the credentials or denied access. Check the username, password and domain, and that the account can read the share and path (Azure Files: the username is the storage account name and the password a storage account key): %v", s.target, errs.err)
	case client.ErrTransient:
		msg = fmt.Sprintf("cannot reach %s, retrying: %v", s.target, errs.err)
	case client.ErrNotFound:
		msg = fmt.Sprintf("not found on %s: %v", s.target, errs.err)
	case client.ErrSharing:
		msg = fmt.Sprintf("cannot open a file on %s: another program keeps it open without letting others read it (sharing violation), or holds a lock on it: %v", s.target, errs.err)
	default:
		msg = fmt.Sprintf("error reading %s: %v", s.target, errs.err)
	}
	if s.password != "" {
		msg = strings.ReplaceAll(msg, s.password, "********")
	}
	return errors.New(msg)
}

// view is one scan's listing of the directories the pattern can match.
type view struct {
	dirs    map[string][]client.Entry // listed directories, by path ("" is the root)
	failed  bool                      // a directory could not be listed
	matches map[string]client.Entry   // matched files, by path
}

// lookup returns p's entry and whether p is listed. known is false when the
// listing cannot tell, because a directory could not be listed.
func (v *view) lookup(p string) (entry client.Entry, listed, known bool) {
	if e, ok := v.matches[p]; ok {
		return e, true, true
	}
	if _, ok := v.dirs[dirOf(p)]; ok {
		return client.Entry{}, false, true
	}
	return client.Entry{}, false, !v.failed
}

// findFile looks for the file file under any name in the listed directories.
// It needs the FileId: a file the creation time alone identifies is not
// looked for.
func (v *view) findFile(file client.Identity) (string, client.Entry, bool) {
	if file.FileID == 0 {
		return "", client.Entry{}, false
	}
	for _, dir := range sortedKeys(v.dirs) {
		for _, e := range v.dirs[dir] {
			if !e.IsDir && e.FileID == file.FileID && file.Matches(e.Identity()) {
				return join(dir, e.Name), e, true
			}
		}
	}
	return "", client.Entry{}, false
}

// filesByID returns the identities of the files in the listed directories, by
// FileId, to look many files up in one listing. Files without a FileId are
// left out, as findFile does.
func (v *view) filesByID() map[uint64][]client.Identity {
	files := make(map[uint64][]client.Identity)
	for _, entries := range v.dirs {
		for _, e := range entries {
			if !e.IsDir && e.FileID != 0 {
				files[e.FileID] = append(files[e.FileID], e.Identity())
			}
		}
	}
	return files
}

// list lists the directories the pattern can match, one ListDir call per
// directory, and matches the pattern one path segment at a time.
func (s *scanner) list(ctx context.Context, errs *scanErrors) *view {
	v := &view{dirs: make(map[string][]client.Entry), matches: make(map[string]client.Entry)}
	segments := strings.Split(s.pattern, "/")
	dirs := []string{""}
	static := true // dirs is the pattern's literal prefix
	for i, segment := range segments {
		last := i == len(segments)-1
		if !last && !hasMeta(segment) {
			for j := range dirs {
				dirs[j] = join(dirs[j], segment)
			}
			continue
		}
		var next []string
		for _, dir := range dirs {
			entries, err := s.client.ListDir(ctx, dir)
			if err != nil {
				if ctx.Err() != nil {
					return v
				}
				if client.Classify(err) == client.ErrNotFound {
					// The directory does not exist: it has no files. That
					// is a configuration problem for the pattern's literal
					// prefix only; other directories can just be gone.
					v.dirs[dir] = nil
					if static {
						errs.add(client.ErrNotFound, fmt.Errorf("directory %q does not exist", dir))
					}
					continue
				}
				v.failed = true
				errs.add(client.Classify(err), err)
				continue
			}
			v.dirs[dir] = entries
			for _, e := range entries {
				if ok, _ := path.Match(segment, e.Name); !ok {
					continue
				}
				p := join(dir, e.Name)
				switch {
				case last && !e.IsDir && !s.excluded(p):
					v.matches[p] = e
				case !last && e.IsDir:
					next = append(next, p)
				}
			}
		}
		dirs = next
		static = false
	}
	return v
}

func (s *scanner) excluded(p string) bool {
	for _, exclude := range s.excludes {
		if ok, _ := path.Match(exclude, p); ok {
			return true
		}
	}
	return false
}

// scanErrors keeps the first error of a scan that the source status reports.
type scanErrors struct {
	err  error
	kind client.ErrorKind
}

func (e *scanErrors) add(kind client.ErrorKind, err error) {
	if e.err == nil {
		e.err, e.kind = err, kind
	}
}

// addFileErr records an error reading one file. A file that disappeared or is
// locked during a rotation is retried on the next scan without an error
// status.
func (e *scanErrors) addFileErr(identifier string, err error) {
	switch kind := client.Classify(err); kind {
	case client.ErrNotFound, client.ErrSharing:
		log.Debugf("SMB file %s: %v (retrying on the next scan)", identifier, err)
	default:
		e.add(kind, err)
	}
}

// fileErr records an error opening or reading the file at the matched path p.
// Like addFileErr, it tolerates a sharing violation or a missing file, which a
// rotation causes for a moment; but once the file has failed that way for
// blockedReportAfter, the error is reported: the writer may keep the file open
// without allowing reads, or a deleted file may stay listed while its writer
// keeps it open.
func (s *scanner) fileErr(p, identifier string, err error, errs *scanErrors) {
	kind := client.Classify(err)
	if kind != client.ErrNotFound && kind != client.ErrSharing {
		errs.add(kind, err)
		return
	}
	now := s.l.clock.Now()
	since, ok := s.blocked[p]
	if !ok {
		s.blocked[p] = now
		since = now
	}
	if now.Sub(since) < blockedReportAfter {
		log.Debugf("SMB file %s: %v (retrying on the next scan)", identifier, err)
		return
	}
	errs.add(kind, fmt.Errorf("%s still cannot be opened after %s: %w", identifier, blockedReportAfter, err))
}

func hasMeta(segment string) bool {
	return strings.ContainsAny(segment, `*?[\`)
}

func join(dir, name string) string {
	if dir == "" {
		return name
	}
	return dir + "/" + name
}

// dirOf returns the directory of p in the form view.dirs uses.
func dirOf(p string) string {
	if dir := path.Dir(p); dir != "." {
		return dir
	}
	return ""
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return keys
}
