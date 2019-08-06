package net

import (
	"fmt"
	"io/ioutil"
	"strings"
)

func getLocalDefinedNTPServers() ([]string, error) {
	return getNTPServersFromFiles([]string{"/etc/ntp.conf", "etc/xntp.conf"})
}

func getNTPServersFromFiles(files []string) ([]string, error) {
	serversMap := make(map[string]bool)

	for _, conf := range files {
		content, err := ioutil.ReadFile(conf)
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
		return nil, fmt.Errorf("Cannot find ntp server in %s", strings.Join(files, ", "))
	}

	var servers []string
	for key := range serversMap {
		servers = append(servers, key)
	}

	return servers, nil
}
