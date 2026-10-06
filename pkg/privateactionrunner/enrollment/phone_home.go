// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package enrollment

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"time"

	"github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/config/setup"
	app "github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/constants"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/opms"
)

func PhoneHomePOC() bool { return os.Getenv(app.PhoneHomePOCEnvVar) == "true" }

// PhoneHomeAttempt is core-owned scheduling metadata, not an enrollment identity.
// Outcomes and exclusive POST claims are scoped to ID, never to a mutable slot.
type PhoneHomeAttempt struct {
	ID                 string    `json:"id"`
	Number             int       `json:"number"`
	StartedAt          time.Time `json:"started_at"`
	NextAttemptAt      time.Time `json:"next_attempt_at,omitempty"`
	RecoveryCount      int       `json:"recovery_count,omitempty"`
	Recovering         bool      `json:"recovering,omitempty"`
	PreviouslyEnrolled bool      `json:"previously_enrolled,omitempty"`
}

type PhoneHomeOutcome struct {
	AttemptID string `json:"attempt_id"`
	opms.EnrollmentFailure
	PersistencePending bool `json:"persistence_pending,omitempty"`
	HelperRetained     bool `json:"helper_retained,omitempty"`
}

var ErrStalePhoneHomeAttempt = errors.New("stale phone-home attempt")

func PhoneHomeAttemptPath(cfg model.Reader) string {
	return cfg.GetString(setup.PARIdentityFilePath) + ".phone-home"
}

func validAttempt(a *PhoneHomeAttempt) bool {
	id, err := hex.DecodeString(a.ID)
	return err == nil && len(id) == 16 && a.Number > 0 && !a.StartedAt.IsZero()
}

func ReadPhoneHomeAttempt(cfg model.Reader) (*PhoneHomeAttempt, error) {
	root := PhoneHomeAttemptPath(cfg)
	if _, err := os.Lstat(root); os.IsNotExist(err) {
		return nil, nil
	}
	var a PhoneHomeAttempt
	if err := readPhoneHomeJSON(filepath.Join(root, "current.json"), &a); err != nil {
		return nil, errors.New("phone-home journal unreadable or old-format; reconciliation required")
	}
	if !validAttempt(&a) {
		return nil, errors.New("invalid phone-home attempt")
	}
	return &a, nil
}

// ReservePhoneHomeAttempt requires one core launch authority and stopped PAR
// processes. It publishes the protected config binding before authorizing launch.
func ReservePhoneHomeAttempt(cfg model.Reader, hostname string, now time.Time, previous *PhoneHomeAttempt) (*PhoneHomeAttempt, error) {
	if !filepath.IsAbs(cfg.GetString(setup.PARIdentityFilePath)) {
		return nil, errors.New("absolute identity_file_path required")
	}
	current, err := ReadPhoneHomeAttempt(cfg)
	if err != nil {
		return nil, err
	}
	if (current == nil) != (previous == nil) || (current != nil && current.ID != previous.ID) {
		return nil, ErrStalePhoneHomeAttempt
	}
	if current != nil {
		outcome, err := ReadPhoneHomeOutcome(cfg, current)
		if err != nil || outcome == nil || !outcome.RetrySafe || current.NextAttemptAt.IsZero() || now.Before(current.NextAttemptAt) {
			return nil, errors.New("previous attempt not authorized for retry")
		}
	}
	id := make([]byte, 16)
	if _, err := rand.Read(id); err != nil {
		return nil, err
	}
	a := &PhoneHomeAttempt{ID: hex.EncodeToString(id), Number: 1, StartedAt: now}
	if previous != nil {
		a.Number = previous.Number + 1
		a.PreviouslyEnrolled = previous.PreviouslyEnrolled
	} else {
		identity, err := getIdentityFromFile(cfg)
		if err != nil {
			return nil, err
		}
		a.PreviouslyEnrolled = identity != nil
	}
	root := PhoneHomeAttemptPath(cfg)
	if err := os.MkdirAll(filepath.Join(root, a.ID), 0700); err != nil {
		return nil, err
	}
	pending := pendingIdentity{AttemptID: a.ID, ConfigHash: PhoneHomeConfigHash(cfg, hostname)}
	if err := writePhoneHomeJSON(pendingIdentityPath(cfg, a.ID), &pending); err != nil {
		return nil, err
	}
	if err := writePhoneHomeJSON(filepath.Join(root, "current.json"), a); err != nil {
		return nil, err
	}
	return a, nil
}

func SavePhoneHomeSchedule(cfg model.Reader, a *PhoneHomeAttempt) error {
	current, err := ReadPhoneHomeAttempt(cfg)
	if err != nil {
		return err
	}
	if current == nil || current.ID != a.ID {
		return ErrStalePhoneHomeAttempt
	}
	return writePhoneHomeJSON(filepath.Join(PhoneHomeAttemptPath(cfg), "current.json"), a)
}

func ReadPhoneHomeOutcome(cfg model.Reader, a *PhoneHomeAttempt) (*PhoneHomeOutcome, error) {
	var outcome PhoneHomeOutcome
	err := readPhoneHomeJSON(filepath.Join(PhoneHomeAttemptPath(cfg), a.ID, "outcome.json"), &outcome)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil || outcome.AttemptID != a.ID || !validPhoneHomeOutcome(outcome) {
		return nil, errors.New("invalid phone-home outcome")
	}
	return &outcome, nil
}

func validPhoneHomeOutcome(o PhoneHomeOutcome) bool {
	switch o.Category {
	case "quota", "missing_scope", "invalid_credentials", "not_submitted":
		return o.RetrySafe && !o.ReconciliationRequired && !o.PersistencePending && !o.HelperRetained
	case "invalid_request", "invalid_config", "config_mismatch", "local_config_or_storage", "identity_persisted":
		return !o.RetrySafe && !o.ReconciliationRequired && !o.PersistencePending && !o.HelperRetained
	case "transport_ambiguous", "response_ambiguous", "forbidden_unknown", "unauthorized_unverified", "invalid_request_unverified", "throttled_unverified", "name_conflict":
		return !o.RetrySafe && o.ReconciliationRequired && !o.PersistencePending && !o.HelperRetained
	case "persistence_pending", "persistence_memory":
		return !o.RetrySafe && !o.ReconciliationRequired && o.PersistencePending && o.HelperRetained == (o.Category == "persistence_memory")
	default:
		return false
	}
}

func RecordPhoneHomeOutcome(cfg model.Reader, outcome PhoneHomeOutcome) error {
	current, err := ReadPhoneHomeAttempt(cfg)
	if err != nil {
		return err
	}
	if current == nil || current.ID != outcome.AttemptID {
		return ErrStalePhoneHomeAttempt
	}
	if !validPhoneHomeOutcome(outcome) {
		return errors.New("invalid phone-home outcome category")
	}
	// Even if core advances after this check, this write can only touch the old
	// attempt's file, not the new outcome/current pointer.
	return writePhoneHomeJSON(filepath.Join(PhoneHomeAttemptPath(cfg), outcome.AttemptID, "outcome.json"), &outcome)
}

func claimPhoneHomePOST(cfg model.Reader, a *PhoneHomeAttempt) error {
	f, err := os.OpenFile(filepath.Join(PhoneHomeAttemptPath(cfg), a.ID, "post"), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return errors.New("phone-home POST already claimed or journal unavailable")
	}
	err = f.Sync()
	err = errors.Join(err, f.Close())
	if err == nil {
		err = syncPhoneHomeDirectory(filepath.Join(PhoneHomeAttemptPath(cfg), a.ID))
	}
	return err
}

// ArchivePhoneHomeAttempt is the deliberate manual recovery operation. The
// caller MUST stop core and both PAR processes and reconcile the backend first.
// It preserves all evidence/credentials; it is never an automatic retry policy.
func ArchivePhoneHomeAttempt(cfg model.Reader, expectedID string) error {
	a, err := ReadPhoneHomeAttempt(cfg)
	if err != nil {
		return err
	}
	if a == nil || a.ID != expectedID {
		return ErrStalePhoneHomeAttempt
	}
	root := PhoneHomeAttemptPath(cfg)
	return os.Rename(root, root+".reconciled-"+a.ID)
}

func readPhoneHomeJSON(path string, value interface{}) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(data, value); err != nil {
		return errors.New("invalid phone-home storage")
	}
	return nil
}

// writePhoneHomeJSON atomically replaces a 0600 file and syncs it and its parent.
// Neither returned errors nor callers' status expose the serialized contents.
func writePhoneHomeJSON(path string, value interface{}) error {
	data, err := json.Marshal(value)
	if err != nil {
		return errors.New("cannot encode phone-home storage")
	}
	return writePhoneHomeFile(path, data)
}

func writePhoneHomeFile(path string, data []byte) error {
	f, err := os.CreateTemp(filepath.Dir(path), ".par-pending-*")
	if err != nil {
		return errors.New("cannot create phone-home storage")
	}
	defer os.Remove(f.Name())
	_, err = f.Write(data)
	if err == nil {
		err = f.Sync()
	}
	err = errors.Join(err, f.Close())
	if err == nil {
		err = os.Rename(f.Name(), path)
	}
	if err == nil {
		err = syncPhoneHomeDirectory(filepath.Dir(path))
	}
	if err != nil {
		return errors.New("cannot publish phone-home storage")
	}
	return nil
}

func syncPhoneHomeDirectory(path string) error {
	dir, err := os.Open(path)
	if err != nil {
		return err
	}
	return errors.Join(dir.Sync(), dir.Close())
}
