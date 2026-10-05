// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package nodetreemodel

import (
	"fmt"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
)

var pathExtractor = regexp.MustCompile(`^\${([^}]+)}(.*)$`)

// Relative path is a feature used by defaults to runtime info.
// The path are express using `/` and localize when needed.
//
// We support (The exact values for each are defined by pkg/util/defaultpaths):
// conf_path:
//     linux: /etc/datadog-agent/
//     windows: programdata Dir, likely c:\programdata\datadog
//     darwin: /opt/datadog-agent/etc/
// install_path:
//     linux: /opt/datadog-agent/
//     windows: likely c:\Program Files\Datadog\Datadog Agent
//     darwin: /opt/datadog-agent
// run_path:
//    linux: {install_path}/run
//    darwin: /opt/datadog-agent/run
//    windows: {conf_path}\run
// log_path:
//     linux: /var/log/datadog
//     darwin: /opt/datadog-agent/logs
//     windows: c:\programdata\datadog\logs

func resolvePath(prefix string, path string) string {
	path = strings.TrimPrefix(path, "/")
	if runtime.GOOS == "windows" {
		path = filepath.FromSlash(path)
	}
	return filepath.Join(prefix, path)
}

func resolve(n *nodeImpl, confPath string, installPath string, runPath string, logPath string) error {
	for name, node := range n.children {
		if node.IsLeafNode() {
			if sval, ok := node.val.(string); ok {
				res := pathExtractor.FindStringSubmatch(sval)
				if len(res) >= 2 {
					relativeTo := res[1]
					path := res[2]
					switch relativeTo {
					case "conf_path":
						node.val = resolvePath(confPath, path)
					case "install_path":
						node.val = resolvePath(installPath, path)
					case "run_path":
						node.val = resolvePath(runPath, path)
					case "log_path":
						node.val = resolvePath(logPath, path)
					default:
						return fmt.Errorf("%s is using an invalid relative path '%v''", name, res)
					}
				}
			}
		} else {
			err := resolve(node, confPath, installPath, runPath, logPath)
			if err != nil {
				return fmt.Errorf("%s.%s", name, err.Error())
			}
		}
	}
	return nil
}

// resolveRelativePath resolve default value that rely on runtime path (ie path relative to the install dir, log dir,
// ...)
func (c *ntmConfig) resolveRelativePath(confPath string, installPath string, runPath string, logPath string) error {
	return resolve(c.defaults, confPath, installPath, runPath, logPath)
}
