// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build darwin

package listener

import (
	"os/exec"
	"strconv"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// const maxUDPFrameLen = 9216
// Try to read the OS UDP max datagram size; fall back to a sane default if unavailable.
func darwinMaxUDPFrameLen() int {
	// macOS uses net.inet.udp.maxdgram; keep sys.inet.* as a fallback for BSD variants.
	for _, oid := range []string{"net.inet.udp.maxdgram", "sys.inet.udp.maxdgram"} {
		out, err := exec.Command("sysctl", "-n", oid).Output()
		if err != nil {
			continue
		}
		s := strings.TrimSpace(string(out))
		if v, err := strconv.Atoi(s); err == nil && v > 0 {
			return v
		}
	}
	log.Warnf("darwinMaxUDPFrameLen: unable to determine UDP max datagram via sysctl; falling back to %d", 9216)
	return 9216
}

var maxUDPFrameLen = darwinMaxUDPFrameLen()
