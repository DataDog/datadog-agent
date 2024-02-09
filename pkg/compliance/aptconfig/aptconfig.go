// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package aptconfig is a compliance submodule that is able to parse the APT tool
// configuration and export it as a log.
package aptconfig

import (
	"context"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"unicode"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// SeclFilter only selects ubuntu hosts for now on which is want to test out
// and apply unattended upgrades checks.
const SeclFilter = `os.id == "ubuntu"`

const (
	resourceType = "host_apt_config"

	aptConfFile         = "/etc/apt/apt.conf"
	aptConfFragmentsDir = "/etc/apt/apt.conf.d"
	systemdConfDir      = "/etc/systemd/system"
)

// LoadConfiguration exports the aggregated APT configuration file and parts
// of the systemd configuration files related to APT timers.
func LoadConfiguration(_ context.Context, hostroot string) (string, interface{}) {
	defer func() {
		if err := recover(); err != nil {
			log.Warnf("could not parse APT configuration properly: %v", err)
		}
	}()

	aptConfDir := filepath.Join(hostroot, aptConfFragmentsDir)
	aptConfFiles, _ := filepath.Glob(filepath.Join(aptConfDir, "*"))
	sort.Strings(aptConfFiles)
	aptConfFiles = append([]string{filepath.Join(hostroot, aptConfFile)}, aptConfFiles...)

	aptConfs := make(map[string]interface{})
	for _, path := range aptConfFiles {
		data, err := readFileLimit(path)
		if err == nil {
			conf := parseAPTConfiguration(data)
			for k, v := range conf {
				aptConfs[k] = v
			}
		}
	}

	systemdConfDir := filepath.Join(hostroot, systemdConfDir)
	systemdTimersConfs := make(map[string]interface{})
	var systemdConfFiles []string
	_ = filepath.Walk(systemdConfDir, func(path string, info fs.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return err
		}
		base := filepath.Base(path)
		if base == "apt-daily-upgrade.timer" || base == "apt-daily.timer" {
			systemdConfFiles = append(systemdConfFiles, path)
		}
		return nil
	})
	sort.Strings(systemdConfFiles)

	for _, path := range systemdConfFiles {
		data, err := readFileLimit(path)
		if err == nil {
			base := filepath.Base(path)
			conf := parseSystemdConf(data)
			systemdTimersConfs[base] = conf
		}
	}

	resourceData := map[string]interface{}{
		"apt": aptConfs,
		"systemd": map[string]interface{}{
			"timers": systemdTimersConfs,
		},
	}

	return resourceType, resourceData
}

type tokenType int

const (
	eos tokenType = iota
	literal
	blockStart
	blockEnd
	data
	equal
	comment
	comma
	parseError
)

type token struct {
	kind  tokenType
	value string
}

func parseAPTConfiguration(str string) map[string]interface{} {
	conf := make(map[string]interface{})
	var cursor []string
	var key string
loop:
	for {
		var tok token
		str, tok = nextTokenAPT(str)
		switch tok.kind {
		case blockStart:
			cursor = append(cursor, key)
		case blockEnd:
			if len(cursor) > 0 {
				cursor = cursor[:len(cursor)-1]
			}
			key = ""
		case literal:
			key = strings.Join(append(cursor, tok.value), "::")
		case data:
			if key != "" {
				if v, ok := conf[key]; ok {
					if a, ok := v.([]string); ok {
						conf[key] = append(a, tok.value)
					} else if s, ok := v.(string); ok {
						conf[key] = append([]string{s}, tok.value)
					}
				} else {
					conf[key] = tok.value
				}
			}
		case comment, comma:
		case eos:
			break loop
		default:
			break loop
		}
	}
	return conf
}

// man apt.conf.5: https://manpages.ubuntu.com/manpages/trusty/man5/apt.conf.5.html
//
//	> Syntactically the configuration language is modeled after what the ISC
//	> tools such as bind and dhcp use. Lines starting with // are treated as
//	> comments (ignored), as well as all text between /* and */, just like C/C++
//	> comments. Each line is of the form APT::Get::Assume-Yes "true";. The
//	> quotation marks and trailing semicolon are required. The value must be on
//	> one line, and there is no kind of string concatenation. Values must not
//	> include backslashes or extra quotation marks. Option names are made up of
//	> alphanumeric characters and the characters "/-:._+". A new scope can be
//	> opened with curly braces, like this:
func nextTokenAPT(str string) (string, token) {
	str = eatWhitespace(str)
	if len(str) == 0 {
		return "", token{kind: eos}
	}
	var t token
	c := str[0]
	i := 0
	switch {
	case c == '/' && strings.HasPrefix(str, "/*"):
		t.kind = comment
		i = 2
		for _, r := range str[2:] {
			i++
			if r == '*' && strings.HasPrefix(str[i:], "*/") {
				i++
				break
			}
		}
	case c == '/' && strings.HasPrefix(str, "//"):
		t.kind = comment
		i = 2
		for _, r := range str[2:] {
			i++
			if r == '\n' {
				break
			}
		}
	case c == '#':
		t.kind = comment
		i = 1
		for _, r := range str[1:] {
			i++
			if r == '\n' {
				break
			}
		}
	case (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z'):
		t.kind = literal
		i = 1
		for _, r := range str[1:] {
			if !isLiteral(r) {
				break
			}
			i++
		}
	case c == '{':
		t.kind = blockStart
		i = 1
	case c == ';':
		t.kind = comma
		i = 1
	case c == '}':
		t.kind = blockEnd
		i = 1
	case c == '"':
		ok := false
		t.kind = data
		i = 1
		for _, r := range str[1:] {
			i++
			if r == '"' {
				if value, err := strconv.Unquote(str[:i]); err == nil {
					ok = true
					t.value = value
					break
				}
			}
		}
		if !ok {
			t.kind = parseError
		}
	}
	if i == 0 {
		t.kind = parseError
	} else if t.kind != data {
		t.value = str[:i]
	}
	return str[i:], t
}

func isLiteral(r rune) bool {
	return (r >= 'A' && r <= 'Z') ||
		(r >= 'a' && r <= 'z') ||
		(r >= '0' && r <= '9') ||
		(r == '/') || (r == '-') || (r == ':') || (r == '.') || (r == '_') || (r == '+')
}

func eatWhitespace(str string) string {
	i := 0
	for _, r := range str {
		if !unicode.IsSpace(r) {
			break
		}
		i++
	}
	return str[i:]
}

func readFileLimit(path string) (string, error) {
	const maxSize = 64 * 1024
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, maxSize))
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// systemd configuration syntax:
// https://www.freedesktop.org/software/systemd/man/systemd.syntax.html
func parseSystemdConf(str string) map[string]string {
	lines := strings.Split(str, "\n")
	conf := make(map[string]string)
	var section = ""
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "[") {
			section = strings.Replace(line[1:], "]", "", 1)
		} else if section != "" {
			parts := strings.SplitN(line, "=", 2)
			if len(parts) == 2 {
				conf[section+"/"+parts[0]] = parts[1]
			}
		}
	}
	return conf
}
