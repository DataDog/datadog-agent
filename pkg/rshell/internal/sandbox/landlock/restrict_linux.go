// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux

package landlock

import (
	"errors"
	"fmt"
	"path/filepath"
	"runtime"
	"syscall"

	ll "github.com/landlock-lsm/go-landlock/landlock/syscall"
	"golang.org/x/sys/unix"
)

// handledAccess contains every filesystem right through Landlock ABI 3. The
// one-shot worker runs registered Go builtins in the already-loaded helper
// binary, so execute can be denied globally. Directory creation/removal,
// regular-file removal, special files, symlinks, and cross-directory moves are
// also denied globally. :rw grants only the regular-file writes, creation, and
// truncation that rshell currently supports. Device ioctl and UNIX-socket
// resolution were introduced after ABI 3 and are covered by seccomp.
const handledAccess = uint64(
	ll.AccessFSExecute |
		ll.AccessFSWriteFile |
		ll.AccessFSReadFile |
		ll.AccessFSReadDir |
		ll.AccessFSRemoveDir |
		ll.AccessFSRemoveFile |
		ll.AccessFSMakeChar |
		ll.AccessFSMakeDir |
		ll.AccessFSMakeReg |
		ll.AccessFSMakeSock |
		ll.AccessFSMakeFifo |
		ll.AccessFSMakeBlock |
		ll.AccessFSMakeSym |
		ll.AccessFSRefer |
		ll.AccessFSTruncate,
)

const readAccess = uint64(ll.AccessFSReadFile | ll.AccessFSReadDir)

const readWriteAccess = uint64(
	ll.AccessFSReadFile |
		ll.AccessFSReadDir |
		ll.AccessFSWriteFile |
		ll.AccessFSTruncate |
		ll.AccessFSMakeReg,
)

const devNullAccess = uint64(ll.AccessFSReadFile | ll.AccessFSWriteFile)

type ruleObjectKind uint8

const (
	ruleDirectory ruleObjectKind = iota
	ruleRegularFile
	ruleCharacterDevice
)

type openedRule struct {
	path          string
	fd            int
	allowedAccess uint64
	allowedPath   bool
	mode          accessMode
}

// Restrict applies an exact, process-wide Landlock policy derived from
// allowedPaths, without command-dependent trusted path exceptions.
func Restrict(allowedPaths []string) error {
	return RestrictWithTrustedPaths(allowedPaths, nil)
}

// RestrictReadOnlyWithTrustedPaths applies a process-wide policy that treats
// every backend AllowedPaths entry as read-only, even when it carries a :rw
// suffix. This is used for selectively elevated read-only actions so the
// kernel policy cannot expose write rights that the interpreter mode rejects.
func RestrictReadOnlyWithTrustedPaths(allowedPaths []string, trustedPaths []TrustedPath) error {
	rules, err := openRulesReadOnly(allowedPaths, trustedPaths)
	if err != nil {
		return fmt.Errorf("build Landlock policy: %w", err)
	}
	defer closeOpenedRules(rules)
	return restrictOpenedRules(rules)
}

// RestrictWithTrustedPaths applies an exact, process-wide Landlock policy.
// Backend AllowedPaths entries without a suffix and entries ending in :ro
// grant file reads and directory listing. :rw additionally grants regular-file
// creation, writes, and truncation. It does not grant deletion. Linking,
// renaming, execution, directory mutation, file removal, and special-file
// creation are handled but never granted by backend AllowedPaths.
//
// trustedPaths are narrow exceptions derived locally from the actual builtin
// command. They are not part of the backend AllowedPaths contract and must not
// be populated from unsigned request data.
//
// Every configured target is opened exactly once with O_PATH. Backend
// AllowedPaths use openat2 with RESOLVE_NO_SYMLINKS and
// RESOLVE_NO_MAGICLINKS, while fixed trusted kernel paths may follow the
// symlink and magic-link components they require. The same descriptor is used
// both to validate its object type and to add the Landlock rule, eliminating a
// validate-close-reopen race. An exact /dev/null read-write rule preserves
// rshell's unconditional null-redirection contract. Missing and inaccessible
// required paths fail closed, and the policy never uses best-effort
// enforcement.
func RestrictWithTrustedPaths(allowedPaths []string, trustedPaths []TrustedPath) error {
	rules, err := openRules(allowedPaths, trustedPaths)
	if err != nil {
		return fmt.Errorf("build Landlock policy: %w", err)
	}
	defer closeOpenedRules(rules)
	return restrictOpenedRules(rules)
}

func restrictOpenedRules(rules []openedRule) error {
	abi, err := requireKernelSupport()
	if err != nil {
		return err
	}

	rulesetAttr := ll.RulesetAttr{HandledAccessFS: handledAccess}
	rulesetFD, err := ll.LandlockCreateRuleset(&rulesetAttr, 0)
	if err != nil {
		if errors.Is(err, syscall.ENOSYS) || errors.Is(err, syscall.EOPNOTSUPP) {
			return fmt.Errorf("%w: create ruleset: %v", ErrUnsupported, err)
		}
		return fmt.Errorf("create Landlock ruleset: %w", err)
	}
	defer unix.Close(rulesetFD)

	for _, rule := range rules {
		attr := ll.PathBeneathAttr{ParentFd: rule.fd, AllowedAccess: rule.allowedAccess}
		if err := ll.LandlockAddPathBeneathRule(rulesetFD, &attr, 0); err != nil {
			return fmt.Errorf("add Landlock rule for %q: %w", rule.path, err)
		}
	}

	if err := enforceRuleset(rulesetFD, abi); err != nil {
		return fmt.Errorf("enforce Landlock policy: %w", err)
	}
	return nil
}

func openRules(allowedPaths []string, trustedPaths []TrustedPath) ([]openedRule, error) {
	return openRulesWithMode(allowedPaths, trustedPaths, false)
}

func openRulesReadOnly(allowedPaths []string, trustedPaths []TrustedPath) ([]openedRule, error) {
	return openRulesWithMode(allowedPaths, trustedPaths, true)
}

func openRulesWithMode(allowedPaths []string, trustedPaths []TrustedPath, forceReadOnly bool) ([]openedRule, error) {
	allowedRules, err := parseAllowedPaths(allowedPaths)
	if err != nil {
		return nil, err
	}

	rules := make([]openedRule, 0, len(allowedRules)+len(trustedPaths))
	closeOnError := func(err error) ([]openedRule, error) {
		closeOpenedRules(rules)
		return nil, err
	}

	for _, rule := range allowedRules {
		mode := rule.mode
		if forceReadOnly {
			mode = accessReadOnly
		}
		access := readAccess
		if mode == accessReadWrite {
			access = readWriteAccess
		}
		opened, skipped, err := openRule(rule.path, ruleDirectory, access, false, true)
		if err != nil {
			return closeOnError(err)
		}
		if skipped {
			return closeOnError(fmt.Errorf("required AllowedPaths entry %q was unexpectedly skipped", rule.path))
		}
		opened.allowedPath = true
		opened.mode = mode
		rules = append(rules, opened)
	}
	if err := rejectResolvedWideningOverlaps(rules); err != nil {
		return closeOnError(err)
	}

	for _, trusted := range trustedPaths {
		access, err := trustedAccess(trusted)
		if err != nil {
			return closeOnError(err)
		}
		kind := ruleDirectory
		if trusted.Kind == TrustedPathFile {
			kind = ruleRegularFile
		}
		opened, skipped, err := openRule(trusted.Path, kind, access, trusted.Optional, false)
		if err != nil {
			return closeOnError(err)
		}
		if !skipped {
			rules = append(rules, opened)
		}
	}

	devNull, _, err := openRule("/dev/null", ruleCharacterDevice, devNullAccess, false, false)
	if err != nil {
		return closeOnError(err)
	}
	rules = append(rules, devNull)
	return rules, nil
}

func trustedAccess(trusted TrustedPath) (uint64, error) {
	if trusted.Path == "" {
		return 0, errors.New("trusted Landlock path must not be empty")
	}
	if !filepath.IsAbs(trusted.Path) {
		return 0, fmt.Errorf("trusted Landlock path %q must be absolute", trusted.Path)
	}
	if trusted.Kind != TrustedPathDirectory && trusted.Kind != TrustedPathFile {
		return 0, fmt.Errorf("trusted Landlock path %q has invalid kind %d", trusted.Path, trusted.Kind)
	}
	switch trusted.Access {
	case TrustedPathReadOnly:
		if trusted.Kind == TrustedPathFile {
			return uint64(ll.AccessFSReadFile), nil
		}
		return readAccess, nil
	case TrustedPathReadRemoveFiles:
		if trusted.Kind != TrustedPathDirectory {
			return 0, fmt.Errorf("trusted Landlock path %q cannot remove files from an exact-file rule", trusted.Path)
		}
		return readAccess | uint64(ll.AccessFSRemoveFile), nil
	default:
		return 0, fmt.Errorf("trusted Landlock path %q has invalid access %d", trusted.Path, trusted.Access)
	}
}

func openRule(path string, kind ruleObjectKind, access uint64, optional, rejectSymlinks bool) (openedRule, bool, error) {
	clean := filepath.Clean(path)
	var fd int
	var err error
	if rejectSymlinks {
		fd, err = unix.Openat2(unix.AT_FDCWD, clean, &unix.OpenHow{
			Flags:   uint64(unix.O_PATH | unix.O_CLOEXEC | unix.O_DIRECTORY),
			Resolve: unix.RESOLVE_NO_SYMLINKS | unix.RESOLVE_NO_MAGICLINKS,
		})
	} else {
		fd, err = unix.Open(clean, unix.O_PATH|unix.O_CLOEXEC, 0)
	}
	if err != nil {
		if optional && errors.Is(err, syscall.ENOENT) {
			return openedRule{}, true, nil
		}
		return openedRule{}, false, fmt.Errorf("open Landlock path %q: %w", clean, err)
	}

	var stat unix.Stat_t
	if err := unix.Fstat(fd, &stat); err != nil {
		_ = unix.Close(fd)
		return openedRule{}, false, fmt.Errorf("validate Landlock path %q: %w", clean, err)
	}
	objectType := stat.Mode & unix.S_IFMT
	switch kind {
	case ruleDirectory:
		if objectType != unix.S_IFDIR {
			_ = unix.Close(fd)
			return openedRule{}, false, fmt.Errorf("Landlock path %q must be a directory", clean)
		}
	case ruleRegularFile:
		if objectType != unix.S_IFREG {
			_ = unix.Close(fd)
			return openedRule{}, false, fmt.Errorf("Landlock path %q must be a regular or pseudo file", clean)
		}
	case ruleCharacterDevice:
		if objectType != unix.S_IFCHR {
			_ = unix.Close(fd)
			return openedRule{}, false, fmt.Errorf("Landlock path %q must be a character device", clean)
		}
	default:
		_ = unix.Close(fd)
		return openedRule{}, false, fmt.Errorf("Landlock path %q has invalid kind %d", clean, kind)
	}
	return openedRule{path: clean, fd: fd, allowedAccess: access}, false, nil
}

// rejectResolvedWideningOverlaps rejects a read-only AllowedPaths directory
// whose already-opened object is at or below an already-opened read-write
// AllowedPaths directory. Landlock grants are additive, so the parent write
// rule would otherwise override the interpreter's most-specific read-only
// child. Walking ".." relative to the pinned O_PATH descriptors compares the
// actual directory objects without resolving or reopening either configured
// path; backend symlink and magic-link components are rejected while opening.
func rejectResolvedWideningOverlaps(rules []openedRule) error {
	for _, parent := range rules {
		if !parent.allowedPath || parent.mode != accessReadWrite {
			continue
		}
		for _, child := range rules {
			if !child.allowedPath || child.mode != accessReadOnly {
				continue
			}
			inside, err := fdIsAncestor(parent.fd, child.fd)
			if err != nil {
				return fmt.Errorf("compare resolved Landlock paths %q and %q: %w", parent.path, child.path, err)
			}
			if inside {
				return fmt.Errorf("Landlock cannot represent read-only path %q beneath read-write path %q", child.path, parent.path)
			}
		}
	}
	return nil
}

func fdIsAncestor(parentFD, childFD int) (bool, error) {
	parentIdentity, err := fstatIdentity(parentFD)
	if err != nil {
		return false, fmt.Errorf("inspect parent descriptor: %w", err)
	}

	currentFD := childFD
	currentOwned := false
	defer func() {
		if currentOwned {
			_ = unix.Close(currentFD)
		}
	}()
	for {
		currentIdentity, err := fstatIdentity(currentFD)
		if err != nil {
			return false, fmt.Errorf("inspect child ancestry descriptor: %w", err)
		}
		if currentIdentity == parentIdentity {
			return true, nil
		}

		nextFD, err := unix.Openat(currentFD, "..", unix.O_PATH|unix.O_CLOEXEC|unix.O_DIRECTORY, 0)
		if err != nil {
			return false, fmt.Errorf("open parent directory: %w", err)
		}
		nextIdentity, err := fstatIdentity(nextFD)
		if err != nil {
			_ = unix.Close(nextFD)
			return false, fmt.Errorf("inspect parent directory: %w", err)
		}
		if currentIdentity == nextIdentity {
			_ = unix.Close(nextFD)
			return false, nil
		}
		if currentOwned {
			_ = unix.Close(currentFD)
		}
		currentFD = nextFD
		currentOwned = true
	}
}

type fileIdentity struct {
	device uint64
	inode  uint64
}

func fstatIdentity(fd int) (fileIdentity, error) {
	var stat unix.Stat_t
	if err := unix.Fstat(fd, &stat); err != nil {
		return fileIdentity{}, err
	}
	return fileIdentity{device: uint64(stat.Dev), inode: stat.Ino}, nil
}

func closeOpenedRules(rules []openedRule) {
	for _, rule := range rules {
		_ = unix.Close(rule.fd)
	}
}

func requireKernelSupport() (int, error) {
	version, err := ll.LandlockGetABIVersion()
	if err != nil {
		return 0, fmt.Errorf("%w: query kernel ABI: %v", ErrUnsupported, err)
	}
	if version < 3 {
		return 0, fmt.Errorf("%w: kernel provides ABI %d, need ABI 3", ErrUnsupported, version)
	}
	return version, nil
}

func enforceRuleset(rulesetFD, abi int) error {
	if abi < 8 {
		return enforceRulesetBeforeTSync(rulesetFD)
	}

	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if err := unix.Prctl(unix.PR_SET_NO_NEW_PRIVS, 1, 0, 0, 0); err != nil {
		return fmt.Errorf("prctl(PR_SET_NO_NEW_PRIVS): %w", err)
	}
	if err := ll.LandlockRestrictSelf(rulesetFD, ll.FlagRestrictSelfTSync); err != nil {
		return fmt.Errorf("landlock_restrict_self(TSYNC): %w", err)
	}
	return nil
}
