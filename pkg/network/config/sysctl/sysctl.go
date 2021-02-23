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
	return s.get(time.Now())
}

func (s *String) get(now time.Time) (string, error) {
	v, updated, err := s.sctl.get(now)
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
	return i.get(time.Now())
}

func (i *Int) get(now time.Time) (int, error) {
	v, updated, err := i.sctl.get(now)
	if err == nil && updated {
		i.v, err = strconv.Atoi(v)
	}

	return i.v, err
}

type sctl struct {
	ttl      time.Duration
	lastRead time.Time
	path     string
}

func newSCtl(procRoot, sysctl string, cacheFor time.Duration) *sctl {
	return &sctl{
		ttl:  cacheFor,
		path: filepath.Join(procRoot, "sys", sysctl),
	}
}

func (s *sctl) get(now time.Time) (string, bool, error) {
	if !s.lastRead.IsZero() && s.lastRead.Add(s.ttl).After(now) {
		return "", false, nil
	}

	content, err := ioutil.ReadFile(s.path)
	if err != nil {
		return "", false, err
	}

	s.lastRead = now
	return strings.TrimSpace(string(content)), true, nil
}
