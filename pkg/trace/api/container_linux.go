// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016 Datadog, Inc.

package api

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"regexp"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
)

// cgroupPath is the path to the cgroup file where we can find the container id if one exists.
var cgroupPath = "/proc/%d/cgroup"

const (
	uuidSource      = "[0-9a-f]{8}[-_][0-9a-f]{4}[-_][0-9a-f]{4}[-_][0-9a-f]{4}[-_][0-9a-f]{12}"
	containerSource = "[0-9a-f]{64}"
	taskSource      = "[0-9a-f]{32}-\\d+"
)

type ucredKey struct{}

var (
	// expLine matches a line in the /proc/self/cgroup file. It has a submatch for the last element (path), which contains the container ID.
	expLine = regexp.MustCompile(`^\d+:[^:]*:(.+)$`)

	// expContainerID matches contained IDs and sources. Source: https://github.com/Qard/container-info/blob/master/index.js
	expContainerID = regexp.MustCompile(fmt.Sprintf(`(%s|%s|%s)(?:.scope)?$`, uuidSource, containerSource, taskSource))
)

// parseContainerID finds the first container ID reading from r and returns it.
func parseContainerID(r io.Reader) string {
	scn := bufio.NewScanner(r)
	for scn.Scan() {
		path := expLine.FindStringSubmatch(scn.Text())
		if len(path) != 2 {
			// invalid entry, continue
			continue
		}
		if parts := expContainerID.FindStringSubmatch(path[1]); len(parts) == 2 {
			return parts[1]
		}
	}
	return ""
}

// readContainerID attempts to return the container ID from the provided file path or empty on failure.
func readContainerID(fpath string) string {
	f, err := os.Open(fpath)
	if err != nil {
		return ""
	}
	defer f.Close()
	return parseContainerID(f)
}

type cacheVal struct {
	containerID string
	accessed    atomic.Value
}

var containerCache map[int32]*cacheVal = make(map[int32]*cacheVal)
var cachelock sync.RWMutex

const cacheExpire = 5 * time.Minute

func cachedContainerID(pid int32) (string, bool) {
	cachelock.RLock()
	defer cachelock.RUnlock()
	if v, ok := containerCache[pid]; ok {
		t := v.accessed.Load().(time.Time)
		if t.Before(time.Now().Add(-cacheExpire)) {
			// If we haven't seen this pid in 5 minutes,
			// it should be re-read.
			return "", false
		}
		v.accessed.Store(time.Now())
		return v.containerID, true
	}
	return "", false
}

func insertContainerID(pid int32, cid string) {
	cachelock.Lock()
	defer cachelock.Unlock()
	// We'll clean the cache whenever we insert a new container ID.
	for k, v := range containerCache {
		t := v.accessed.Load().(time.Time)
		if t.Before(time.Now().Add(-cacheExpire)) {
			delete(containerCache, k)
		}
	}
	cv := &cacheVal{containerID: cid}
	cv.accessed.Store(time.Now())
	containerCache[pid] = cv
}

var createPath func(pid int32) string = func(pid int32) string {
	return fmt.Sprintf(cgroupPath, pid)
}

// retrieveContainerID looks in the local cache for a container ID associated with the given pid.
// If there is a valid (not stale) container ID for the given pid, that is returned. Otherwise the
// container ID is parsed using readContainerID.
func retrieveContainerID(pid int32) string {
	if id, ok := cachedContainerID(pid); ok {
		return id
	}
	if cid := readContainerID(createPath(pid)); cid != "" {
		insertContainerID(pid, cid)
		return cid
	}
	return ""
}

// connContext is a function that injects a Unix Domain Socket's User Credentials into the
// context.Context object provided. This is useful as the ConnContext member of an http.Server, to
// provide User Credentials to http handlers.
func connContext(ctx context.Context, c net.Conn) context.Context {
	s, ok := c.(*net.UnixConn)
	if !ok {
		return ctx
	}
	file, err := s.File()
	if err != nil {
		return ctx
	}
	fd := int(file.Fd())
	acct, err := syscall.GetsockoptUcred(fd, syscall.SOL_SOCKET, syscall.SO_PEERCRED)
	ctx = context.WithValue(ctx, ucredKey{}, acct)
	return ctx
}

// getContainerID attempts first to read the container ID set by the client in the request
// header. If no such header is present or the value is empty, it will attempt to use the UDS
// credentials to determine the connection's container ID.
func getContainerID(req *http.Request) string {
	if id := req.Header.Get(headerContainerID); id != "" {
		return id
	}
	ucred, ok := req.Context().Value(ucredKey{}).(*syscall.Ucred)
	if !ok || ucred == nil {
		return ""
	}
	return retrieveContainerID(ucred.Pid)
}
