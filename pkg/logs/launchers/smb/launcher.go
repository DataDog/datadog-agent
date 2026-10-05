// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package smb launches tailers for log files on SMB shares (sources of type
// smb), read over the network without mounting the share.
//
// Each source gets a scanner goroutine that lists the directories of the
// source's path pattern every poll_interval, matches the pattern client-side
// and polls one tailer per matched file. Sources that use the same share and
// account share one reconnecting SMB client.
package smb

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/benbjohnson/clock"

	"github.com/DataDog/datadog-agent/comp/logs-library/pipeline"
	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
	auditor "github.com/DataDog/datadog-agent/comp/logs/auditor/def"
	"github.com/DataDog/datadog-agent/pkg/fips"
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
)

// errFIPS is the status of smb sources in FIPS builds.
var errFIPS = errors.New("smb log sources are not supported in FIPS builds of the Agent: SMB NTLMv2 authentication requires MD4, HMAC-MD5 and RC4, which are not FIPS-approved algorithms")

// Launcher starts and stops the scanners of smb sources.
type Launcher struct {
	// closeTimeout bounds the drain of a rotated file.
	closeTimeout time.Duration

	// Test seams.
	clock          clock.Clock
	dial           client.DialFunc // nil means client.Dial
	builtForFIPS   func() bool
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
	refused      map[*sources.LogSource]bool // added but not started (invalid, FIPS)
	removedEarly map[*sources.LogSource]bool // removal delivered before the addition
	clients      map[clientKey]*sharedClient
}

// NewLauncher returns a Launcher. closeTimeout bounds how long a rotated file
// keeps being read under its new name (logs_config.close_timeout).
func NewLauncher(closeTimeout time.Duration) *Launcher {
	return &Launcher{
		closeTimeout: closeTimeout,
		clock:        clock.New(),
		builtForFIPS: fips.BuiltForFIPS,
		tailers:      tailers.NewTailerContainer[*tailer.Tailer](),
		claims:       &claims{owners: make(map[string]*scanner)},
		addedDone:    make(chan struct{}),
		removedDone:  make(chan struct{}),
		scanners:     make(map[*sources.LogSource]*scanner),
		refused:      make(map[*sources.LogSource]bool),
		removedEarly: make(map[*sources.LogSource]bool),
		clients:      make(map[clientKey]*sharedClient),
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
// their SMB calls, and the client aborts a session whose call keeps running
// after that.
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
	if _, ok := l.scanners[source]; ok || l.refused[source] {
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
	if l.builtForFIPS() {
		l.refuse(source, errFIPS)
		return
	}
	key, c := l.acquireClient(source.Config.SMB)
	s := newScanner(l, source, c, key)
	l.scanners[source] = s
	s.start(ctx)
}

func (l *Launcher) refuse(source *sources.LogSource, err error) {
	// Config validation errors never contain the password.
	log.Warnf("Not tailing smb source %s: %v", source.Name, err)
	source.Status().Error(err)
	l.refused[source] = true
}

// removeSource stops source's scanner, which stops its tailers.
func (l *Launcher) removeSource(source *sources.LogSource) {
	if l.refused[source] {
		delete(l.refused, source)
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
	for key, shared := range l.clients {
		if err := shared.client.Close(); err != nil {
			log.Debugf("Error closing SMB client: %v", err)
		}
		delete(l.clients, key)
	}
	// The tracker outlives this launcher (it survives agent restarts), so it
	// must not keep listing stopped tailers.
	for _, t := range l.tailers.All() {
		l.tailers.Remove(t)
	}
}

// clientKey identifies the sources that can share an SMB session: same
// server, share and account. The password is part of it so a source whose
// secret was refreshed does not reuse a session opened with the old one; the
// key is never printed.
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
	if err := shared.client.Close(); err != nil {
		log.Debugf("Error closing SMB client: %v", err)
	}
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

func (c *claims) release(identifier string, s *scanner) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.owners[identifier] == s {
		delete(c.owners, identifier)
	}
}
