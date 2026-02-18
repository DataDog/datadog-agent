// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package usersessions holds model related to the user sessions resolver
package usersessions

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"math/rand/v2"
	"net"
	"os"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/cilium/ebpf"
	lru "github.com/hashicorp/golang-lru/v2"
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
	if e.SessionType != usersession.UserSessionTypeK8S {
		seclog.Debugf("not k8s session: %v", e.SessionType)
	}
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
	readFromJournalctl bool
	// chan to stop journalctl
	stopReading chan struct{} // make(chan struct{}, 1)
}

// SSHSessionKey describes the key to a ssh session in the LRU
type SSHSessionKey struct {
	SSHDPid string
	IP      string // net.IP.String()
	Port    string
}

// SSHSessionValue describes the value to a ssh session in the LRU
type SSHSessionValue struct {
	AuthenticationMethod int
	PublicKey            string
}

// Resolver is used to resolve the user sessions context
type Resolver struct {
	sync.RWMutex
	k8suserSessions *simplelru.LRU[uint64, *model.K8SSessionContext]

	userSessionsMap *ebpf.Map

	sshEnabled       bool
	sshLogReader     *incrementalFileReader
	sshSessionParsed *lru.Cache[SSHSessionKey, SSHSessionValue]
}

// NewResolver returns a new instance of Resolver
func NewResolver(cacheSize int, sshEnabled bool) (*Resolver, error) {
	lru, err := simplelru.NewLRU[uint64, *model.K8SSessionContext](cacheSize, nil)
	if err != nil {
		return nil, fmt.Errorf("couldn't create User Session resolver cache: %v", err)
	}

	return &Resolver{
		k8suserSessions: lru,
		sshEnabled:      sshEnabled,
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

	// start the resolver for ssh sessions only if enabled
	if r.sshEnabled {
		err = r.StartSSHUserSessionResolver()
		if err != nil {
			return err
		}
	}
	return nil
}

// ResolveK8SUserSession returns the user session associated to the provided ID
func (r *Resolver) ResolveK8SUserSession(id uint64) *model.K8SSessionContext {
	if id == 0 {
		return nil
	}

	r.Lock()

	defer r.Unlock()

	// is this session already in cache ?
	if session, ok := r.k8suserSessions.Get(id); ok {
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
	sessionType := int(value.SessionType)
	if sessionType != int(usersession.UserSessionTypeK8S) {
		seclog.Debugf("session %d is not a k8s session: %v", id, sessionType)
	}
	ctx := &model.K8SSessionContext{
		K8SSessionID: id,
	}
	// parse the content of the user session context
	err = json.Unmarshal([]byte(value.RawData), ctx)
	if err != nil {
		seclog.Debugf("failed to parse user session data: %v", err)
		return nil
	}

	ctx.K8SResolved = true

	// cache resolved context
	r.k8suserSessions.Add(id, ctx)
	return ctx
}

// GetSSHSession retrieves SSH session information from the cache
func (r *Resolver) GetSSHSession(key SSHSessionKey) (SSHSessionValue, bool) {
	if r.sshSessionParsed == nil {
		return SSHSessionValue{}, false
	}
	return r.sshSessionParsed.Get(key)
}

// Init opens the file and sets the initial offset
func (ifr *incrementalFileReader) Init(f *os.File) error {
	if ifr.f != nil {
		return nil
	}

	st, err := f.Stat()
	if err != nil {
		_ = f.Close()
		seclog.Warnf("Fail to stat log file: %v", err)
		return err
	}
	// Start from the beginning
	ifr.offset = 0

	ifr.f = f
	ifr.ino = inodeOf(st)
	_, err = ifr.f.Seek(ifr.offset, io.SeekStart)
	if err != nil {
		// Comment: already in an error path
		_ = ifr.close(false)
		ifr.f = nil
	}
	return err
}

// startReading start to parse the potential ssh session and store them in the LRU
func (r *Resolver) startReading() {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	if !r.sshLogReader.readFromJournalctl {
		// For now we only read from the ssh log file
		for {
			select {
			case <-r.sshLogReader.stopReading:
				return
			case <-ticker.C:
				r.sshLogReader.mu.Lock()
				err := r.sshLogReader.resolveFromLogFile(r.sshSessionParsed)
				if err != nil {
					seclog.Errorf("failed to read ssh log lines: %v", err)
				}
				r.sshLogReader.mu.Unlock()

			}
		}
	}
}

// parseSSHLogLine parse the ssh log line
// Does not return any error and just automatically updates the LRU when a new session is found
func parseSSHLogLine(line string, sshSessionParsed *lru.Cache[SSHSessionKey, SSHSessionValue]) {
	type SSHLogLine struct {
		Date      string
		Hostname  string
		Service   string
		SSHDPid   string
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
	words := strings.Fields(line)
	sshLogLine := SSHLogLine{}
	if len(words) < 5 {
		return
	}
	switch {
	// We saw two different types of logs, so we try to parse both
	// We use HasPrefix because sshd is followed by its PID : sshd[pid]
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
		return
	}
	startPid := strings.Index(sshLogLine.Service, "[")
	endPid := strings.Index(sshLogLine.Service, "]")
	if startPid == -1 || endPid == -1 {
		seclog.Debugf("Pid not found for sshd line %q", line)
		return
	}
	sshLogLine.SSHDPid = sshLogLine.Service[startPid+1 : endPid]

	// if the service is "sshd" and the line starts with "Accepted" it's the beginning of an ssh session
	if strings.HasPrefix(sshLogLine.Service, "sshd") && strings.HasPrefix(sshLogLine.Remaining, "Accepted") {
		// One example of line is: "Accepted publickey for lima from 192.168.5.2 port 38835 ssh2: ED25519 SHA256:J3I5W45pnQtan5u0m27HWzyqAMZfTbG+nRet/pzzylU"
		// Get the infos like that : Accepted <authentification method> for <username> from <ip> port <port> <ssh version> <Remaining (hash)>

		sshWords := strings.Split(sshLogLine.Remaining, " ")
		if len(sshWords) < 9 {
			seclog.Debugf("fail to parse ssh log line: %s", line)
			return
		}
		sshParsedLine := SSHParsedLine{
			AuthentificationMethod: sshWords[1],
			User:                   sshWords[3],
			IP:                     sshWords[5],
			Port:                   sshWords[7],
			SSHVersion:             sshWords[8],
			Remaining:              strings.Join(sshWords[9:], " "),
		}
		// Convert string IP to net.IP and compare normalized values
		parsedIP := net.ParseIP(sshParsedLine.IP)

		// We store every session in the LRU cache
		var authType usersession.AuthType
		var publicKey string
		switch sshParsedLine.AuthentificationMethod {
		case "publickey":
			authType = usersession.SSHAuthMethodPublicKey
			// Here Parse the Public Key which can be ED25519 SHA256:J3I5W45pnQtan5u0m27HWzyqAMZfTbG+nRet/pzzylU
			parts := strings.SplitN(sshParsedLine.Remaining, ":", 2)
			if len(parts) == 2 {
				publicKey = parts[1]
			}
		case "password":
			authType = usersession.SSHAuthMethodPassword
		// Other types not implemented yet
		default:
			seclog.Debugf("fail to parse ssh auth type in log line: %s", line)
			authType = usersession.SSHAuthMethodUnknown
		}
		key := SSHSessionKey{
			SSHDPid: sshLogLine.SSHDPid,
			IP:      parsedIP.String(),
			Port:    sshParsedLine.Port,
		}
		value := SSHSessionValue{
			AuthenticationMethod: int(authType),
			PublicKey:            publicKey,
		}
		sshSessionParsed.Add(key, value)
	}
}

// resolveFromLogFile read all the lines that have been added since the last call without reopening the file.
// Return new lines, the byte offsets at the end of each line, and an error.
func (ifr *incrementalFileReader) resolveFromLogFile(sshSessionParsed *lru.Cache[SSHSessionKey, SSHSessionValue]) error {
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

// close closes the file.
// The lock of IncrementalFileReader must be held
func (ifr *incrementalFileReader) close(closeReader bool) error {
	var err error
	if ifr.f != nil {
		err = ifr.f.Close()
		ifr.f = nil
	}
	if closeReader {
		// Stop the reader
		ifr.stopReading <- struct{}{}
	}

	return err
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
			_ = ifr.close(false)
			ifr.f = nil
		}
		f, err := os.Open(ifr.path)
		if err != nil {
			_ = ifr.close(false)
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
// Lock must be held
func (r *Resolver) StartSSHUserSessionResolver() error {
	var err error

	// Initialize the SSH session LRU cache (needed in all cases)
	r.sshSessionParsed, err = lru.New[SSHSessionKey, SSHSessionValue](100)
	if err != nil {
		seclog.Errorf("couldn't create SSH Session LRU cache: %v", err)
		return err
	}

	// Try to find the ssh log file
	possibleLogPaths := []string{
		"/var/log/auth.log", // Debian/Ubuntu
		"/var/log/secure",   // RHEL/CentOS/Fedora
		"/var/log/messages", // openSUSE/autres
	}
	path := ""
	for _, possiblePath := range possibleLogPaths {
		_, err = os.Stat(possiblePath)
		if err == nil {
			path = possiblePath
			break
		}
	}
	// If there is no log file, we use journalctl (atm we do nothing)
	if path == "" {
		// // Don't want to continue in case there is no log file
		// TODO : use journalctl instead
		seclog.Trace("no ssh log file found")
		return nil
	}
	// Initialize the SSH log reader
	r.sshLogReader = &incrementalFileReader{
		path:        path,
		stopReading: make(chan struct{}, 1),
	}

	r.sshLogReader.readFromJournalctl = false
	// Now we can open the file if there is one
	f, err := os.OpenFile(path, os.O_RDONLY, 0644)
	if err != nil {
		seclog.Errorf("failed to open ssh log file: %v", err)
		return err
	}
	if err := r.sshLogReader.Init(f); err != nil {
		seclog.Errorf("failed to init ssh log reader: %v", err)
		if f != nil {
			f.Close()
		}
		return err
	}
	go r.startReading()
	// Note: the file is not closed here, as the sshLogReader manage it
	return nil
}

// Close closes the resolver
func (r *Resolver) Close() {
	r.Lock()

	defer r.Unlock()

	if r.sshLogReader != nil {
		r.sshLogReader.mu.Lock()
		defer r.sshLogReader.mu.Unlock()
		_ = r.sshLogReader.close(true)
	}
}

// getEnvVar extracts a specific environment variable from a list of environment variables.
// Each environment variable is in the format "KEY=VALUE".
func getEnvVar(envp []string, key string) string {
	prefix := key + "="
	for _, env := range envp {
		if after, ok := strings.CutPrefix(env, prefix); ok {
			return after
		}
	}
	return ""
}
func getIPfromEnv(ipStr string) net.IPNet {
	ip := net.ParseIP(ipStr)
	if ip != nil {
		if ip.To4() != nil {
			return net.IPNet{
				IP:   ip,
				Mask: net.CIDRMask(32, 32),
			}
		} else if ip.To16() != nil {
			return net.IPNet{
				IP:   ip,
				Mask: net.CIDRMask(128, 128),
			}
		}
	}
	return net.IPNet{}
}

// HandleSSHUserSessionFromProcFS handles the ssh user session from snapshot using resolvers
func (r *Resolver) HandleSSHUserSessionFromPCE(pce *model.ProcessCacheEntry) {
	if r.sshEnabled {
		HandleSSHUserSession(&pce.ProcessContext, pce.EnvsEntry.Values)
	}
}

// HandleSSHUserSession handles the ssh user session
// This function is triggered only if ssh is enabled
func HandleSSHUserSession(pc *model.ProcessContext, envp []string) {
	// First, we check if this event is link to an existing ssh session from his parent
	parent := pc.Parent

	// If the parent is a sshd process, we consider it's a new ssh session
	// A sshd process will always be sshd, except on Ubuntu 25 where they introduced sshd-session
	if parent != nil && strings.HasPrefix(parent.Comm, "sshd") && !strings.HasPrefix(pc.Comm, "sshd") {
		sshSessionID := rand.Uint64()
		pc.UserSession.SSHSessionID = sshSessionID
		// Try to extract the SSH client IP and port
		sshClientVar := getEnvVar(envp, "SSH_CLIENT")
		parts := strings.Fields(sshClientVar)
		if len(parts) >= 2 {
			pc.UserSession.SSHClientIP = getIPfromEnv(parts[0])
			if port, err := strconv.Atoi(parts[1]); err != nil {
				seclog.Warnf("failed to parse SSH_CLIENT port from %q: %v", sshClientVar, err)
			} else {
				pc.UserSession.SSHClientPort = port
				pc.UserSession.SSHDPid = getSSHDPid(pc)
			}
		} else {
			seclog.Tracef("SSH_CLIENT is not in the expected format: %q", sshClientVar)
		}
	}
}

func getSSHDPid(pc *model.ProcessContext) uint32 {
	numberOfSSHD := 0
	currentPid := pc.Pid
	// Get first parent to handle only pce after
	pce := pc.Ancestor
	if pce != nil && strings.HasPrefix(pce.Comm, "sshd") && pce.Pid != currentPid {
		numberOfSSHD++
	}
	for pce.Ancestor != nil && numberOfSSHD < 2 {
		parent := pce.Ancestor
		if strings.HasPrefix(parent.Comm, "sshd") && parent.ProcessContext.Pid != currentPid {
			numberOfSSHD++
		}
		pce = pce.Ancestor
	}
	return pce.Pid
}
