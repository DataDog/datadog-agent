// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package client is the SMB client of the SMB log source. It is the only
// package the SMB launcher and tailers use to talk to a share, and it keeps the
// vendored SMB library out of their view.
//
// The interface is stateless on purpose: a read opens the file, reads a byte
// range and closes the handle before returning, so the Agent never holds a
// handle that could block or delay the log writer's rotation (rename, delete,
// truncate). File identity across rotations comes from the server's 64-bit
// FileId, reported both by directory listings and at open.
package client

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path"
	"strconv"
	"strings"
	"time"
)

const (
	defaultPort        = 445
	defaultDialTimeout = 10 * time.Second
	defaultOpTimeout   = 30 * time.Second

	// redactedSecret replaces the password wherever it could be printed.
	redactedSecret = "********"
)

// ErrClosed is returned by every call made after Close.
var ErrClosed = errors.New("smb client is closed")

// Config describes one share and the account used to mount it.
//
// The password is never printed: String and GoString redact it, so the usual
// fmt verbs (%v, %+v, %s, %#v) are safe. Do not copy the field into logs,
// status or error messages yourself.
type Config struct {
	Host     string // server name or address, without port
	Share    string // share name, e.g. "logs"
	Username string
	Password string
	Domain   string // NTLM domain; empty lets the server choose
	Port     int    // 0 means 445

	DialTimeout time.Duration // bounds connect, authentication and mount; 0 means 10s
	OpTimeout   time.Duration // bounds each ListDir/ReadAt call; 0 means 30s
}

// Entry is one directory entry returned by ListDir.
type Entry struct {
	Name         string    // base name
	Size         int64     // EndOfFile from the listing (a hint; may be stale)
	FileID       uint64    // server FileId; 0 when unknown
	ModTime      time.Time // LastWriteTime
	CreationTime time.Time
	IsDir        bool
}

// ReadResult is the outcome of one ReadAt call.
type ReadResult struct {
	FileID uint64 // identity observed at open (QFid DiskFileId); 0 if unavailable
	Size   int64  // EndOfFile observed at open (authoritative)
	Data   []byte // bytes read starting at the requested offset (len <= max)
}

// Client is a connection to one share. Implementations are safe for concurrent
// use.
type Client interface {
	// ListDir lists dir, relative to the share root ("" is the root), sorted by
	// name and without "." and "..".
	ListDir(ctx context.Context, dir string) ([]Entry, error)
	// ReadAt opens path (read-only, share READ|WRITE|DELETE, no lease), reads
	// up to max bytes from off, and closes the handle before returning. It
	// never keeps a handle open between calls.
	//
	// Data holds min(max, Size-off) bytes unless the file changes during the
	// call. An offset at or past the end of the file is not an error: Data is
	// empty and FileID and Size still describe the file. max == 0 only reports
	// FileID and Size.
	ReadAt(ctx context.Context, path string, off int64, max int) (ReadResult, error)
	// Close releases the connection. Calls made after Close return ErrClosed.
	Close() error
}

// DialFunc opens a Client. Dial is the production implementation.
type DialFunc func(ctx context.Context, cfg Config) (Client, error)

// Abort closes c like Close, but without logging off: it drops the connection
// at once, so it never waits for the server, and it cuts short a Close of c
// that is waiting for its LOGOFF. The server closes the session's tree
// connects and opens when the connection drops ([MS-SMB2] 3.3.7.1), and no
// handle outlives a call of this package, so nothing is lost. A Client that
// cannot abort is closed with Close.
func Abort(c Client) error {
	if a, ok := c.(interface{ Abort() error }); ok {
		return a.Abort()
	}
	return c.Close()
}

// CleanPath returns p in the form the Client uses for paths relative to the
// share root: '/' separators (a '\' is treated as one), no leading, trailing
// or repeated separator, no "." element, and "" for the root. It rejects ".."
// elements and NUL bytes, so a path can never leave the share.
func CleanPath(p string) (string, error) {
	if strings.ContainsRune(p, 0) {
		return "", fmt.Errorf("invalid SMB path %q: %w", p, os.ErrInvalid)
	}
	p = strings.ReplaceAll(p, `\`, "/")
	for _, elem := range strings.Split(p, "/") {
		if elem == ".." {
			return "", fmt.Errorf("invalid SMB path %q: %w", p, os.ErrInvalid)
		}
	}
	p = path.Clean("/" + p)
	return strings.TrimPrefix(p, "/"), nil
}

// String implements fmt.Stringer without the password.
func (c Config) String() string {
	return fmt.Sprintf("{Host:%s Share:%s Username:%s Password:%s Domain:%s Port:%d DialTimeout:%s OpTimeout:%s}",
		c.Host, c.Share, c.Username, redact(c.Password), c.Domain, c.Port, c.DialTimeout, c.OpTimeout)
}

// GoString implements fmt.GoStringer without the password.
func (c Config) GoString() string {
	return fmt.Sprintf("client.Config{Host:%q, Share:%q, Username:%q, Password:%q, Domain:%q, Port:%d, DialTimeout:%d, OpTimeout:%d}",
		c.Host, c.Share, c.Username, redact(c.Password), c.Domain, c.Port, c.DialTimeout, c.OpTimeout)
}

// target names the share in logs and errors, without credentials.
func (c Config) target() string {
	host := c.Host
	if c.Port != 0 && c.Port != defaultPort {
		host = host + ":" + strconv.Itoa(c.Port)
	}
	return "smb://" + host + "/" + c.Share
}

func (c Config) withDefaults() Config {
	if c.Port == 0 {
		c.Port = defaultPort
	}
	if c.DialTimeout <= 0 {
		c.DialTimeout = defaultDialTimeout
	}
	if c.OpTimeout <= 0 {
		c.OpTimeout = defaultOpTimeout
	}
	return c
}

func (c Config) validate() error {
	switch {
	case c.Host == "":
		return errors.New("smb: host is required")
	case c.Share == "":
		return errors.New("smb: share is required")
	case strings.ContainsAny(c.Share, `/\`):
		return fmt.Errorf("smb: share %q must be a single share name", c.Share)
	case c.Port < 0 || c.Port > 65535:
		return fmt.Errorf("smb: port %d is out of range", c.Port)
	}
	return nil
}

func redact(secret string) string {
	if secret == "" {
		return ""
	}
	return redactedSecret
}

// normalizeFileID maps the all-ones "no identifier" value to 0, so callers
// only need to check for 0.
func normalizeFileID(id uint64) uint64 {
	if id == ^uint64(0) {
		return 0
	}
	return id
}
