// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package sysctl implements reading of system parameters such as system limits
package sysctl

import (
	"errors"
	"os"
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
	return &String{sctl: newSCtl(procRoot, sysctl, cacheFor, os.ReadFile)}
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
	return &Int{sctl: newSCtl(procRoot, sysctl, cacheFor, os.ReadFile)}
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
	ttl        time.Duration
	lastRead   time.Time
	path       string
	err        error
	fileReader func(path string) ([]byte, error)
}

func newSCtl(procRoot, sysctl string, cacheFor time.Duration, fileReader func(path string) ([]byte, error)) *sctl {
	return &sctl{
		ttl:        cacheFor,
		path:       filepath.Join(procRoot, "sys", sysctl),
		fileReader: fileReader,
	}
}

func (s *sctl) get(now time.Time) (string, bool, error) {
	if s.err != nil || (!s.lastRead.IsZero() && s.lastRead.Add(s.ttl).After(now)) {
		return "", false, s.err
	}

	content, err := s.fileReader(s.path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) || errors.Is(err, os.ErrPermission) {
			// sticky error, we won't try again
			s.err = err
		}

		return "", false, err
	}

	s.lastRead = now
	return strings.TrimSpace(string(content)), true, nil
}

// IntPair represents a sysconfig with a single line with two integer values such as
// 1234   5678
type IntPair struct {
	*sctl
	v1 int
	v2 int
}

// NewIntPair creates a new sysctl.IntPair
// an IntPair is a sysctl that has two space-separated integer values
//
// `procRoot` points to the procfs root, e.g. /proc
// `sysctl` is the path for the sysctl, e.g. /proc/sys/<sysctl>
// `cacheFor` caches the sysctl's value for the given time duration;
// `0` disables caching
func NewIntPair(procRoot, sysctl string, cacheFor time.Duration) *IntPair {
	return &IntPair{sctl: newSCtl(procRoot, sysctl, cacheFor, os.ReadFile)}
}

// Get gets the current value of the sysctl
func (i *IntPair) Get() (int, int, error) {
	return i.get(time.Now())
}

func (i *IntPair) get(now time.Time) (int, int, error) {
	v, updated, err := i.sctl.get(now)
	if err == nil && updated {
		vals := strings.Fields(v)
		i.v1, err = strconv.Atoi(vals[0])
		if err == nil {
			i.v2, err = strconv.Atoi(vals[1])
		}
	}

	return i.v1, i.v2, err
}
