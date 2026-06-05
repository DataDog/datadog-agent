// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

package ntp

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// timesyncdDropInDirs is the list of directories systemd reads timesyncd drop-in
// configs from. Declared as a var so tests can override it. Override semantics
// (a drop-in's NTP= replaces an earlier one) are not modeled: all values are
// unioned, which is acceptable since the check only needs a server to query.
var timesyncdDropInDirs = []string{
	"/etc/systemd/timesyncd.conf.d",
	"/run/systemd/timesyncd.conf.d",
	"/usr/local/lib/systemd/timesyncd.conf.d",
	"/usr/lib/systemd/timesyncd.conf.d",
}

func getLocalDefinedNTPServers() ([]string, error) {
	files := []string{"/etc/ntp.conf", "/etc/xntp.conf", "/etc/chrony.conf", "/etc/chrony/chrony.conf", "/etc/ntpd.conf", "/etc/openntpd/ntpd.conf", "/etc/systemd/timesyncd.conf"}
	for _, dir := range timesyncdDropInDirs {
		matches, _ := filepath.Glob(filepath.Join(dir, "*.conf"))
		files = append(files, matches...)
	}
	return getNTPServersFromFiles(files)
}

func getNTPServersFromFiles(files []string) ([]string, error) {
	serversMap := make(map[string]bool)

	for _, conf := range files {
		content, err := os.ReadFile(conf)
		if err == nil {
			lines := strings.SplitSeq(string(content), "\n")

			for line := range lines {
				line = strings.TrimSpace(line)
				if idx := strings.Index(line, "#"); idx >= 0 {
					line = strings.TrimSpace(line[:idx])
				}
				if line == "" {
					continue
				}

				// chrony / ntp.conf: "server|pool|peer <host>"
				fields := strings.Fields(line)
				if len(fields) >= 2 && (fields[0] == "server" || fields[0] == "pool" || fields[0] == "peer") {
					serversMap[fields[1]] = true
					continue
				}

				// systemd-timesyncd: "NTP=host1 host2" or "FallbackNTP=host1 host2"
				if key, value, ok := strings.Cut(line, "="); ok {
					key = strings.TrimSpace(key)
					if key == "NTP" || key == "FallbackNTP" {
						for _, host := range strings.Fields(value) {
							serversMap[host] = true
						}
					}
				}
			}
		}
	}

	if len(serversMap) == 0 {
		return nil, fmt.Errorf("Cannot find NTP server in %s", strings.Join(files, ", "))
	}

	var servers []string
	for key := range serversMap {
		servers = append(servers, key)
	}

	return servers, nil
}
