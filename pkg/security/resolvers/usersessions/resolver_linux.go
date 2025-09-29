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

func parseSSHLogLines(lines []string, ctx *model.UserSessionContext) {
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
	for i := len(lines) - 1; i >= 0; i-- {
		line := lines[i]
		// separate the line into words
		words := strings.Split(line, " ")
		sshLogLine := SSHLogLine{}
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
			continue
		}
		// if the service is "sshd" and the line starts with "Accepted" it's the beginning of an ssh session
		if strings.HasPrefix(sshLogLine.Service, "sshd") && strings.HasPrefix(sshLogLine.Remaining, "Accepted") {
			// One example of line is: "Accepted publickey for lima from 192.168.5.2 port 38835 ssh2: ED25519 SHA256:J3I5W45pnQtan5u0m27HWzyqAMZfTbG+nRet/pzzylU"
			// Get the infos like that : Accepted <authentification method> for <username> from <ip> port <port> <ssh version> <Remaining (hash)>
			// Here it should be the good line to parse. If we have multiple connexion with same username, we start by the last one so it should be the good one
			// TODO?: Maybe add a check on the date and time ( + eventually correlated to edit time of the file ?)

			sshWords := strings.Split(sshLogLine.Remaining, " ")
			if len(sshWords) < 9 {
				continue
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
			if sshParsedLine.IP == ctx.SSHClientIP && sshParsedLine.Port == fmt.Sprintf("%d", ctx.SSHPort) {
				ctx.SSHUsername = sshParsedLine.User
				switch sshParsedLine.AuthentificationMethod {
				case "publickey":
					ctx.SSHAuthMethod = 1
					// Here Parse the Public Key which can be ED25519 SHA256:J3I5W45pnQtan5u0m27HWzyqAMZfTbG+nRet/pzzylU
					sshParsedLine.Remaining = strings.Split(sshParsedLine.Remaining, ":")[1]
					ctx.SSHPublicKey = sshParsedLine.Remaining
					return
				case "password":
					ctx.SSHAuthMethod = 2
					return
				// Other types not implemented yet
				default:
					ctx.SSHAuthMethod = 0
					return
				}
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

	parseSSHLogLines(lines, ctx)

}

// ResolveSSHUserSession resolves the ssh user session from the auth log
func (r *Resolver) ResolveSSHUserSession(ctx *model.UserSessionContext) *model.UserSessionContext {
	id := ctx.ID

	if id == 0 {
		return nil
	}

	r.Lock()
	defer r.Unlock()

	f, err := os.OpenFile("/var/log/auth.log", os.O_RDONLY, 0644)
	defer f.Close()
	if err == nil {
	} else if err != nil {
		// Fallback for Red Hat / CentOS / Fedora
		f, err = os.OpenFile("/var/log/secure", os.O_RDONLY, 0644)
		if err == nil {
		} else if err != nil {
			// Last Fallback for openSUSE
			f, err = os.OpenFile("/var/log/messages", os.O_RDONLY, 0644)
			if err == nil {
			} else if err != nil {
				resolveFromJournalctl(ctx)

				ctx.Resolved = true
				// cache resolved context
				r.userSessions.Add(id, ctx)
				return ctx
			}
		}
	}

	var lines []string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		lines = append(lines, sc.Text())
	}
	parseSSHLogLines(lines, ctx)
	ctx.Resolved = true

	// cache resolved context
	r.userSessions.Add(id, ctx)
	return ctx

}
