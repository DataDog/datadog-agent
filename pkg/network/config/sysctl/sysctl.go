// +build linux

package sysctl

import (
	"io/ioutil"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// String represents a string sysctl
type String struct {
	*sctl
	v string
}

// NewString creates a new sysctl.String
//
// `procRoot` points to the procfs root, e.g. /proc
// `sysctl` is the path for the sysctl, e.g. /proc/sys/<sysctl>
// `cacheFor` caches the sysctl's value for the given time duration;
// `0` disables caching
func NewString(procRoot, sysctl string, cacheFor time.Duration) *String {
	return &String{sctl: newSCtl(procRoot, sysctl, cacheFor)}
}

// Get gets the current value of the sysctl
func (s *String) Get() (string, error) {
	v, updated, err := s.sctl.Get()
	if err == nil && updated {
		s.v = v
	}

	return s.v, err
}

// Int represents an int sysctl
type Int struct {
	*sctl
	v int
}

// NewInt creates a new sysctl.Int
//
// `procRoot` points to the procfs root, e.g. /proc
// `sysctl` is the path for the sysctl, e.g. /proc/sys/<sysctl>
// `cacheFor` caches the sysctl's value for the given time duration;
// `0` disables caching
func NewInt(procRoot, sysctl string, cacheFor time.Duration) *Int {
	return &Int{sctl: newSCtl(procRoot, sysctl, cacheFor)}
}

// Get gets the current value of the sysctl
func (i *Int) Get() (int, error) {
	v, updated, err := i.sctl.Get()
	if err == nil && updated {
		i.v, err = strconv.Atoi(v)
	}

	return i.v, err
}

type sctl struct {
	ttl      int64
	lastRead int64
	path     string
}

func newSCtl(procRoot, sysctl string, cacheFor time.Duration) *sctl {
	return &sctl{
		ttl:  int64(cacheFor.Seconds()),
		path: filepath.Join(procRoot, "sys", sysctl),
	}
}

func (s *sctl) Get() (string, bool, error) {
	now := time.Now().Unix()
	if s.lastRead > 0 && s.lastRead+s.ttl > now {
		return "", false, nil
	}

	content, err := ioutil.ReadFile(s.path)
	if err != nil {
		return "", false, err
	}

	s.lastRead = now
	return strings.TrimSpace(string(content)), true, nil
}
