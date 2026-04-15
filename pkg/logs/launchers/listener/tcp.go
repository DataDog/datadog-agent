// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package listener

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"slices"
	"strconv"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/log"

	"github.com/DataDog/datadog-agent/comp/logs-library/pipeline"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
	tailer "github.com/DataDog/datadog-agent/pkg/logs/tailers/socket"
	"github.com/DataDog/datadog-agent/pkg/util/startstop"
)

const (
	defaultTLSIdleTimeout = 60 * time.Second
	defaultMaxConnections = 512
	tlsHandshakeTimeout   = 10 * time.Second
)

// A TCPListener listens and accepts TCP connections and delegates the read operations to a tailer.
type TCPListener struct {
	pipelineProvider pipeline.Provider
	source           *sources.LogSource
	idleTimeout      time.Duration
	frameSize        int
	maxConnections   int
	tlsCredentials   *tls.Config
	listener         net.Listener
	tailers          []*tailer.Tailer
	mu               sync.Mutex
	stopped          bool
	connSem          chan struct{}
	stop             chan struct{}
	ctx              context.Context
	cancel           context.CancelFunc
}

// NewTCPListener returns an initialized TCPListener or an error if critical
// configuration (TLS credentials) fails to build.
func NewTCPListener(pipelineProvider pipeline.Provider, source *sources.LogSource, frameSize int) (*TCPListener, error) {
	var idleTimeout time.Duration
	if source.Config.IdleTimeout != "" {
		var err error
		idleTimeout, err = time.ParseDuration(source.Config.IdleTimeout)
		if err != nil {
			log.Errorf("Error parsing log's idle_timeout as a duration: %s", err)
			idleTimeout = 0
		}
	}

	var tlsCreds *tls.Config
	ctx, cancel := context.WithCancel(context.Background())
	if source.Config.TLS != nil {
		var err error
		tlsCreds, err = source.Config.TLS.BuildTLSConfig(ctx)
		if err != nil {
			cancel()
			return nil, fmt.Errorf("failed to load TLS credentials for TCP listener on port %d: %w", source.Config.Port, err)
		}
		if idleTimeout == 0 {
			idleTimeout = defaultTLSIdleTimeout
		}
	}

	maxConns := source.Config.MaxConnections
	if maxConns <= 0 {
		maxConns = defaultMaxConnections
	}

	return &TCPListener{
		pipelineProvider: pipelineProvider,
		source:           source,
		idleTimeout:      idleTimeout,
		frameSize:        frameSize,
		maxConnections:   maxConns,
		tlsCredentials:   tlsCreds,
		tailers:          []*tailer.Tailer{},
		connSem:          make(chan struct{}, maxConns),
		stop:             make(chan struct{}, 1),
		ctx:              ctx,
		cancel:           cancel,
	}, nil
}

// Start starts the listener to accepts new incoming connections.
func (l *TCPListener) Start() {
	tlsLabel := ""
	if l.tlsCredentials != nil {
		tlsLabel = "+TLS"
	}
	log.Infof("Starting TCP%s forwarder on port %d, with read buffer size: %d", tlsLabel, l.source.Config.Port, l.frameSize)
	err := l.startListener()
	if err != nil {
		log.Errorf("Can't start TCP%s forwarder on port %d: %v", tlsLabel, l.source.Config.Port, err)
		l.source.Status.Error(err)
		return
	}
	l.source.Status.Success()
	go l.run()
}

// Stop stops the listener from accepting new connections and all the activer tailers.
func (l *TCPListener) Stop() {
	log.Infof("Stopping TCP forwarder on port %d", l.source.Config.Port)
	l.mu.Lock()
	defer l.mu.Unlock()
	l.stopped = true
	l.stop <- struct{}{}
	l.cancel()
	if l.listener != nil {
		l.listener.Close()
	}
	stopper := startstop.NewParallelStopper()
	for _, tailer := range l.tailers {
		stopper.Add(tailer)
	}
	stopper.Stop()

	// At this point all the tailers have been stopped - remove them all from the active tailer list
	l.tailers = []*tailer.Tailer{}
}

// run accepts new TCP connections and create a dedicated tailer for each.
func (l *TCPListener) run() {
	defer l.listener.Close()
	for {
		select {
		case <-l.stop:
			return
		default:
			conn, err := l.listener.Accept()
			switch {
			case err != nil && isClosedConnError(err):
				return
			case err != nil:
				log.Warnf("Can't listen on port %d, restarting a listener: %v", l.source.Config.Port, err)
				l.listener.Close()
				err := l.startListener()
				if err != nil {
					log.Errorf("Can't restart listener on port %d: %v", l.source.Config.Port, err)
					l.source.Status.Error(err)
					return
				}
				l.source.Status.Success()
				continue
			default:
				select {
				case l.connSem <- struct{}{}:
					go l.handleConnection(conn)
				default:
					log.Warnf("Max connections (%d) reached on port %d, rejecting connection from %s",
						l.maxConnections, l.source.Config.Port, conn.RemoteAddr())
					conn.Close()
				}
			}
		}
	}
}

// startListener starts a new listener, returns an error if it failed.
func (l *TCPListener) startListener() error {
	bindAddr := net.JoinHostPort(l.source.Config.BindHost, strconv.Itoa(l.source.Config.Port))
	listener, err := net.Listen("tcp", bindAddr)
	if err != nil {
		return err
	}
	if l.tlsCredentials != nil {
		listener = tls.NewListener(listener, l.tlsCredentials)
	}
	l.listener = listener
	return nil
}

// read reads data from connection, returns an error if it failed and stop the tailer.
func (l *TCPListener) read(tailer *tailer.Tailer) ([]byte, string, error) {
	if l.idleTimeout > 0 {
		tailer.Conn.SetReadDeadline(time.Now().Add(l.idleTimeout)) //nolint:errcheck
	}
	frame := make([]byte, l.frameSize)
	n, err := tailer.Conn.Read(frame)
	if err != nil {
		log.Debugf("Connection error on port %d from %s: %v", l.source.Config.Port, tailer.Conn.RemoteAddr(), err)
		go l.stopTailer(tailer)
		return nil, "", err
	}
	return frame[:n], tailer.Conn.RemoteAddr().String(), nil
}

// handleConnection performs the TLS handshake (if applicable) outside of any
// mutex, then registers the tailer. This prevents a slow or malicious client
// from blocking the accept loop.
func (l *TCPListener) handleConnection(conn net.Conn) {
	if tlsConn, ok := conn.(*tls.Conn); ok {
		ctx, cancel := context.WithTimeout(l.ctx, tlsHandshakeTimeout)
		defer cancel()
		if err := tlsConn.HandshakeContext(ctx); err != nil {
			log.Warnf("TLS handshake failed on port %d from %s: %v", l.source.Config.Port, conn.RemoteAddr(), err)
			conn.Close()
			<-l.connSem
			return
		}
	}

	l.mu.Lock()
	if l.stopped {
		l.mu.Unlock()
		conn.Close()
		<-l.connSem
		return
	}
	t := tailer.NewTailer(l.source, conn, l.pipelineProvider.NextPipelineChan(), l.read)
	l.tailers = append(l.tailers, t)
	l.mu.Unlock()
	t.Start()
	l.source.Status.Success()
}

// stopTailer stops the tailer.
func (l *TCPListener) stopTailer(tailer *tailer.Tailer) {
	l.mu.Lock()
	defer l.mu.Unlock()
	for i, t := range l.tailers {
		if t == tailer {
			// Only stop the tailer if it has not already been stopped
			tailer.Stop()
			l.tailers = slices.Delete(l.tailers, i, i+1)
			<-l.connSem
			break
		}
	}
}
