// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux
// +build linux

package java

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"io/fs"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/DataDog/datadog-agent/pkg/process/util"
)

// Hotspot java has a specific protocol, described here:
//   o touch .attach_pid<pid-of-java>
//   o kill -SIGQUIT <pid-of-java>
//   o java process check if .attach_pid<his-pid> exit
//   o then create an unix socket .java_pid<his-pid>
//   o we can write command through the unix socket
//
// Public documentation https://openjdk.org/groups/hotspot/docs/Serviceability.html#battach
//
type Hotspot struct {
	pid   int
	nsPid int
	root  string
	cwd   string // viewed by the process
	conn  *net.UnixConn

	socketPath string
}

// NewHotspot create an object to connect to an JVM hotspot
// pid (host pid) and nsPid (within the namespace pid)
func NewHotspot(pid int, nsPid int) (h *Hotspot, err error) {
	h = &Hotspot{
		pid:   pid,
		nsPid: nsPid,
	}
	// workaround Centos 7 (kernel 3.10) NSPid field was introduced in 4.1
	// so we don't support container on Centos 7
	if h.nsPid == 0 {
		h.nsPid = pid
	}

	procPath := fmt.Sprintf("%s/%d", util.HostProc(), pid)
	h.root = procPath + "/root"
	h.cwd, err = os.Readlink(procPath + "/cwd")
	if err != nil {
		return nil, err
	}
	return h, nil
}

func (h *Hotspot) tmpPath() string {
	return fmt.Sprintf("%s/tmp", h.root)
}

func (h *Hotspot) socketExists() bool {
	mode, err := os.Stat(h.socketPath)
	return err == nil && (mode.Mode()&fs.ModeSocket > 0)
}

func getPathOwner(path string) (uint32, uint32, error) {
	mode, err := os.Stat(path)
	if err != nil {
		return 0, 0, err
	}
	stat := mode.Sys().(*syscall.Stat_t)
	return stat.Uid, stat.Gid, nil
}

func (h *Hotspot) copyAgent(agent string, uid int, gid int) (dstPath string, cleanup func(), err error) {
	dstPath = h.cwd + "/" + filepath.Base(agent)
	if dst, err := os.Stat(h.root + dstPath); err == nil {
		// if the destination file already exist
		// check if it's not the source agent file
		if src, err := os.Stat(agent); err == nil {
			s := src.Sys().(*syscall.Stat_t)
			d := dst.Sys().(*syscall.Stat_t)
			if s.Dev == d.Dev && s.Ino == d.Ino {
				return "", func() {}, nil
			}
		}
	}

	fagent, err := os.Open(agent)
	if err != nil {
		return "", nil, err
	}
	defer fagent.Close()

	dst, err := os.OpenFile(h.root+dstPath, os.O_RDWR|os.O_CREATE|os.O_TRUNC, os.FileMode(0444))
	if err != nil {
		return "", nil, err
	}
	_, err = io.Copy(dst, fagent)
	dst.Close()
	if err != nil {
		return "", nil, err
	}
	if err := syscall.Chown(h.root+dstPath, uid, gid); err != nil {
		return "", nil, err
	}

	return dstPath, func() {
		os.Remove(h.root + dstPath)
	}, nil
}

func (h *Hotspot) connect() (cleanup func(), err error) {
	addr, err := net.ResolveUnixAddr("unix", h.socketPath)
	if err != nil {
		return nil, err
	}
	conn, err := net.DialUnix("unix", nil, addr)
	if err != nil {
		return nil, err
	}

	if err := conn.SetDeadline(time.Now().Add(3 * time.Second)); err != nil {
		conn.Close()
		return nil, err
	}
	h.conn = conn
	return func() { h.conn.Close() }, nil
}

func (h *Hotspot) parseResponse(buf []byte) (returnCommand int, returnCode int, response string) {
	line := 0
	scanner := bufio.NewScanner(bytes.NewReader(buf))
	for scanner.Scan() {
		s := string(scanner.Bytes())
		switch line {
		case 0:
			returnCommand, _ = strconv.Atoi(s)
		case 1:
			if strings.HasPrefix(s, "return code: ") {
				returnCode, _ = strconv.Atoi(s[len("return code: "):])
			}
		}
		response += s + "\n"
		line++
	}
	return returnCommand, returnCode, response
}

// command: tailingNull is necessary here to flush command
//	otherwise the JVM is block waiting for more bytes
//	This is apply only for some command like : 'load agent.so true'
func (h *Hotspot) command(cmd string, tailingNull bool) error {
	if _, err := h.conn.Write([]byte{'1', 0}); err != nil { // Protocol version
		return err
	}

	for _, c := range strings.Split(cmd, " ") {
		cmd := append([]byte(c), 0)
		if _, err := h.conn.Write(cmd); err != nil {
			return err
		}
	}
	if tailingNull {
		if _, err := h.conn.Write([]byte{0}); err != nil {
			return err
		}
	}

	buf := make([]byte, 8192)
	if _, err := h.conn.Read(buf); err != nil {
		return err
	}
	returnCommand, returnCode, responseText := h.parseResponse(buf)

	if returnCommand != 0 {
		return fmt.Errorf("command '%s' return command %d code %d\n%s\n", cmd, returnCommand, returnCode, responseText)
	}
	return nil
}

// the (short) protocol is following
//  o create a file .attach_pid%d
//  o send a SIGQUIT signal
//  o wait for socket file created by the java process
func (h *Hotspot) attachJVMProtocol(uid int, gid int) error {
	attachPIDPath := func(root string) string {
		return fmt.Sprintf("%s/.attach_pid%d", root, h.nsPid)
	}

	attachPath := attachPIDPath(h.root + h.cwd)
	hook, err := os.OpenFile(attachPath, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0660)
	hook.Close()
	// we don't check Chown() return error here as it can failed on some filesystem
	_ = syscall.Chown(attachPath, uid, gid)
	hookUID, _, ownerErr := getPathOwner(attachPath)
	if err != nil || ownerErr != nil || hookUID != uint32(uid) {
		// we failed to create the .attach_pid file in the process directory
		// let's try in /tmp

		// Note: some filesystem over write the owner when creating a file
		// JVM doesn't like this
		if ownerErr == nil {
			os.Remove(attachPath)
		}

		attachPath = attachPIDPath(h.tmpPath())
		hook, err = os.OpenFile(attachPath, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0660)
		hook.Close()
	}
	defer os.Remove(attachPath)
	if err != nil {
		return err
	}

	process, _ := os.FindProcess(h.pid)
	if err := process.Signal(syscall.SIGQUIT); err != nil {
		return fmt.Errorf("process %d/%d SIGQUIT failed : %s", h.pid, h.nsPid, err)
	}

	h.socketPath = fmt.Sprintf("%s/.java_pid%d", h.tmpPath(), h.nsPid)
	end := time.Now().Add(6 * time.Second)
	for end.After(time.Now()) {
		time.Sleep(200 * time.Millisecond)
		if h.socketExists() {
			return nil
		}
	}
	return fmt.Errorf("the java process %d/%d didn't create a socket file", h.pid, h.nsPid)
}

// Attach() an agent to the hostspot, uid/gid must be accessible read-only by the targeted hotspot
func (h *Hotspot) Attach(agent string, args string, uid int, gid int) error {

	// ask JVM to create a socket to communicate
	if err := h.attachJVMProtocol(uid, gid); err != nil {
		return err
	}

	// copy the agent in the cwd of the process and change his owner/group
	agentPath, agentCleanup, err := h.copyAgent(agent, uid, gid)
	if err != nil {
		return err
	}
	defer agentCleanup()

	// connect and ask to load the agent .jar or .so
	cleanConn, err := h.connect()
	if err != nil {
		return err
	}
	defer cleanConn()

	var loadCommand string
	isJar := strings.HasSuffix(filepath.Base(agent), ".jar")
	if isJar { // agent is a .jar
		loadCommand = fmt.Sprintf("load instrument false %s", agentPath)
		if args != "" {
			loadCommand += "=" + args
		}
	} else {
		loadCommand = fmt.Sprintf("load %s true", agentPath)
		if args != "" {
			loadCommand += " " + args
		}
	}
	if err := h.command(loadCommand, !isJar); err != nil {
		return err
	}

	return nil
}
