// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build darwin

package notableevents

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"unicode/utf8"

	notableeventtypes "github.com/DataDog/datadog-agent/pkg/notableevents/types"
	"golang.org/x/sys/unix"
)

const (
	darwinBookmarkFilename = "bookmark.json"
	maxDarwinBookmarkSize  = 4 * 1024 * 1024
	maxDarwinCustomDepth   = notableeventtypes.MaxCustomDepth
	maxDarwinCustomNodes   = notableeventtypes.MaxCustomNodes
	maxDarwinCustomItems   = notableeventtypes.MaxCustomItems
)

type secureDarwinBookmarkStore struct {
	baseDirectory         string
	directories           []string
	managedDirectoryStart int
	expectedUID           uint32
	fsync                 func(int) error

	treeMu      sync.Mutex
	treeDurable bool
}

// newProductionDarwinBookmarkStore stores state below the canonical,
// symlink-free spelling of the macOS Agent state tree.
func newProductionDarwinBookmarkStore() darwinBookmarkStore {
	return &secureDarwinBookmarkStore{
		baseDirectory:         "/",
		directories:           []string{"private", "var", "db", "datadog-agent", "notable-events"},
		managedDirectoryStart: 3,
		expectedUID:           0,
		fsync:                 unix.Fsync,
	}
}

// newTestDarwinBookmarkStore creates an otherwise production-equivalent store
// below an injectable trusted base directory.
func newTestDarwinBookmarkStore(baseDirectory string, expectedUID uint32) *secureDarwinBookmarkStore {
	return &secureDarwinBookmarkStore{
		baseDirectory:         baseDirectory,
		directories:           []string{"notable-events"},
		managedDirectoryStart: 0,
		expectedUID:           expectedUID,
		fsync:                 unix.Fsync,
	}
}

// Load securely opens and decodes the private collector state.
func (s *secureDarwinBookmarkStore) Load() (*darwinBookmarkState, error) {
	directory, err := s.openStateDirectory()
	if err != nil {
		return nil, err
	}
	defer directory.Close()

	fd, err := unix.Openat(
		int(directory.Fd()),
		darwinBookmarkFilename,
		unix.O_RDONLY|unix.O_CLOEXEC|unix.O_NOFOLLOW|unix.O_NONBLOCK,
		0,
	)
	if errors.Is(err, unix.ENOENT) {
		return newDarwinBookmarkState(), nil
	}
	if err != nil {
		return nil, fmt.Errorf("open bookmark: %w", err)
	}

	file := os.NewFile(uintptr(fd), darwinBookmarkFilename)
	if file == nil {
		_ = unix.Close(fd)
		return nil, errors.New("create bookmark file handle")
	}
	defer file.Close()

	if err := s.validateStateFile(fd); err != nil {
		return nil, err
	}
	data, err := io.ReadAll(io.LimitReader(file, maxDarwinBookmarkSize+1))
	if err != nil {
		return nil, fmt.Errorf("read bookmark: %w", err)
	}
	if len(data) > maxDarwinBookmarkSize {
		return nil, fmt.Errorf("%w: bookmark exceeds %d bytes", errDarwinBookmarkCorrupt, maxDarwinBookmarkSize)
	}

	var state darwinBookmarkState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, fmt.Errorf("%w: %v", errDarwinBookmarkCorrupt, err)
	}
	if state.Version != darwinBookmarkSchemaVersion {
		return nil, fmt.Errorf(
			"%w: unsupported private bookmark schema version %d",
			errDarwinBookmarkCorrupt,
			state.Version,
		)
	}
	if state.Directories == nil {
		state.Directories = make(map[string]*directoryBookmarkState)
	}
	if state.Acknowledged == nil {
		state.Acknowledged = make(map[string]int64)
	}
	if state.Pending == nil {
		state.Pending = make(map[string]Event)
	}
	if err := validateDarwinBookmarkState(&state); err != nil {
		return nil, err
	}
	return &state, nil
}

// validateDarwinBookmarkState rejects semantically corrupt persisted state before it becomes live.
func validateDarwinBookmarkState(state *darwinBookmarkState) error {
	if state == nil {
		return corruptDarwinBookmark("state is null")
	}
	if state.Version != darwinBookmarkSchemaVersion {
		return corruptDarwinBookmark("unsupported schema version")
	}
	if len(state.Directories) > maxDarwinDirectories {
		return corruptDarwinBookmark("too many directories")
	}
	if len(state.Pending) > maxDarwinPendingEvents {
		return corruptDarwinBookmark("too many pending events")
	}
	if len(state.Acknowledged) > defaultDarwinMaxAcknowledged {
		return corruptDarwinBookmark("too many acknowledged events")
	}

	totalFiles := 0
	for directoryKey, directory := range state.Directories {
		if !isLowerHexString(directoryKey, sha256HexLength) {
			return corruptDarwinBookmark("invalid directory key")
		}
		if directory == nil || directory.Files == nil {
			return corruptDarwinBookmark("invalid directory state")
		}
		if directory.LastSeen <= 0 {
			return corruptDarwinBookmark("invalid directory timestamp")
		}
		if len(directory.Files) > maxDarwinFilesPerDirectory {
			return corruptDarwinBookmark("too many files in directory")
		}
		totalFiles += len(directory.Files)
		if totalFiles > maxDarwinTotalFiles {
			return corruptDarwinBookmark("too many file records")
		}
		for name, fileState := range directory.Files {
			if validateReportBasename(name) != nil {
				return corruptDarwinBookmark("invalid report filename")
			}
			if fileState.Fingerprint == "" || len(fileState.Fingerprint) > maxDarwinFingerprintBytes || !utf8.ValidString(fileState.Fingerprint) {
				return corruptDarwinBookmark("invalid report fingerprint")
			}
			if fileState.EventID != "" && !isDarwinEventID(fileState.EventID) {
				return corruptDarwinBookmark("invalid report event id")
			}
			if fileState.BaselineOnly && fileState.EventID != "" {
				return corruptDarwinBookmark("baseline-only report already has an event id")
			}
		}
	}

	for id, timestamp := range state.Acknowledged {
		if !isDarwinEventID(id) || timestamp <= 0 {
			return corruptDarwinBookmark("invalid acknowledged event")
		}
		if _, pending := state.Pending[id]; pending {
			return corruptDarwinBookmark("event is both pending and acknowledged")
		}
	}
	for id, event := range state.Pending {
		if !isDarwinEventID(id) || event.ID != id {
			return corruptDarwinBookmark("invalid pending event id")
		}
		if err := validateDarwinEvent(event); err != nil {
			return err
		}
	}
	return nil
}

const sha256HexLength = 64

func validateDarwinEvent(event Event) error {
	if err := notableeventtypes.ValidateEvent(event); err != nil {
		return corruptDarwinBookmark(err.Error())
	}
	return nil
}

func isDarwinEventID(id string) bool {
	return notableeventtypes.IsEventID(id)
}

func isLowerHexString(value string, expectedLength int) bool {
	if len(value) != expectedLength || strings.ToLower(value) != value {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

func corruptDarwinBookmark(message string) error {
	return fmt.Errorf("%w: %s", errDarwinBookmarkCorrupt, message)
}

// Save atomically persists collector state and its containing directory.
func (s *secureDarwinBookmarkStore) Save(state *darwinBookmarkState) error {
	if err := validateDarwinBookmarkState(state); err != nil {
		return fmt.Errorf("refuse invalid macOS notable events bookmark: %w", err)
	}
	data, err := json.Marshal(state)
	if err != nil {
		return err
	}
	if len(data) > maxDarwinBookmarkSize {
		return fmt.Errorf("bookmark exceeds %d bytes", maxDarwinBookmarkSize)
	}

	directory, err := s.openStateDirectory()
	if err != nil {
		return err
	}
	defer directory.Close()
	directoryFD := int(directory.Fd())

	if err := s.validateExistingStateFile(directoryFD); err != nil {
		return err
	}

	tempName, tempFD, err := s.createTempFile(directoryFD)
	if err != nil {
		return err
	}
	cleanupTemp := true
	defer func() {
		if cleanupTemp {
			_ = unix.Unlinkat(directoryFD, tempName, 0)
		}
	}()

	tempFile := os.NewFile(uintptr(tempFD), tempName)
	if tempFile == nil {
		_ = unix.Close(tempFD)
		return errors.New("create temporary bookmark file handle")
	}
	written, err := tempFile.Write(data)
	if err != nil {
		_ = tempFile.Close()
		return fmt.Errorf("write temporary bookmark: %w", err)
	}
	if written != len(data) {
		_ = tempFile.Close()
		return fmt.Errorf("write temporary bookmark: %w", io.ErrShortWrite)
	}
	if err := s.fsync(int(tempFile.Fd())); err != nil {
		_ = tempFile.Close()
		return fmt.Errorf("sync temporary bookmark: %w", err)
	}
	if err := tempFile.Close(); err != nil {
		return fmt.Errorf("close temporary bookmark: %w", err)
	}

	if err := unix.Renameat(directoryFD, tempName, directoryFD, darwinBookmarkFilename); err != nil {
		return fmt.Errorf("replace bookmark: %w", err)
	}
	cleanupTemp = false
	if err := s.fsync(directoryFD); err != nil {
		return fmt.Errorf("sync bookmark directory: %w", err)
	}
	return nil
}

// openStateDirectory creates and opens each state-tree component relative to
// the previously verified descriptor.
func (s *secureDarwinBookmarkStore) openStateDirectory() (*os.File, error) {
	s.treeMu.Lock()
	defer s.treeMu.Unlock()

	current, err := openDiagnosticReportDirectory(s.baseDirectory)
	if err != nil {
		return nil, fmt.Errorf("open state base directory: %w", err)
	}

	// A store instance synchronizes every managed parent on its first complete
	// traversal, including when all children already exist. This closes the
	// ambiguous-retry window where mkdirat succeeded but its parent fsync did
	// not. Later traversals only need to sync parents of newly created entries.
	syncAllManagedParents := !s.treeDurable
	for index, component := range s.directories {
		nextFD, openErr := unix.Openat(
			int(current.Fd()),
			component,
			unix.O_RDONLY|unix.O_CLOEXEC|unix.O_DIRECTORY|unix.O_NOFOLLOW,
			0,
		)
		wasMissing := errors.Is(openErr, unix.ENOENT)
		if wasMissing {
			if index < s.managedDirectoryStart {
				_ = current.Close()
				return nil, fmt.Errorf("open trusted state directory %s: %w", component, openErr)
			}
			// From this point onward the entry may have been created even if a
			// later open or validation step fails. Force the next traversal to
			// synchronize every managed parent before trusting the tree again.
			s.treeDurable = false
			if mkdirErr := unix.Mkdirat(int(current.Fd()), component, 0o700); mkdirErr != nil && !errors.Is(mkdirErr, unix.EEXIST) {
				_ = current.Close()
				return nil, fmt.Errorf("create state directory %s: %w", component, mkdirErr)
			}
			nextFD, openErr = unix.Openat(
				int(current.Fd()),
				component,
				unix.O_RDONLY|unix.O_CLOEXEC|unix.O_DIRECTORY|unix.O_NOFOLLOW,
				0,
			)
		}
		if openErr != nil {
			_ = current.Close()
			return nil, fmt.Errorf("open state directory %s: %w", component, openErr)
		}
		if err := s.validateDirectory(nextFD, component); err != nil {
			_ = unix.Close(nextFD)
			_ = current.Close()
			return nil, err
		}
		if index >= s.managedDirectoryStart && (syncAllManagedParents || wasMissing) {
			// The initial ENOENT also covers an EEXIST race from mkdirat: the
			// entry's durability is uncertain regardless of who created it.
			if err := s.fsync(int(current.Fd())); err != nil {
				_ = unix.Close(nextFD)
				_ = current.Close()
				return nil, fmt.Errorf("sync parent of state directory %s: %w", component, err)
			}
		}
		_ = current.Close()
		current = os.NewFile(uintptr(nextFD), component)
		if current == nil {
			_ = unix.Close(nextFD)
			return nil, fmt.Errorf("create state directory handle for %s", component)
		}
	}
	s.treeDurable = true
	return current, nil
}

func (s *secureDarwinBookmarkStore) validateDirectory(fd int, component string) error {
	var stat unix.Stat_t
	if err := unix.Fstat(fd, &stat); err != nil {
		return fmt.Errorf("stat state directory %s: %w", component, err)
	}
	if stat.Mode&unix.S_IFMT != unix.S_IFDIR {
		return fmt.Errorf("state path component %s is not a directory", component)
	}
	if stat.Uid != s.expectedUID {
		return fmt.Errorf("state directory %s is owned by uid %d, expected %d", component, stat.Uid, s.expectedUID)
	}
	if stat.Mode&0o022 != 0 {
		return fmt.Errorf("state directory %s has unsafe mode %#o", component, stat.Mode&0o777)
	}
	return nil
}

func (s *secureDarwinBookmarkStore) validateStateFile(fd int) error {
	var stat unix.Stat_t
	if err := unix.Fstat(fd, &stat); err != nil {
		return fmt.Errorf("stat bookmark: %w", err)
	}
	if stat.Mode&unix.S_IFMT != unix.S_IFREG {
		return errors.New("bookmark is not a regular file")
	}
	if stat.Uid != s.expectedUID {
		return fmt.Errorf("bookmark is owned by uid %d, expected %d", stat.Uid, s.expectedUID)
	}
	if stat.Mode&0o777 != 0o600 {
		return fmt.Errorf("bookmark mode is %#o, expected 0600", stat.Mode&0o777)
	}
	if stat.Size > maxDarwinBookmarkSize {
		return fmt.Errorf("%w: bookmark exceeds %d bytes", errDarwinBookmarkCorrupt, maxDarwinBookmarkSize)
	}
	return nil
}

func (s *secureDarwinBookmarkStore) validateExistingStateFile(directoryFD int) error {
	fd, err := unix.Openat(
		directoryFD,
		darwinBookmarkFilename,
		unix.O_RDONLY|unix.O_CLOEXEC|unix.O_NOFOLLOW|unix.O_NONBLOCK,
		0,
	)
	if errors.Is(err, unix.ENOENT) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("open existing bookmark: %w", err)
	}
	defer unix.Close(fd)
	return s.validateStateFile(fd)
}

func (s *secureDarwinBookmarkStore) createTempFile(directoryFD int) (string, int, error) {
	for attempt := 0; attempt < 10; attempt++ {
		random := make([]byte, 12)
		if _, err := rand.Read(random); err != nil {
			return "", -1, fmt.Errorf("generate temporary bookmark name: %w", err)
		}
		name := ".bookmark-" + hex.EncodeToString(random) + ".tmp"
		fd, err := unix.Openat(
			directoryFD,
			name,
			unix.O_WRONLY|unix.O_CREAT|unix.O_EXCL|unix.O_CLOEXEC|unix.O_NOFOLLOW,
			0o600,
		)
		if errors.Is(err, unix.EEXIST) {
			continue
		}
		if err != nil {
			return "", -1, fmt.Errorf("create temporary bookmark: %w", err)
		}
		if err := s.validateStateFile(fd); err != nil {
			_ = unix.Close(fd)
			_ = unix.Unlinkat(directoryFD, name, 0)
			return "", -1, err
		}
		return name, fd, nil
	}
	return "", -1, errors.New("create unique temporary bookmark")
}
