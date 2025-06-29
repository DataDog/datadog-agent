// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package impl

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/log"
	"github.com/DataDog/datadog-agent/comp/remote-shell/remoteshell/def"
	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
	"github.com/gorilla/websocket"
)

type remoteShell struct {
	config     config.Component
	log        log.Component
	sessions   map[string]*session
	sessionsMu sync.RWMutex
	ctx        context.Context
	cancel     context.CancelFunc
}

type session struct {
	id           string
	podName      string
	namespace    string
	container    string
	websocketURI string
	expiresAt    int64
	metadata     map[string]string
	conn         *websocket.Conn
	writeMu      sync.Mutex
}

// NewComponent creates a new remote shell component
func NewComponent(cfg config.Component, l log.Component) def.Component {
	ctx, cancel := context.WithCancel(context.Background())
	return &remoteShell{
		config:   cfg,
		log:      l,
		sessions: make(map[string]*session),
		ctx:      ctx,
		cancel:   cancel,
	}
}

// Start starts the remote shell component
func (r *remoteShell) Start() {
	// Start a goroutine to clean up expired sessions
	go r.cleanupExpiredSessions()
}

// Stop stops the remote shell component
func (r *remoteShell) Stop() {
	r.cancel()
	r.sessionsMu.Lock()
	defer r.sessionsMu.Unlock()
	for _, s := range r.sessions {
		s.Close()
	}
}

// GetActiveSessions returns a map of active shell sessions
func (r *remoteShell) GetActiveSessions() map[string]def.Session {
	r.sessionsMu.RLock()
	defer r.sessionsMu.RUnlock()
	sessions := make(map[string]def.Session)
	for id, s := range r.sessions {
		sessions[id] = s
	}
	return sessions
}

// cleanupExpiredSessions periodically checks for and removes expired sessions
func (r *remoteShell) cleanupExpiredSessions() {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-r.ctx.Done():
			return
		case <-ticker.C:
			r.sessionsMu.Lock()
			now := time.Now().Unix()
			for id, s := range r.sessions {
				if s.expiresAt < now {
					s.Close()
					delete(r.sessions, id)
				}
			}
			r.sessionsMu.Unlock()
		}
	}
}

// GetID returns the session ID
func (s *session) GetID() string {
	return s.id
}

// GetPodName returns the pod name
func (s *session) GetPodName() string {
	return s.podName
}

// GetNamespace returns the namespace
func (s *session) GetNamespace() string {
	return s.namespace
}

// GetContainer returns the container name
func (s *session) GetContainer() string {
	return s.container
}

// GetWebsocketURI returns the websocket URI
func (s *session) GetWebsocketURI() string {
	return s.websocketURI
}

// GetExpiresAt returns the expiration timestamp
func (s *session) GetExpiresAt() int64 {
	return s.expiresAt
}

// GetMetadata returns the session metadata
func (s *session) GetMetadata() map[string]string {
	return s.metadata
}

// Write writes data to the shell session
func (s *session) Write(data []byte) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	return s.conn.WriteMessage(websocket.TextMessage, data)
}

// Read reads data from the shell session
func (s *session) Read() ([]byte, error) {
	_, message, err := s.conn.ReadMessage()
	return message, err
}

// Close closes the shell session
func (s *session) Close() error {
	return s.conn.Close()
}

// HandleRemoteShellConfig handles a new remote shell configuration
func (r *remoteShell) HandleRemoteShellConfig(config state.RemoteShellConfig) error {
	r.sessionsMu.Lock()
	defer r.sessionsMu.Unlock()

	// Check if session already exists
	if _, exists := r.sessions[config.Config.SessionID]; exists {
		return fmt.Errorf("session %s already exists", config.Config.SessionID)
	}

	// Create websocket connection
	conn, _, err := websocket.DefaultDialer.Dial(config.Config.WebsocketURI, nil)
	if err != nil {
		return fmt.Errorf("failed to connect to websocket: %w", err)
	}

	// Create new session
	s := &session{
		id:           config.Config.SessionID,
		podName:      config.Config.PodName,
		namespace:    config.Config.Namespace,
		container:    config.Config.Container,
		websocketURI: config.Config.WebsocketURI,
		expiresAt:    config.Config.ExpiresAt,
		metadata:     config.Config.Metadata,
		conn:         conn,
	}

	// Add session to map
	r.sessions[config.Config.SessionID] = s

	return nil
}
