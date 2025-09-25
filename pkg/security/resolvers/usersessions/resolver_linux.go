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
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"

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

// Resolver is used to resolve the user sessions context
type Resolver struct {
	sync.RWMutex
	userSessions *simplelru.LRU[uint64, *model.UserSessionContext]

	userSessionsMap *ebpf.Map
}

// NewResolver returns a new instance of Resolver
func NewResolver(cacheSize int) (*Resolver, error) {
	lru, err := simplelru.NewLRU[uint64, *model.UserSessionContext](cacheSize, nil)
	if err != nil {
		return nil, fmt.Errorf("couldn't create User Session resolver cache: %w", err)
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
		return fmt.Errorf("couldn't start user session resolver: %w", err)
	}
	r.userSessionsMap = m
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
		SessionType: value.SessionType,
	}
	if value.SessionType == 1 {
		// parse the content of the user session context
		err = json.Unmarshal([]byte(value.RawData), ctx)
		if err != nil {
			seclog.Debugf("failed to parse user session data: %v", err)
			return nil
		}
	}
	if value.SessionType == 2 {
		lenData := len(value.RawData)
		if lenData < 0 {
			fmt.Print("empty ssh session data\n")
			return nil
		}
		fmt.Printf("SSH session: %s\n", value.RawData)
		ctx.SSHUsername = string(value.RawData)
		ResolveSSHUserSession(ctx)
	}
	ctx.Resolved = true

	// cache resolved context
	r.userSessions.Add(id, ctx)
	return ctx
}

func parseSSHLogLine(line string, ctx *model.UserSessionContext) {
	fmt.Printf("SSH log line: %s\n", line)
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
	switch {
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
	// if the first word starts with "sshd" and the second word is the username, then print the line
	if strings.HasPrefix(sshLogLine.Service, "sshd") && strings.HasPrefix(sshLogLine.Remaining, "Accepted") {
		// One example of line is Accepted publickey for lima from 192.168.5.2 port 38835 ssh2: ED25519 SHA256:J3I5W45pnQtan5u0m27HWzyqAMZfTbG+nRet/pzzylU
		// Get the infos like that : Accepted *** for lima from *** port *** ssh2: ED25519 SHA256:***
		sshWords := strings.Split(sshLogLine.Remaining, " ")
		if len(sshWords) < 9 {
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
		if sshParsedLine.User == ctx.SSHUsername {
			// Here it should be the good line to parse. If we have multiple connexion with same username, attributes will be overwritten until last line (last one)
			// TODO: Maybe add a check on the date and time ( + eventually correlated to edit time of the file ?)
			ctx.SSHClientIP = sshParsedLine.IP
			fmt.Printf("SSH Client IP: %s\n", sshParsedLine.IP)
			if port, err := strconv.Atoi(sshParsedLine.Port); err == nil {
				ctx.SSHPort = port
				fmt.Printf("SSH Port: %d\n", port)
			}
			switch sshParsedLine.AuthentificationMethod {
			case "publickey":
				ctx.SSHAuthMethod = 1
				// Here Parse the Public Key which can be ED25519 SHA256:J3I5W45pnQtan5u0m27HWzyqAMZfTbG+nRet/pzzylU
				sshParsedLine.Remaining = strings.Split(sshParsedLine.Remaining, ":")[1]
				ctx.SSHPublicKey = sshParsedLine.Remaining
				fmt.Printf("SSH Public Key: %s\n", sshParsedLine.Remaining)
			case "password":
				ctx.SSHAuthMethod = 2
				fmt.Printf("SSH Auth Method: %d\n", ctx.SSHAuthMethod)
			case "keyboard-interactive":
				ctx.SSHAuthMethod = 3
			default:
				ctx.SSHAuthMethod = 0
			}
		}
	}

}

func resolveFromJournalctl(ctx *model.UserSessionContext) {
	cmd := exec.Command("sh", "-c", "journalctl | grep Accepted")

	var out bytes.Buffer
	cmd.Stdout = &out

	if err := cmd.Run(); err != nil {
		return
	}

	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	for _, line := range lines {
		parseSSHLogLine(line, ctx)
	}
	return
}

// ResolveSSHUserSession resolves the ssh user session from the auth log
func ResolveSSHUserSession(ctx *model.UserSessionContext) {
	f, err := os.OpenFile("/var/log/auth.log", os.O_RDONLY, 0644)
	if err == nil {
		ctx.WhereIsLog = 1
		fmt.Print("Found /var/log/auth.log\n")
	}
	if err != nil {
		// Fallback for Red Hat / CentOS / Fedora
		f, err = os.OpenFile("/var/log/secure", os.O_RDONLY, 0644)
		if err == nil {
			ctx.WhereIsLog = 2
			fmt.Print("Found /var/log/secure\n")
		}
		if err != nil {
			// Last Fallback for openSUSE
			f, err = os.OpenFile("/var/log/messages", os.O_RDONLY, 0644)
			if err == nil {
				ctx.WhereIsLog = 3
				fmt.Print("Found /var/log/messages\n")
			}
			if err != nil {
				fmt.Printf("Can't find any log file")
				ctx.WhereIsLog = 4
				resolveFromJournalctl(ctx)
				return
			}
		}
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()
		parseSSHLogLine(line, ctx)
	}
}
