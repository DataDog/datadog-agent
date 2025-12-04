// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package usersessions

import "time"

// Journal représente une interface pour interagir avec systemd journal
// Cette interface permet de faciliter les tests en mockant le journal
type Journal interface {
	// AddMatch ajoute un filtre pour les entrées du journal
	AddMatch(match string) error
	// Close ferme le journal
	Close() error
	// GetCursor retourne le cursor actuel
	GetCursor() (string, error)
	// GetEntry retourne l'entrée courante du journal
	GetEntry() (*JournalEntry, error)
	// Next avance au prochain événement dans le journal
	Next() (uint64, error)
	// NextSkip saute les n prochaines entrées
	NextSkip(skip uint64) (uint64, error)
	// SeekCursor positionne le journal à un cursor spécifique
	SeekCursor(cursor string) error
	// SeekHead positionne le journal au début
	SeekHead() error
	// SeekTail positionne le journal à la fin
	SeekTail() error
	// Wait attend qu'il y ait de nouvelles entrées dans le journal
	Wait(timeout time.Duration) int
}

// JournalEntry représente une entrée du journal systemd
type JournalEntry struct {
	Fields             map[string]string
	RealtimeTimestamp  uint64
	MonotonicTimestamp uint64
}
