// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build darwin

package sysctl

import (
	"encoding/binary"
	"strings"
	"syscall"
	"time"
)

type sctl struct {
	ttl      time.Duration
	lastRead time.Time
	path     string
}

func newSCtl(sysctl string, cacheFor time.Duration) *sctl {
	return &sctl{
		ttl:  cacheFor,
		path: sysctl,
	}
}

// returns the value in string format; is valid
// when the second return is `true`; false with error nil
// indicates the duration hasn't exceeded, false withn
// error indicates actual error.
func (s *sctl) get(now time.Time) (string, bool, error) {
	if !s.lastRead.IsZero() && s.lastRead.Add(s.ttl).After(now) {
		return "", false, nil
	}

	content, err := syscall.Sysctl(s.path)
	if err != nil {
		return "", false, err
	}

	s.lastRead = now
	return strings.TrimSpace(content), true, nil
}

// Int16 represents a 16 bit int sysctl
type Int16 struct {
	*sctl
	v uint16
}

// NewInt16 creates a new sysctl.Int16
//
// `sysctl` is the path for the sysctl, e.g. net.inet.ip.portrange.first
// `cacheFor` caches the sysctl's value for the given time duration;
// `0` disables caching
func NewInt16(sysctl string, cacheFor time.Duration) *Int16 {
	return &Int16{sctl: newSCtl(sysctl, cacheFor)}
}

// Get gets the current value of the sysctl
func (i *Int16) Get() (uint16, error) {
	return i.get(time.Now())
}

func (i *Int16) get(now time.Time) (uint16, error) {
	v, updated, err := i.sctl.get(now)
	if err == nil && updated {
		asBytes := []byte(v)
		i.v = binary.LittleEndian.Uint16(asBytes)

	}

	return i.v, err
}
