// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package usersessions

import (
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/security/seclog"
)

const (
	defaultWaitDuration       = 1 * time.Second
	maxLinesToProcessPerCycle = 10000
)

// JournalReader manages the reading of the journal for SSH sessions
type sshJournalReader struct {
	journal     Journal
	cursor      string
	mu          sync.Mutex
	stopReading chan struct{}

	parser func(line string, sshSessionParsed *sshSessionParsed)
}

// NewJournalReader crée un nouveau lecteur de journal
func NewsshJournalReader(journal Journal, parser func(line string, sshSessionParsed *sshSessionParsed)) *sshJournalReader {
	return &sshJournalReader{
		journal:     journal,
		stopReading: make(chan struct{}, 1),
		parser:      parser,
	}
}

// Start démarre la lecture du journal depuis un cursor donné
func (jr *sshJournalReader) Start(cursor string) error {
	jr.mu.Lock()
	defer jr.mu.Unlock()

	// Configure les filtres pour sshd uniquement
	if err := jr.journal.AddMatch("_COMM=sshd"); err != nil {
		return fmt.Errorf("impossible d'ajouter le filtre _COMM=sshd: %w", err)
	}

	// Positionne le journal
	if err := jr.seek(cursor); err != nil {
		return fmt.Errorf("impossible de se positionner dans le journal: %w", err)
	}

	seclog.Debugf("Démarrage du lecteur de journal systemd pour les sessions SSH")
	return nil
}

// seek positionne le journal au cursor approprié
func (jr *sshJournalReader) seek(cursor string) error {
	if cursor != "" {
		// Reprendre depuis le dernier cursor connu
		if err := jr.journal.SeekCursor(cursor); err != nil {
			seclog.Warnf("Impossible de se positionner au cursor %s: %v, démarrage depuis la fin", cursor, err)
			return jr.seekTail()
		}
		// Sauter une entrée car le cursor pointe sur la dernière entrée lue
		if _, err := jr.journal.NextSkip(1); err != nil {
			return err
		}
		jr.cursor = cursor
		return nil
	}

	// Pas de cursor, commencer depuis la fin du journal
	return jr.seekTail()
}

// seekTail positionne le journal à la fin
func (jr *sshJournalReader) seekTail() error {
	if err := jr.journal.SeekTail(); err != nil {
		return err
	}
	// SeekTail doit être suivi de Next()
	_, err := jr.journal.Next()
	return err
}

// Stop stop the journal reader
// Lock jr.mu must be free before calling
func (jr *sshJournalReader) Stop() {
	fmt.Printf("Stoppin1\n")
	if jr == nil {
		return
	}
	jr.mu.Lock()
	defer jr.mu.Unlock()

	if jr.journal != nil {
		jr.journal.Close()
		jr.journal = nil
	}
}

// ReadEntries reads the entries from the journal and processes them
// The lock jr.mu must be held before calling
func (jr *sshJournalReader) ReadEntries(sshSessionParsed *sshSessionParsed) error {
	if jr == nil {
		return nil
	}
	fmt.Printf("ReadEntries\n")
	if jr.journal == nil {
		return fmt.Errorf("le journal est nil")
	}

	linesProcessed := 0

	for linesProcessed < maxLinesToProcessPerCycle {
		// Lire la prochaine entrée
		n, err := jr.journal.Next()
		if err != nil && err != io.EOF {
			return fmt.Errorf("erreur lors de la lecture du journal: %w", err)
		}

		// Pas de nouvelle entrée disponible
		if n == 0 {
			// Attendre de nouvelles entrées
			jr.journal.Wait(defaultWaitDuration)
			break
		}

		linesProcessed++

		// Récupérer l'entrée
		entry, err := jr.journal.GetEntry()
		if err != nil {
			seclog.Warnf("Impossible de récupérer l'entrée du journal: %v", err)
			continue
		}

		// Traiter l'entrée
		line := jr.formatLogLine(entry)
		jr.parser(line, sshSessionParsed)
	}

	// Sauvegarder le cursor pour reprendre plus tard
	if cursor, err := jr.journal.GetCursor(); err == nil {
		jr.cursor = cursor
	} else {
		seclog.Debugf("Impossible d'obtenir le cursor actuel: %v", err)
	}

	return nil
}

// formatLogLine formate une entrée de journal dans le format attendu par le parser
func (jr *sshJournalReader) formatLogLine(entry *JournalEntry) string {
	message := entry.Fields["MESSAGE"]
	if message == "" {
		return ""
	}

	hostname := entry.Fields["_HOSTNAME"]
	if hostname == "" {
		hostname = "localhost"
	}

	pid := entry.Fields["_PID"]
	if pid == "" {
		pid = "0"
	}

	// Convertir le timestamp
	timestamp := time.Unix(0, int64(entry.RealtimeTimestamp)*1000)
	dateStr := timestamp.Format("2006-01-02T15:04:05-0700")

	// Construire la ligne de log dans le format attendu par parseSSHLogLine
	return fmt.Sprintf("%s %s sshd[%s]: %s",
		dateStr,
		hostname,
		pid,
		message,
	)
}

// GetCursor retourne le cursor actuel
func (jr *sshJournalReader) GetCursor() string {
	jr.mu.Lock()
	defer jr.mu.Unlock()
	return jr.cursor
}
