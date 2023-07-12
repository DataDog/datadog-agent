// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package java

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/DataDog/datadog-agent/pkg/process/util"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// Hotspot java has a specific protocol, described here:
//
//	o touch .attach_pid<pid-of-java>
//	o kill -SIGQUIT <pid-of-java>
//	o java process check if .attach_pid<his-pid> exit
//	o then create an unix socket .java_pid<his-pid>
//	o we can write command through the unix socket
//	<pid-of-java> refers to the namespaced pid of the process.
//
// Public documentation https://openjdk.org/groups/hotspot/docs/Serviceability.html#battach
type Hotspot struct {
	pid   int
	nsPid int
	root  string
	cwd   string // viewed by the process
	conn  *net.UnixConn

	socketPath string
	uid        int
	gid        int
}

// NewHotspot create an object to connect to a JVM hotspot
// pid (host pid) and nsPid (within the namespace pid)
//
// NSPid field was introduced in kernel >= 4.1
// So we can't support container on Centos 7 (kernel 3.10)
func NewHotspot(pid int, nsPid int) (*Hotspot, error) {
	h := &Hotspot{
		pid:   pid,
		nsPid: nsPid,
	}
	// Centos 7 workaround to support host environment
	if h.nsPid == 0 {
		h.nsPid = pid
	}

	var err error
	procPath := fmt.Sprintf("%s/%d", util.HostProc(), pid)
	h.root = procPath + "/root"
	h.cwd, err = os.Readlink(procPath + "/cwd")
	if err != nil {
		return nil, err
	}
	h.socketPath = fmt.Sprintf("%s/.java_pid%d", h.tmpPath(), h.nsPid)
	return h, nil
}

func (h *Hotspot) tmpPath() string {
	return fmt.Sprintf("%s/tmp", h.root)
}

func (h *Hotspot) isSocketExists() bool {
	mode, err := os.Stat(h.socketPath)
	return err == nil && (mode.Mode()&fs.ModeSocket > 0)
}

// getPathOwner return the uid/gid pointed by the path
func getPathOwner(path string) (uint32, uint32, error) {
	mode, err := os.Stat(path)
	if err != nil {
		return 0, 0, err
	}
	stat, ok := mode.Sys().(*syscall.Stat_t)
	if stat == nil || !ok {
		return 0, 0, fmt.Errorf("stat cast issue on path %s %T", path, mode.Sys())
	}
	return stat.Uid, stat.Gid, nil
}

// copyAgent copy the agent-usm.jar to a directory where the running java process can load it.
// the agent-usm.jar file must be readable from java process point of view
// copyAgent return :
//
//	o dstPath is path to the copy of agent-usm.jar (from container perspective), this would be pass to the hotspot command
//	o cleanup must be called to remove the created file
func (h *Hotspot) copyAgent(agent string, uid int, gid int) (dstPath string, cleanup func(), err error) {
	dstPath = h.cwd + "/" + filepath.Base(agent)
	// path from the host point of view pointing to the process root namespace (/proc/pid/root/usr/...)
	nsDstPath := h.root + dstPath
	if dst, err := os.Stat(nsDstPath); err == nil {
		// if the destination file already exist
		// check if it's not the source agent file
		if src, err := os.Stat(agent); err == nil {
			s, oks := src.Sys().(*syscall.Stat_t)
			d, okd := dst.Sys().(*syscall.Stat_t)
			if s == nil || d == nil || !oks || !okd {
				return "", nil, fmt.Errorf("stat cast issue on path %s %T %s %T", agent, src.Sys(), nsDstPath, dst.Sys())
			}
			if s.Dev == d.Dev && s.Ino == d.Ino {
				return "", func() {}, nil
			}
		}
	}

	srcAgent, err := os.Open(agent)
	if err != nil {
		return "", nil, err
	}
	defer srcAgent.Close()

	dst, err := os.OpenFile(nsDstPath, os.O_RDWR|os.O_CREATE|os.O_TRUNC, os.FileMode(0444))
	if err != nil {
		return "", nil, err
	}
	_, err = io.Copy(dst, srcAgent)
	dst.Close() // we are closing the file here as Chown will be call just after on the same path
	if err != nil {
		return "", nil, err
	}
	if err := syscall.Chown(nsDstPath, uid, gid); err != nil {
		os.Remove(nsDstPath)
		return "", nil, err
	}

	return dstPath, func() {
		os.Remove(nsDstPath)
	}, nil
}

func (h *Hotspot) dialunix(raddr *net.UnixAddr, withCredential bool) (*net.UnixConn, error) {
	// Hotspot reject connection credentials by checking uid/gid of the client calling connect()
	// via getsockopt(SOL_SOCKET/SO_PEERCRED).
	// but older hotspot JRE (1.8.0) accept only the same uid/gid and reject root
	//
	// For go, during the connect() syscall we don't want to fork() and stay on the same pthread
	// to avoid side effect (pollution) of set effective uid/gid.
	if withCredential {
		runtime.LockOSThread()
		syscall.ForkLock.Lock()
		origeuid := syscall.Geteuid()
		origegid := syscall.Getegid()
		defer func() {
			_ = syscall.Seteuid(origeuid)
			_ = syscall.Setegid(origegid)
			syscall.ForkLock.Unlock()
			runtime.UnlockOSThread()
		}()

		if err := syscall.Setegid(h.gid); err != nil {
			return nil, err
		}
		if err := syscall.Seteuid(h.uid); err != nil {
			return nil, err
		}
	}
	return net.DialUnix("unix", nil, raddr)
}

// connect to the previously created hotspot unix socket
// return close function must be call when finished
func (h *Hotspot) connect(withCredential bool) (close func(), err error) {
	h.conn = nil
	addr, err := net.ResolveUnixAddr("unix", h.socketPath)
	if err != nil {
		return nil, err
	}
	conn, err := h.dialunix(addr, withCredential)
	if err != nil {
		return nil, err
	}

	if err := conn.SetDeadline(time.Now().Add(3 * time.Second)); err != nil {
		conn.Close()
		return nil, err
	}
	h.conn = conn
	return func() {
		if h.conn != nil {
			h.conn.Close()
		}
	}, nil
}

// parseResponse parse the response from the hotspot command
// JVM will return a command error code and some command have a specific return code
// the response will contain the full message
func (h *Hotspot) parseResponse(buf []byte) (returnCommand int, returnCode int, response string, err error) {
	line := 0
	scanner := bufio.NewScanner(bytes.NewReader(buf))
	for scanner.Scan() {
		s := string(scanner.Bytes())
		switch line {
		case 0:
			returnCommand, err = strconv.Atoi(s)
			if err != nil {
				return 0, 0, "", fmt.Errorf("parsing hotspot response failed %d %s", line, s)
			}
		case 1:
			if strings.HasPrefix(s, "return code: ") {
				returnCode, err = strconv.Atoi(s[len("return code: "):])
				if err != nil {
					return 0, 0, "", fmt.Errorf("parsing hotspot response failed %d %s", line, s)
				}
			}
		default:
			break
		}
		line++
	}
	return returnCommand, returnCode, string(buf), nil
}

// command: tailingNull is necessary here to flush command
//
//	otherwise the JVM is blocked and waiting for more bytes
//	This applies only for some command like : 'load agent.so true'
func (h *Hotspot) command(cmd string, tailingNull bool) error {
	if err := h.commandWriteRead(cmd, tailingNull); err != nil {
		// if we receive EPIPE (write) or ECONNRESET (read) from the kernel may from old hotspot JVM
		// let's retry with credentials, see dialunix() for more info
		if !errors.Is(err, syscall.EPIPE) && !errors.Is(err, syscall.ECONNRESET) {
			return err
		}
		log.Debugf("java attach hotspot pid %d/%d old hotspot JVM detected, requires credentials", h.pid, h.nsPid)
		_ = h.conn.Close()
		// we don't need to propagate the cleanConn() callback as it's doing the same thing.
		if _, err := h.connect(true); err != nil {
			return err
		}

		if err := h.commandWriteRead(cmd, tailingNull); err != nil {
			return err
		}
	}
	return nil
}

// commandWriteRead: tailingNull is necessary here to flush command
//
//	otherwise the JVM is blocked and waiting for more bytes
//	This applies only for some command like : 'load agent.so true'
func (h *Hotspot) commandWriteRead(cmd string, tailingNull bool) error {
	if _, err := h.conn.Write([]byte{'1', 0}); err != nil { // Protocol version
		return err
	}

	// We split by space for at most 4 words, since our longest command is "load instrument false <javaagent=args>" which is 4 words
	for _, c := range strings.SplitN(cmd, " ", 4) {
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
	returnCommand, returnCode, responseText, err := h.parseResponse(buf)
	if err != nil {
		return err
	}

	if returnCommand != 0 {
		return fmt.Errorf("command sent to hotspot JVM '%s' return %d and return code %d, response text:\n%s\n", cmd, returnCommand, returnCode, responseText)
	}
	return nil
}

// attachJVMProtocol use this (short) protocol :
//
//	o create a file .attach_pid%d
//	o send a SIGQUIT signal
//	o wait for socket file to be created by the java process
func (h *Hotspot) attachJVMProtocol(uid int, gid int) error {
	attachPIDPath := func(root string) string {
		return fmt.Sprintf("%s/.attach_pid%d", root, h.nsPid)
	}

	attachPath := attachPIDPath(h.root + h.cwd)
	hook, err := os.OpenFile(attachPath, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0660)
	hookUID := uint32(0)
	var ownerErr error
	if err == nil {
		hook.Close()
		// we don't check Chown() return error here as it can fail on some filesystems (read below)
		_ = syscall.Chown(attachPath, uid, gid)
		hookUID, _, ownerErr = getPathOwner(attachPath)
	}
	if err != nil || ownerErr != nil || hookUID != uint32(uid) {
		// We are trying an alternative attach path (in /tmp)
		//  o if we can't create one in the cwd of the java process (probably read only filesystem)
		//  o the filesystem changed the owner (id mapped mounts, like nfs force_uid, ...)

		if ownerErr == nil {
			os.Remove(attachPath)
		}

		attachPath = attachPIDPath(h.tmpPath())
		hook, err = os.OpenFile(attachPath, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0660)
		if err != nil {
			return err
		}
		hook.Close()
	}
	defer os.Remove(attachPath)

	process, _ := os.FindProcess(h.pid) // os.FindProcess() will never fail on linux
	if err := process.Signal(syscall.SIGQUIT); err != nil {
		return fmt.Errorf("process %d/%d SIGQUIT failed : %s", h.pid, h.nsPid, err)
	}

	end := time.Now().Add(6 * time.Second)
	for end.After(time.Now()) {
		time.Sleep(200 * time.Millisecond)
		if h.isSocketExists() {
			return nil
		}
	}
	return fmt.Errorf("the java process %d/%d didn't create a socket file", h.pid, h.nsPid)
}

// Attach an agent to the hotspot, uid/gid must be accessible read-only by the targeted hotspot
func (h *Hotspot) Attach(agentPath string, args string, uid int, gid int) error {
	if !h.isSocketExists() {
		// ask JVM to create a socket to communicate
		if err := h.attachJVMProtocol(uid, gid); err != nil {
			return err
		}
	}

	// copy the agent in the cwd of the process and change his owner/group
	dstAgentPath, agentCleanup, err := h.copyAgent(agentPath, uid, gid)
	if err != nil {
		return err
	}
	defer agentCleanup()

	h.uid = uid
	h.gid = gid
	// connect and ask to load the agent .jar or .so
	cleanConn, err := h.connect(false)
	if err != nil {
		return err
	}
	defer cleanConn()

	var loadCommand string
	isJar := strings.HasSuffix(agentPath, ".jar")
	if isJar { // agent is a .jar
		loadCommand = fmt.Sprintf("load instrument false %s", dstAgentPath)
		if args != "" {
			loadCommand += "=" + args
		}
	} else {
		loadCommand = fmt.Sprintf("load %s true", dstAgentPath)
		if args != "" {
			loadCommand += " " + args
		}
	}

	if err := h.command(loadCommand, !isJar); err != nil {
		log.Debugf("java attach hotspot pid %d/%d command '%s' failed isJar=%v : %s", h.pid, h.nsPid, loadCommand, isJar, err)
		return err
	}
	return nil
}
