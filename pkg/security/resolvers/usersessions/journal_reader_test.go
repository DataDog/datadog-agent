// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package usersessions

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/hashicorp/golang-lru/v2/simplelru"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// MockJournal est un mock de Journal pour les tests
type MockJournal struct {
	entries       []*JournalEntry
	currentIndex  int
	cursor        string
	closed        bool
	mu            sync.Mutex
	addMatchError error
}

// NewMockJournal creates a new MockJournal
func NewMockJournal(entries []*JournalEntry) *MockJournal {
	return &MockJournal{
		entries:      entries,
		currentIndex: -1,
	}
}

func (m *MockJournal) AddMatch(match string) error {
	if m.addMatchError != nil {
		return m.addMatchError
	}
	return nil
}

func (m *MockJournal) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.closed = true
	return nil
}

func (m *MockJournal) GetCursor() (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.currentIndex < 0 || m.currentIndex >= len(m.entries) {
		return "", fmt.Errorf("invalid index")
	}
	return fmt.Sprintf("cursor-%d", m.currentIndex), nil
}

func (m *MockJournal) GetEntry() (*JournalEntry, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.currentIndex < 0 || m.currentIndex >= len(m.entries) {
		return nil, fmt.Errorf("invalid index")
	}
	return m.entries[m.currentIndex], nil
}

func (m *MockJournal) Next() (uint64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.currentIndex++
	if m.currentIndex >= len(m.entries) {
		return 0, nil
	}
	return 1, nil
}

func (m *MockJournal) NextSkip(skip uint64) (uint64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.currentIndex += int(skip)
	if m.currentIndex >= len(m.entries) {
		return 0, nil
	}
	return 1, nil
}

func (m *MockJournal) SeekCursor(cursor string) error {
	var index int
	_, err := fmt.Sscanf(cursor, "cursor-%d", &index)
	if err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.currentIndex = index
	return nil
}

func (m *MockJournal) SeekHead() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.currentIndex = -1
	return nil
}

func (m *MockJournal) SeekTail() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.currentIndex = len(m.entries) - 1
	return nil
}

func (m *MockJournal) Wait(timeout time.Duration) int {
	// Simuler l'attente
	time.Sleep(10 * time.Millisecond)
	return 0
}

func TestJournalReader_Start(t *testing.T) {
	entries := []*JournalEntry{
		{
			Fields: map[string]string{
				"MESSAGE":   "Accepted publickey for testuser from 127.0.0.1 port 22 ssh2: RSA SHA256:test",
				"_HOSTNAME": "localhost",
				"_PID":      "1234",
			},
			RealtimeTimestamp: uint64(time.Now().UnixNano() / 1000),
		},
	}

	mockJournal := NewMockJournal(entries)
	parser := func(line string, sshSessionParsed *sshSessionParsed) {
		// Parser de test
	}

	reader := NewJournalReader(mockJournal, parser)
	err := reader.Start("")
	require.NoError(t, err)

	// Vérifier que le journal est positionné à la fin
	assert.Equal(t, len(entries)-1, mockJournal.currentIndex)
}

func TestJournalReader_ReadEntries(t *testing.T) {
	entries := []*JournalEntry{
		{
			Fields: map[string]string{
				"MESSAGE":   "Accepted publickey for testuser from 127.0.0.1 port 22222 ssh2: ED25519 SHA256:testkey123",
				"_HOSTNAME": "localhost",
				"_PID":      "1234",
			},
			RealtimeTimestamp: uint64(time.Now().UnixNano() / 1000),
		},
	}

	mockJournal := NewMockJournal(entries)

	// Créer un LRU pour les sessions SSH
	lru, err := simplelru.NewLRU[SSHSessionKey, SSHSessionValue](100, nil)
	require.NoError(t, err)

	sshParsed := &sshSessionParsed{
		Lru: lru,
	}

	reader := NewJournalReader(mockJournal, parseSSHLogLine)
	err = reader.Start("")
	require.NoError(t, err)

	// Lire les entrées
	err = reader.ReadEntries(sshParsed)
	require.NoError(t, err)

	// Vérifier que la session SSH a été parsée et ajoutée au cache
	key := SSHSessionKey{
		IP:   "127.0.0.1",
		Port: "22222",
	}
	value, ok := sshParsed.Lru.Get(key)
	assert.True(t, ok, "La session SSH devrait être dans le cache")
	assert.NotEmpty(t, value.PublicKey, "La clé publique devrait être extraite")
}

func TestJournalReader_Stop(t *testing.T) {
	entries := []*JournalEntry{}
	mockJournal := NewMockJournal(entries)

	parser := func(line string, sshSessionParsed *sshSessionParsed) {}
	reader := NewJournalReader(mockJournal, parser)

	err := reader.Start("")
	require.NoError(t, err)

	reader.Stop()

	// Vérifier que le journal est fermé
	assert.True(t, mockJournal.closed, "Le journal devrait être fermé")
}

func TestJournalReader_SeekWithCursor(t *testing.T) {
	entries := []*JournalEntry{
		{
			Fields: map[string]string{
				"MESSAGE": "Entry 1",
			},
			RealtimeTimestamp: uint64(time.Now().UnixNano() / 1000),
		},
		{
			Fields: map[string]string{
				"MESSAGE": "Entry 2",
			},
			RealtimeTimestamp: uint64(time.Now().UnixNano() / 1000),
		},
	}

	mockJournal := NewMockJournal(entries)
	parser := func(line string, sshSessionParsed *sshSessionParsed) {}

	reader := NewJournalReader(mockJournal, parser)

	// Démarrer avec un cursor
	err := reader.Start("cursor-0")
	require.NoError(t, err)

	// Vérifier que le journal a sauté une entrée (NextSkip(1))
	assert.Equal(t, 1, mockJournal.currentIndex)
}

func TestJournalReader_FormatLogLine(t *testing.T) {
	entry := &JournalEntry{
		Fields: map[string]string{
			"MESSAGE":   "Accepted publickey for testuser from 127.0.0.1 port 22 ssh2",
			"_HOSTNAME": "testhost",
			"_PID":      "5678",
		},
		RealtimeTimestamp: uint64(time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC).UnixNano() / 1000),
	}

	mockJournal := NewMockJournal([]*JournalEntry{})
	parser := func(line string, sshSessionParsed *sshSessionParsed) {}
	reader := NewJournalReader(mockJournal, parser)

	line := reader.formatLogLine(entry)

	// Vérifier que la ligne est formatée correctement
	assert.Contains(t, line, "testhost")
	assert.Contains(t, line, "sshd[5678]")
	assert.Contains(t, line, "Accepted publickey for testuser from 127.0.0.1 port 22 ssh2")
}
