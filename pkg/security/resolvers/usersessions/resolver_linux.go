// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package usersessions holds model related to the user sessions resolver
package usersessions

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/cilium/ebpf"
	"github.com/hashicorp/golang-lru/v2/simplelru"

	manager "github.com/DataDog/ebpf-manager"

	"github.com/DataDog/datadog-agent/pkg/security/probe/managerhelper"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model/usersession"
	"github.com/DataDog/datadog-agent/pkg/security/seclog"
)

// UserSessionKey describes the key to a user session
type UserSessionKey struct {
	ID      uint64
	Cursor  byte
	Padding [7]byte
}

// UserSessionData stores user session context data retrieved from the kernel
type UserSessionData struct {
	SessionType usersession.Type
	RawData     string
}

// UnmarshalBinary unmarshalls a binary representation of itself
func (e *UserSessionData) UnmarshalBinary(data []byte) error {
	if len(data) < 256 {
		return model.ErrNotEnoughSpace
	}

	e.SessionType = usersession.Type(data[0])
	e.RawData += model.NullTerminatedString(data[1:240])
	return nil
}

// incrementalFileReader is used to read a file incrementally
type incrementalFileReader struct {
	path               string
	f                  *os.File
	offset             int64
	mu                 sync.Mutex
	ino                uint64
	lastRead           time.Time
	readFromJournalctl bool
}

type sshSessionKey struct {
	IP   string // net.IP.String()
	Port string
}

type sshSessionValue struct {
	AuthenticationMethod int
	PublicKey            string
}

type sshSessionParsed struct {
	mu  sync.Mutex
	lru *simplelru.LRU[sshSessionKey, sshSessionValue]
}

// Resolver is used to resolve the user sessions context
type Resolver struct {
	sync.RWMutex
	userSessions *simplelru.LRU[uint64, *model.UserSessionContext]

	userSessionsMap *ebpf.Map

	sshLogReader     *incrementalFileReader
	sshSessionParsed sshSessionParsed
}

// NewResolver returns a new instance of Resolver
func NewResolver(cacheSize int) (*Resolver, error) {
	lru, err := simplelru.NewLRU[uint64, *model.UserSessionContext](cacheSize, nil)
	if err != nil {
		return nil, fmt.Errorf("couldn't create User Session resolver cache: %v", err)
	}

	return &Resolver{
		userSessions: lru,
	}, nil
}

// Start initializes the eBPF map of the resolver
func (r *Resolver) Start(manager *manager.Manager) error {
	r.Lock()
	defer r.Unlock()

	m, err := managerhelper.Map(manager, "user_sessions")
	if err != nil {
		return fmt.Errorf("couldn't start user session resolver: %v", err)
	}
	r.userSessionsMap = m

	// start the resolver for ssh sessions
	r.StartSSHUserSessionResolver()
	return nil
}

// ResolveUserSession returns the user session associated to the provided ID
func (r *Resolver) ResolveUserSession(id uint64) *model.UserSessionContext {
	if id == 0 {
		return nil
	}

	r.Lock()
	defer r.Unlock()

	// is this session already in cache ?
	if session, ok := r.userSessions.Get(id); ok {
		return session
	}

	// lookup the session in kernel space
	key := UserSessionKey{
		ID:     id,
		Cursor: 1,
	}

	value := UserSessionData{}
	err := r.userSessionsMap.Lookup(&key, &value)
	for err == nil {
		key.Cursor++
		err = r.userSessionsMap.Lookup(&key, &value)
	}
	if key.Cursor == 1 && err != nil {
		// the session doesn't exist, leave now
		return nil
	}

	ctx := &model.UserSessionContext{
		ID:          id,
		SessionType: int(value.SessionType),
	}
	// parse the content of the user session context
	err = json.Unmarshal([]byte(value.RawData), ctx)
	if err != nil {
		seclog.Debugf("failed to parse user session data: %v", err)
		return nil
	}

	ctx.Resolved = true

	// cache resolved context
	r.userSessions.Add(id, ctx)
	return ctx
}

func newIncrementalFileReader(path string) *incrementalFileReader {
	return &incrementalFileReader{
		path: path,
	}
}

// Init opens the file and sets the initial offset
func (ifr *incrementalFileReader) Init(f *os.File) error {
	ifr.mu.Lock()
	defer ifr.mu.Unlock()

	if ifr.f != nil {
		return nil
	}

	st, err := f.Stat()
	if err != nil {
		_ = f.Close()
		seclog.Warnf("Fail to stat log file: %v", err)
		return err
	}

	ifr.offset = st.Size()

	ifr.f = f
	ifr.ino = inodeOf(st)
	_, err = ifr.f.Seek(ifr.offset, io.SeekStart)
	if err != nil {
		ifr.close()
		ifr.f = nil
	}
	return err
}

// startReading start to parse the potential ssh session and store them in the LRU
func (r *Resolver) startReading() {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	switch r.sshLogReader.readFromJournalctl {
	case true:
		for {
			<-ticker.C
			err := r.sshLogReader.resolveFromJournalctl(&r.sshSessionParsed)
			if err != nil {
				seclog.Errorf("failed to read journalctl: %v", err)
			}
		}
	case false:
		for {
			<-ticker.C
			err := r.sshLogReader.resolveFromLogFile(&r.sshSessionParsed)
			if err != nil {
				seclog.Errorf("failed to read ssh log lines: %v", err)
			}
		}
	}
}

func parseSSHLogLine(line string, sshSessionParsed *sshSessionParsed) (bool, string) {
	type SSHLogLine struct {
		Date      string
		Hostname  string
		Service   string
		Remaining string
	}
	type SSHParsedLine struct {
		AuthentificationMethod string
		User                   string
		IP                     string
		Port                   string
		SSHVersion             string
		Remaining              string
	}
	// separate the line into words
	words := strings.Split(line, " ")
	sshLogLine := SSHLogLine{}
	if len(words) < 5 {
		return false, ""
	}
	switch {
	// We saw two different types of logs, so we try to parse both
	case strings.HasPrefix(words[2], "sshd"):
		sshLogLine = SSHLogLine{
			Date:      words[0],
			Hostname:  words[1],
			Service:   words[2],
			Remaining: strings.Join(words[3:], " "),
		}
	case strings.HasPrefix(words[4], "sshd"):
		sshLogLine = SSHLogLine{
			Date:      words[2],
			Hostname:  words[3],
			Service:   words[4],
			Remaining: strings.Join(words[5:], " "),
		}
	default:
		return false, ""
	}
	// if the service is "sshd" and the line starts with "Accepted" it's the beginning of an ssh session
	if strings.HasPrefix(sshLogLine.Service, "sshd") && strings.HasPrefix(sshLogLine.Remaining, "Accepted") {
		// One example of line is: "Accepted publickey for lima from 192.168.5.2 port 38835 ssh2: ED25519 SHA256:J3I5W45pnQtan5u0m27HWzyqAMZfTbG+nRet/pzzylU"
		// Get the infos like that : Accepted <authentification method> for <username> from <ip> port <port> <ssh version> <Remaining (hash)>

		sshWords := strings.Split(sshLogLine.Remaining, " ")
		if len(sshWords) < 9 {
			return false, ""
		}
		sshParsedLine := SSHParsedLine{
			AuthentificationMethod: sshWords[1],
			User:                   sshWords[3],
			IP:                     sshWords[5],
			Port:                   sshWords[7],
			SSHVersion:             sshWords[8],
			Remaining:              strings.Join(sshWords[9:], " "),
		}
		// We compare port and IP to ensure that the line is the one we want
		// Convert string IP to net.IP and compare normalized values
		parsedIP := net.ParseIP(sshParsedLine.IP)

		// We store every session in the LRU cache
		var authType int
		var publicKey string
		switch sshParsedLine.AuthentificationMethod {
		case "publickey":
			authType = usersession.SSHAuthMethodPublicKey
			// Here Parse the Public Key which can be ED25519 SHA256:J3I5W45pnQtan5u0m27HWzyqAMZfTbG+nRet/pzzylU
			sshParsedLine.Remaining = strings.Split(sshParsedLine.Remaining, ":")[1]
			publicKey = sshParsedLine.Remaining
		case "password":
			authType = usersession.SSHAuthMethodPassword
		// Other types not implemented yet
		default:
			authType = usersession.SSHAuthMethodUnknown
		}
		key := sshSessionKey{
			IP:   parsedIP.String(),
			Port: sshParsedLine.Port,
		}
		value := sshSessionValue{
			AuthenticationMethod: authType,
			PublicKey:            publicKey,
		}
		sshSessionParsed.mu.Lock()
		sshSessionParsed.lru.Add(key, value)
		sshSessionParsed.mu.Unlock()
		return true, sshLogLine.Date
	}
	return false, ""
}

// resolveFromLogFile read all the lines that have been added since the last call without reopening the file.
// Return new lines, the byte offsets at the end of each line, and an error.
func (ifr *incrementalFileReader) resolveFromLogFile(sshSessionParsed *sshSessionParsed) error {
	ifr.mu.Lock()
	defer ifr.mu.Unlock()

	if err := ifr.reloadIfRotated(); err != nil {
		return err
	}

	st, err := ifr.f.Stat()
	if err != nil {
		return err
	}

	if st.Size() == ifr.offset {
		return nil
	}
	// If the file is truncated, we restart from the beginning
	if st.Size() < ifr.offset {
		ifr.offset = 0
		if _, err := ifr.f.Seek(0, io.SeekStart); err != nil {
			return err
		}
	} else {
		// If the file is not truncated, we seek to the offset
		if _, err := ifr.f.Seek(ifr.offset, io.SeekStart); err != nil {
			return err
		}
	}

	sc := bufio.NewScanner(ifr.f)
	for sc.Scan() {
		line := sc.Text()
		parseSSHLogLine(line, sshSessionParsed)
	}
	newOffset, err := ifr.f.Seek(0, io.SeekCurrent)
	if err != nil {
		return err
	}

	ifr.offset = newOffset
	return err
}

func (ifr *incrementalFileReader) resolveFromJournalctl(sshSessionParsed *sshSessionParsed) error {
	// format for journalctl
	sinceStr := ifr.lastRead.Format("2006-01-02 15:04:05")

	cmd := exec.Command("journalctl", "--no-pager", "--since", sinceStr, "--output=short-iso")

	var out bytes.Buffer
	cmd.Stdout = &out

	if err := cmd.Run(); err != nil {
		seclog.Errorf("failed to read journalctl: %v", err)
		return err
	}

	lines := strings.Split(strings.TrimSpace(out.String()), "\n")

	var lastDate string
	for i := 0; i < len(lines); i++ {
		_, lastDate = parseSSHLogLine(lines[i], sshSessionParsed)
	}
	// We update the lastRead like this to avoid skipping another line that could be another ssh session
	parsedSince, err := time.Parse("2006-01-02T15:04:05-0700", lastDate)
	if err != nil {
		seclog.Errorf("failed to parse date from journalctl: %v", err)
	}
	ifr.lastRead = parsedSince
	return nil

}

// close closes the file.
// The lock of IncrementalFileReader must be held
func (ifr *incrementalFileReader) close() error {
	if ifr.f != nil {
		err := ifr.f.Close()
		ifr.f = nil
		return err
	}
	return nil
}

// inodeOf get the inode of the file.
func inodeOf(fi os.FileInfo) uint64 {
	if st, ok := fi.Sys().(*syscall.Stat_t); ok {
		return st.Ino
	}
	return 0
}

// reloadIfRotated reopens the file if the inode has changed.
func (ifr *incrementalFileReader) reloadIfRotated() error {
	curSt, err := os.Stat(ifr.path)
	if err != nil {
		return err
	}
	curIno := inodeOf(curSt)
	if curIno != 0 && ifr.ino != 0 && curIno != ifr.ino {
		// The file has been rotated
		if ifr.f != nil {
			_ = ifr.close()
			ifr.f = nil
		}
		f, err := os.Open(ifr.path)
		if err != nil {
			ifr.close()
			ifr.f = nil
			return err
		}
		ifr.f = f
		ifr.ino = curIno

		// We restart from the beginning because it's a new file
		ifr.offset = 0
	}
	return nil
}

// StartSSHUserSessionResolver initializes the ssh log reader by looking for the available file, opening it and setting up the initial offset
func (r *Resolver) StartSSHUserSessionResolver() {
	possibleLogPaths := []string{
		"/var/log/auth.log", // Debian/Ubuntu
		"/var/log/secure",   // RHEL/CentOS/Fedora
		"/var/log/messages", // openSUSE/autres
	}
	path := ""
	var err error
	for _, possiblePath := range possibleLogPaths {
		_, err = os.Stat(possiblePath)
		if err == nil {
			path = possiblePath
			break
		}
	}

	r.sshLogReader = newIncrementalFileReader(path)

	// Initialize the SSH session LRU cache (needed in all cases)
	r.sshSessionParsed.lru, err = simplelru.NewLRU[sshSessionKey, sshSessionValue](100, nil)
	if err != nil {
		seclog.Errorf("couldn't create SSH Session LRU cache: %v", err)
		return
	}

	if path == "" {
		// Don't want to continue in case there is no log file, use journalctl instead
		r.sshLogReader.lastRead = time.Now()
		r.sshLogReader.readFromJournalctl = true
		go r.startReading()
		return
	}

	r.sshLogReader.readFromJournalctl = false
	// Now we can open the file if there is one
	f, err := os.OpenFile(path, os.O_RDONLY, 0644)
	if err != nil {
		seclog.Errorf("failed to open ssh log file: %v", err)
		return
	}
	if err := r.sshLogReader.Init(f); err != nil {
		seclog.Errorf("failed to init ssh log reader: %v", err)
		// If Init fails, we close the file
		if f != nil {
			f.Close()
		}
		return
	}
	go r.startReading()
	// Note: the file is not closed here, as the sshLogReader manage it

}

// ResolveSSHUserSession resolves the ssh user session from the auth log
func (r *Resolver) ResolveSSHUserSession(ctx *model.UserSessionContext) *model.UserSessionContext {
	id := ctx.ID

	if id == 0 {
		return nil
	}

	r.Lock()
	defer r.Unlock()

	key := sshSessionKey{
		IP:   ctx.SSHClientIP.IP.String(),
		Port: fmt.Sprintf("%d", ctx.SSHPort),
	}
	r.sshSessionParsed.mu.Lock()
	value, ok := r.sshSessionParsed.lru.Get(key)
	r.sshSessionParsed.mu.Unlock()

	if ok {
		ctx.SSHAuthMethod = value.AuthenticationMethod
		ctx.SSHPublicKey = value.PublicKey
		ctx.Resolved = true
		// cache resolved context
		r.userSessions.Add(id, ctx)

		return ctx
	}

	return ctx
}
