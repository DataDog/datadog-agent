// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build smb && !goexperiment.systemcrypto && !goexperiment.boringcrypto && !requirefips

// Package smb launches tailers for log files on SMB shares (sources of type
// smb), read over the network without mounting the share.
//
// Each source gets a scanner goroutine that lists the directories of the
// source's path pattern every poll_interval, matches the pattern client-side
// and polls one tailer per matched file. Sources that use the same share and
// account share one reconnecting SMB client.
//
// The SMB log source is built into the full Agent on Linux, Windows and macOS
// only (the smb build tag) and never into FIPS builds, whose tags are negated
// here as in pkg/fips.BuiltForFIPS. Other builds use launcher_nosmb.go, which
// reports each smb source as unsupported.
package smb

import (
	"context"
	"sync"
	"time"

	"github.com/benbjohnson/clock"

	"github.com/DataDog/datadog-agent/comp/logs-library/pipeline"
	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
	auditor "github.com/DataDog/datadog-agent/comp/logs/auditor/def"
	"github.com/DataDog/datadog-agent/pkg/logs/internal/smb/client"
	"github.com/DataDog/datadog-agent/pkg/logs/launchers"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
	"github.com/DataDog/datadog-agent/pkg/logs/tailers"
	tailer "github.com/DataDog/datadog-agent/pkg/logs/tailers/smb"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/startstop"
)

const (
	// defaultPollInterval applies when a source sets no poll_interval.
	defaultPollInterval = time.Second
	// drainCaughtUpPolls is how many consecutive polls must find no new data
	// in a rotated file, already read to its end, before its drain ends.
	drainCaughtUpPolls = 2
	// blockedReportAfter is how long a matched file may keep failing to open
	// with a sharing violation or a not found error, as it does for a moment
	// during a rotation, before the source status reports it.
	blockedReportAfter = 30 * time.Second
)

// Launcher starts and stops the scanners of smb sources.
type Launcher struct {
	// closeTimeout bounds the drain of a rotated file.
	closeTimeout time.Duration

	// Test seams.
	clock          clock.Clock
	dial           client.DialFunc // nil means client.Dial
	chunkSize      int
	pollBudget     int
	forceReadEvery int

	pipelineProvider pipeline.Provider
	registry         auditor.Registry
	tailers          *tailers.TailerContainer[*tailer.Tailer]
	claims           *claims

	cancel      context.CancelFunc
	stopped     chan struct{}
	addedDone   chan struct{}
	removedDone chan struct{}
	stopOnce    sync.Once

	// Owned by the run goroutine.
	scanners     map[*sources.LogSource]*scanner
	refused      map[*sources.LogSource]bool // added but not started (invalid)
	replaced     map[*sources.LogSource]bool // stopped for a newer source of the same configuration
	removedEarly map[*sources.LogSource]bool // removal delivered before the addition
	clients      map[clientKey]*sharedClient
	closing      map[client.Client]chan struct{} // clients logging off in the background; the channel closes when done
}

// NewLauncher returns a Launcher. closeTimeout bounds how long a rotated file
// keeps being read under its new name (logs_config.close_timeout).
func NewLauncher(closeTimeout time.Duration) *Launcher {
	return &Launcher{
		closeTimeout: closeTimeout,
		clock:        clock.New(),
		tailers:      tailers.NewTailerContainer[*tailer.Tailer](),
		claims:       &claims{owners: make(map[string]*scanner)},
		addedDone:    make(chan struct{}),
		removedDone:  make(chan struct{}),
		scanners:     make(map[*sources.LogSource]*scanner),
		refused:      make(map[*sources.LogSource]bool),
		replaced:     make(map[*sources.LogSource]bool),
		removedEarly: make(map[*sources.LogSource]bool),
		clients:      make(map[clientKey]*sharedClient),
		closing:      make(map[client.Client]chan struct{}),
	}
}

// Start implements launchers.Launcher. It does no network I/O: each source's
// scanner connects on its own goroutine.
func (l *Launcher) Start(sourceProvider launchers.SourceProvider, pipelineProvider pipeline.Provider, registry auditor.Registry, tracker *tailers.TailerTracker) {
	l.pipelineProvider = pipelineProvider
	l.registry = registry
	if tracker != nil {
		tracker.Add(l.tailers)
	}
	ctx, cancel := context.WithCancel(context.Background())
	l.cancel = cancel
	l.stopped = make(chan struct{})
	added, removed := sourceProvider.SubscribeForType(config.SMBType, l.addedDone, l.removedDone)
	go l.run(ctx, added, removed)
}

// Stop implements launchers.Launcher. It stops every scanner and tailer and
// closes the SMB clients. It is safe to call when Start never ran.
//
// Stop does not wait for the server: cancelling the scanners' context ends
// their SMB calls, the client aborts a session whose call keeps running after
// that, and the sessions are then aborted rather than logged off.
func (l *Launcher) Stop() {
	l.stopOnce.Do(func() {
		if l.cancel == nil {
			return
		}
		close(l.addedDone)
		close(l.removedDone)
		l.cancel()
		<-l.stopped
	})
}

func (l *Launcher) run(ctx context.Context, added, removed chan *sources.LogSource) {
	defer close(l.stopped)
	for {
		select {
		case source := <-added:
			l.addSource(ctx, source)
		case source := <-removed:
			l.removeSource(source)
		case <-ctx.Done():
			l.stopAll()
			return
		}
	}
}

// addSource starts a scanner for source. Sources replayed by the subscription
// skipped validation, so every source is validated again here.
func (l *Launcher) addSource(ctx context.Context, source *sources.LogSource) {
	if _, ok := l.scanners[source]; ok || l.refused[source] || l.replaced[source] {
		return
	}
	if l.removedEarly[source] {
		delete(l.removedEarly, source)
		return
	}
	if err := source.Config.Validate(); err != nil {
		l.refuse(source, err)
		return
	}
	key, c := l.acquireClient(source.Config.SMB)
	s, err := newScanner(l, source, c, key)
	if err != nil {
		l.releaseClient(key)
		l.refuse(source, err)
		return
	}
	for _, previous := range l.previousSources(source) {
		l.replace(previous, s)
	}
	l.scanners[source] = s
	s.start(ctx)
}

// previousSources returns the running and refused sources that source
// replaces: those created from the same configuration entry (the same item of
// the same integration config). Autodiscovery schedules a conf.d config again
// without unscheduling it when its secrets are refreshed (the logs scheduler
// cannot unschedule a config without a service), so the refreshed source,
// with the new password, arrives next to the old one.
func (l *Launcher) previousSources(source *sources.LogSource) []*sources.LogSource {
	entry, ok := configEntryOf(source)
	if !ok {
		return nil
	}
	var previous []*sources.LogSource
	for candidate := range l.scanners {
		if e, ok := configEntryOf(candidate); ok && e == entry {
			previous = append(previous, candidate)
		}
	}
	for candidate := range l.refused {
		if e, ok := configEntryOf(candidate); ok && e == entry {
			previous = append(previous, candidate)
		}
	}
	return previous
}

// replace stops previous, which next (not started yet) replaces: next resumes
// previous's files where previous stopped reading them, and previous is hidden
// from agent status. A later removal of previous is ignored.
func (l *Launcher) replace(previous *sources.LogSource, next *scanner) {
	log.Infof("SMB source %s was configured again (for example after a secret refresh): the new configuration replaces the previous one", previous.Name)
	if s, ok := l.scanners[previous]; ok {
		delete(l.scanners, previous)
		s.Stop() // releases its files, so next can claim them
		next.resumeFrom(s)
		l.releaseClient(s.clientKey)
	}
	delete(l.refused, previous)
	l.replaced[previous] = true
	previous.HideFromStatus()
}

func (l *Launcher) refuse(source *sources.LogSource, err error) {
	// Config validation errors never contain the password.
	log.Warnf("Not tailing smb source %s: %v", source.Name, err)
	source.Status().Error(err)
	l.refused[source] = true
}

// removeSource stops source's scanner, which stops its tailers.
func (l *Launcher) removeSource(source *sources.LogSource) {
	if l.refused[source] || l.replaced[source] {
		delete(l.refused, source)
		delete(l.replaced, source)
		return
	}
	s, ok := l.scanners[source]
	if !ok {
		l.removedEarly[source] = true
		return
	}
	delete(l.scanners, source)
	s.Stop()
	l.releaseClient(s.clientKey)
}

func (l *Launcher) stopAll() {
	stopper := startstop.NewParallelStopper()
	for source, s := range l.scanners {
		stopper.Add(s)
		delete(l.scanners, source)
	}
	stopper.Stop()
	// Abort the sessions rather than log them off, which can take seconds per
	// session with a server that stopped answering, and cut short the logoffs
	// of removed sources' sessions. No handle outlives an SMB call, so the
	// server has nothing to clean up but the sessions, which it drops with
	// their connections.
	for key, shared := range l.clients {
		if err := client.Abort(shared.client); err != nil {
			log.Debugf("Error closing SMB client: %v", err)
		}
		delete(l.clients, key)
	}
	for c, done := range l.closing {
		_ = client.Abort(c)
		<-done
		delete(l.closing, c)
	}
	// The tracker outlives this launcher (it survives agent restarts), so it
	// must not keep listing stopped tailers.
	for _, t := range l.tailers.All() {
		l.tailers.Remove(t)
	}
}

// clientKey identifies the sources that can share an SMB session: same
// server, share and account. The password is part of it so a source whose
// secret was refreshed (see previousSources) does not reuse a session opened
// with the old one; the key is never printed.
type clientKey struct {
	host     string
	port     int
	share    string
	username string
	domain   string
	password string
}

type sharedClient struct {
	client client.Client
	refs   int
}

func (l *Launcher) acquireClient(cfg *config.SMBConfig) (clientKey, client.Client) {
	key := clientKey{host: cfg.Host, port: cfg.Port, share: cfg.Share, username: cfg.Username, domain: cfg.Domain, password: cfg.Password}
	if key.port == 0 {
		key.port = 445
	}
	if shared, ok := l.clients[key]; ok {
		shared.refs++
		return key, shared.client
	}
	opts := []client.Option{client.WithClock(l.clock)}
	if l.dial != nil {
		opts = append(opts, client.WithDialer(l.dial))
	}
	c := client.NewReconnecting(client.Config{
		Host:     cfg.Host,
		Share:    cfg.Share,
		Username: cfg.Username,
		Password: cfg.Password,
		Domain:   cfg.Domain,
		Port:     cfg.Port,
	}, opts...)
	l.clients[key] = &sharedClient{client: c, refs: 1}
	return key, c
}

func (l *Launcher) releaseClient(key clientKey) {
	shared, ok := l.clients[key]
	if !ok {
		return
	}
	shared.refs--
	if shared.refs > 0 {
		return
	}
	delete(l.clients, key)
	for c, done := range l.closing {
		select {
		case <-done:
			delete(l.closing, c)
		default:
		}
	}
	// Log off in the background: the run goroutine also serves source
	// additions and removals, which must not wait for the server. stopAll
	// cuts the logoff short.
	done := make(chan struct{})
	l.closing[shared.client] = done
	go func() {
		defer close(done)
		if err := shared.client.Close(); err != nil {
			log.Debugf("Error closing SMB client: %v", err)
		}
	}()
}

// claims makes sure two sources never tail the same file: two tailers
// committing offsets to one registry identifier would move it back and forth.
type claims struct {
	mu     sync.Mutex
	owners map[string]*scanner
}

// claim reports whether s owns identifier, making it the owner if it has none.
func (c *claims) claim(identifier string, s *scanner) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	owner, ok := c.owners[identifier]
	if !ok {
		c.owners[identifier] = s
		return true
	}
	return owner == s
}

// ownedByAnother reports whether a scanner other than s owns identifier.
func (c *claims) ownedByAnother(identifier string, s *scanner) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	owner, ok := c.owners[identifier]
	return ok && owner != s
}

func (c *claims) release(identifier string, s *scanner) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.owners[identifier] == s {
		delete(c.owners, identifier)
	}
}
