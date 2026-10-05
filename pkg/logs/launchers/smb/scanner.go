// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package smb

import (
	"context"
	"errors"
	"fmt"
	"path"
	"regexp"
	"slices"
	"strings"
	"sync"
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
// A tailer follows one file identity (FileId). On each scan, for every path
// with an active tailer:
//   - same FileId in the listing: poll it;
//   - another FileId (rotation): the tailer becomes a drain of its file, which
//     it looks up by FileId under any listed name, and a new tailer reads the
//     path from offset 0 right away;
//   - path no longer listed: the tailer becomes a drain the same way;
//   - shorter than the offset (truncation): a new tailer reads from offset 0.
//
// A drain ends when two consecutive polls find no new data in the file, when
// closeTimeout elapses, or when its FileId is no longer listed; bytes known to
// exist but not read are then reported as missed. While it lasts, agent status
// lists the drain next to the path's new tailer.
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

	active    map[string]*tailer.Tailer // by path
	draining  []*drain
	stopping  sync.WaitGroup            // drains being stopped
	fromStart map[string]bool           // paths whose next tailer reads from offset 0
	handoffs  map[string]handoff        // paths whose next tailer resumes a drain
	patterns  map[string]*regexp.Regexp // multiline patterns of the paths' previous tailers
	conflicts map[string]bool           // paths tailed by another source, already logged
	mismatch  map[string]int            // consecutive identity changes the listing does not show
	failing   bool                      // the previous scan reported an error
	lastErr   string
}

type drain struct {
	t        *tailer.Tailer
	deadline time.Time
	caughtUp int // consecutive polls that found no new data
}

// handoff is where a drained file was left, when it sits at a path matched by
// the pattern: the path's next tailer resumes there instead of re-reading it.
type handoff struct {
	fileID uint64
	offset int64
}

func newScanner(l *Launcher, source *sources.LogSource, c client.Client, key clientKey) *scanner {
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
		fromStart: make(map[string]bool),
		handoffs:  make(map[string]handoff),
		patterns:  make(map[string]*regexp.Regexp),
		conflicts: make(map[string]bool),
		mismatch:  make(map[string]int),
		done:      make(chan struct{}),
	}
	if cfg.PollInterval > 0 {
		s.interval = time.Duration(cfg.PollInterval * float64(time.Second))
	}
	s.mode, _ = config.TailingModeFromString(source.Config.TailingMode)
	// Validate accepted the pattern; CleanPath only normalizes it.
	s.pattern, _ = client.CleanPath(source.Config.Path)
	for _, exclude := range source.Config.ExcludePaths {
		if p, err := client.CleanPath(exclude); err == nil {
			s.excludes = append(s.excludes, p)
		}
	}
	return s
}

func (s *scanner) start(ctx context.Context) {
	ctx, s.cancel = context.WithCancel(ctx)
	go s.run(ctx)
}

// Stop stops the scanner and its tailers, and returns once they have flushed.
func (s *scanner) Stop() {
	if s.cancel == nil {
		return
	}
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

// stopTailers stops every tailer of the scanner and waits for them.
func (s *scanner) stopTailers() {
	stopper := startstop.NewParallelStopper()
	for p, t := range s.active {
		s.deactivate(p, t)
		stopper.Add(t)
	}
	for _, d := range s.draining {
		s.l.tailers.Remove(d.t)
		stopper.Add(d.t)
	}
	s.draining = nil
	stopper.Stop()
	s.stopping.Wait()
}

// scan lists the share once, then polls every tailer.
func (s *scanner) scan(ctx context.Context) {
	var errs scanErrors
	v := s.list(ctx, &errs)
	if ctx.Err() != nil {
		return
	}

	for _, p := range sortedKeys(s.active) {
		t := s.active[p]
		entry, listed, known := v.lookup(p)
		switch {
		case !known:
			// Its directory could not be listed: nothing to conclude.
		case !listed:
			log.Infof("SMB file %s is no longer listed; reading the rest of FileId %d wherever it was moved", t.Identifier(), t.FileID())
			s.deactivate(p, t)
			s.startDrain(t)
			s.fromStart[p] = true
		case entry.FileID != 0 && t.FileID() != 0 && entry.FileID != t.FileID():
			log.Infof("SMB file %s rotated (FileId %d, now %d); reading the new file from the beginning", t.Identifier(), t.FileID(), entry.FileID)
			s.deactivate(p, t)
			s.startDrain(t)
			s.fromStart[p] = true // started below, in this scan
		default:
			s.poll(ctx, p, t, &entry, &errs)
		}
	}

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

// poll polls the active tailer t of path p and handles what it found.
func (s *scanner) poll(ctx context.Context, p string, t *tailer.Tailer, entry *client.Entry, errs *scanErrors) {
	outcome, err := t.Poll(ctx, entry)
	if err != nil {
		errs.addFileErr(t.Identifier(), err)
		return
	}
	switch outcome {
	case tailer.OutcomeIdentityChanged:
		if entry.FileID != 0 {
			// The file was replaced between the listing and the read. The
			// next listing shows the new FileId and the rotation is handled
			// then, with a drain of the old file.
			s.mismatch[p]++
			if s.mismatch[p] == 3 {
				log.Warnf("SMB file %s: the directory listing keeps reporting FileId %d but opening the file reports another one; the server's FileIds may be inconsistent", t.Identifier(), entry.FileID)
			}
			return
		}
		// The listing has no FileIds, so the old file cannot be found again.
		log.Infof("SMB file %s was replaced by another file; reading the new file from the beginning", t.Identifier())
		s.deactivate(p, t)
		t.RecordMissedBytes("SMB file replaced")
		t.StartDraining() // its last messages must not commit offsets
		s.stopAsync(t)
		s.fromStart[p] = true // started by the caller's loop over new paths
	case tailer.OutcomeTruncated:
		log.Infof("SMB file %s was truncated; reading it again from the beginning", t.Identifier())
		s.deactivate(p, t)
		t.RecordMissedBytes("SMB file truncated")
		// Its last messages must not commit offsets past the new content.
		t.StartDraining()
		s.stopAsync(t)
		s.fromStart[p] = true
	default:
		delete(s.mismatch, p)
	}
}

// startTailer starts a tailer for the matched path p and polls it.
func (s *scanner) startTailer(ctx context.Context, p string, entry client.Entry, errs *scanErrors) {
	if entry.FileID != 0 && s.drainingFileID(entry.FileID) {
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

	offset, fileID, ok := s.startPosition(ctx, p, identifier, entry, errs)
	if !ok {
		s.l.claims.release(identifier, s)
		return
	}
	rotated := s.fromStart[p]
	delete(s.fromStart, p)
	pattern := s.patterns[p]
	delete(s.patterns, p)

	replaceable := sources.NewReplaceableSource(s.source)
	info := status.NewInfoRegistry()
	outputChan, monitor := s.l.pipelineProvider.NextPipelineChanWithMonitor()
	t := tailer.NewTailer(&tailer.TailerOptions{
		Source:          replaceable,
		Client:          s.client,
		Host:            s.host,
		Share:           s.share,
		Path:            p,
		FileID:          fileID,
		OutputChan:      outputChan,
		CapacityMonitor: monitor,
		Decoder:         decoder.NewDecoderFromSourceWithPattern(replaceable, pattern, info),
		Info:            info,
		Rotated:         rotated,
		ChunkSize:       s.l.chunkSize,
		PollBudget:      s.l.pollBudget,
		ForceReadEvery:  s.l.forceReadEvery,
	})
	t.Start(offset)
	s.active[p] = t
	s.l.tailers.Add(t)
	s.source.AddInput(identifier)
	s.l.registry.SetTailed(identifier, true)
	s.poll(ctx, p, t, &entry, errs)
}

// startPosition returns the offset a new tailer of p starts at:
//   - where a drain of the same file ended, for a drained file now at p;
//   - 0 for a path whose previous file was replaced while being tailed;
//   - the registry offset, when it was recorded for the same FileId and is
//     not past the end of the file;
//   - 0 when the registry offset was recorded for another FileId or is past
//     the end of the file: the file was replaced or truncated while it was not
//     tailed, so all of it is new (as for file sources,
//     pkg/logs/launchers/file/position.go);
//   - otherwise 0 (start_position: beginning) or the end of the file (end).
//
// The drain's offset comes first: a path whose previous file rotated away can
// receive a file that was already drained, the previous file of another
// matched path (app.log.1 renamed to app.log.2 and app.log to app.log.1).
//
// The size and FileId come from opening the file, since listing sizes can be
// stale.
func (s *scanner) startPosition(ctx context.Context, p, identifier string, entry client.Entry, errs *scanErrors) (offset int64, fileID uint64, ok bool) {
	if h, found := s.handoffs[p]; found {
		delete(s.handoffs, p)
		if h.fileID == entry.FileID {
			return h.offset, h.fileID, true
		}
	}
	if s.fromStart[p] {
		return 0, entry.FileID, true
	}
	res, err := s.client.ReadAt(ctx, p, 0, 0)
	if err != nil {
		errs.addFileErr(identifier, err)
		return 0, 0, false
	}
	fileID = res.FileID
	if fileID == 0 {
		fileID = entry.FileID
	}
	if storedID, stored, found := tailer.DecodeOffset(s.l.registry.GetOffset(identifier)); found {
		switch {
		case storedID != 0 && fileID != 0 && storedID != fileID:
			log.Infof("SMB file %s was replaced while it was not tailed (FileId %d, now %d); reading it from the beginning", identifier, storedID, fileID)
			return 0, fileID, true
		case stored <= res.Size:
			return stored, fileID, true
		default:
			log.Infof("Stored offset %d for SMB file %s is past its end (%d bytes): it was truncated while it was not tailed; reading it from the beginning", stored, identifier, res.Size)
			return 0, fileID, true
		}
	}
	if s.mode == config.Beginning {
		return 0, fileID, true
	}
	return res.Size, fileID, true
}

// startDrain turns t, already removed from the active tailers, into a drain.
// The drain is tracked under its own ID (see tailer.GetID), so agent status
// shows it until it ends.
func (s *scanner) startDrain(t *tailer.Tailer) {
	t.StartDraining()
	s.l.tailers.Add(t)
	s.draining = append(s.draining, &drain{t: t, deadline: s.l.clock.Now().Add(s.l.closeTimeout)})
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
func (s *scanner) pollDrain(ctx context.Context, d *drain, v *view, errs *scanErrors) bool {
	t := d.t
	if !s.l.clock.Now().Before(d.deadline) {
		s.endDrain(t, v, t.Offset(), fmt.Sprintf("SMB rotation drain timed out after %s (logs_config.close_timeout)", s.l.closeTimeout))
		return false
	}
	at, entry, found := v.findFileID(t.FileID())
	if !found {
		if v.failed {
			return true // a directory could not be listed: it may be there
		}
		s.endDrain(t, v, t.Offset(), "Rotated SMB file is no longer listed")
		return false
	}
	t.SetReadPath(at)
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
		s.endDrain(t, v, 0, "")
		return false
	case t.CaughtUp() && t.Offset() == offset:
		// Nothing new since the previous poll: the writer may be done.
		d.caughtUp++
		if d.caughtUp >= drainCaughtUpPolls {
			s.endDrain(t, v, t.Offset(), "")
			return false
		}
		return true
	default:
		d.caughtUp = 0
		return true
	}
}

// endDrain stops the drained tailer t. If its file sits at a path the pattern
// matches (another one, or its own path again after a rename back), that
// path's next tailer resumes at offset, so nothing is lost or read twice.
// Otherwise the bytes t did not read are reported as missed, with reason.
func (s *scanner) endDrain(t *tailer.Tailer, v *view, offset int64, reason string) {
	at := t.ReadPath()
	if e, ok := v.matches[at]; ok && e.FileID == t.FileID() && s.active[at] == nil {
		s.handoffs[at] = handoff{fileID: t.FileID(), offset: offset}
	} else if reason != "" {
		t.RecordMissedBytes(reason)
	}
	s.l.tailers.Remove(t)
	s.stopAsync(t)
}

// drainingFileID reports whether a drain reads the file fileID.
func (s *scanner) drainingFileID(fileID uint64) bool {
	for _, d := range s.draining {
		if d.t.FileID() == fileID {
			return true
		}
	}
	return false
}

// deactivate removes t, the active tailer of p, from the active tailers and
// releases its identifier. It does not stop t.
func (s *scanner) deactivate(p string, t *tailer.Tailer) {
	delete(s.active, p)
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

// findFileID looks for the file fileID under any name in the listed
// directories.
func (v *view) findFileID(fileID uint64) (string, client.Entry, bool) {
	if fileID == 0 {
		return "", client.Entry{}, false
	}
	for _, dir := range sortedKeys(v.dirs) {
		for _, e := range v.dirs[dir] {
			if !e.IsDir && e.FileID == fileID {
				return join(dir, e.Name), e, true
			}
		}
	}
	return "", client.Entry{}, false
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
