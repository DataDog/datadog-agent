// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

package net

import (
	"fmt"
	"os"
	"strings"
)

func getLocalDefinedNTPServers() ([]string, error) {
	return getNTPServersFromFiles([]string{"/etc/ntp.conf", "/etc/xntp.conf", "/etc/chrony.conf", "/etc/ntpd.conf", "/etc/openntpd/ntpd.conf"})
}

func getNTPServersFromFiles(files []string) ([]string, error) {
	serversMap := make(map[string]bool)

	for _, conf := range files {
		content, err := os.ReadFile(conf)
		if err == nil {
			lines := strings.Split(string(content), "\n")

			for _, line := range lines {
				line = strings.TrimSpace(line)
				fields := strings.Fields(line)
				if len(fields) >= 2 && fields[0] == "server" {
					serversMap[fields[1]] = true
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
