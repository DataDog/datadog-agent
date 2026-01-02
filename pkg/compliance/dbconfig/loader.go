// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package dbconfig is a compliance submodule that is able to parse and export
// databases applications configurations.
package dbconfig

import (
	"bufio"
	"bytes"
	"context"
	"encoding/xml"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/compliance/types"
	"github.com/DataDog/datadog-agent/pkg/compliance/utils"

	"github.com/shirou/gopsutil/v4/process"
	yaml "gopkg.in/yaml.v3"
)

const (
	maxFileSize = 1 * 1024 * 1024

	cassandraLogbackPath = "/etc/cassandra/logback.xml"
	cassandraConfigGlob  = "/etc/cassandra/cassandra.y?ml"

	mongoDBConfigPath = "/etc/mongod.conf"
)

// GetProcResourceType returns the type of database resource associated with
// the given process.
func GetProcResourceType(proc *process.Process) (types.ResourceType, bool) {
	name, _ := proc.Name()
	switch name {
	case "postgres":
		return types.ResourceTypeDbPostgresql, true
	case "mongod":
		return types.ResourceTypeDbMongodb, true
	case "java":
		cmdline, _ := proc.CmdlineSlice()
		if len(cmdline) > 0 && cmdline[len(cmdline)-1] == "org.apache.cassandra.service.CassandraDaemon" {
			return types.ResourceTypeDbCassandra, true
		}
	}
	return "", false
}

// LoadConfiguration loads and returns an optional DBResource associated with the
// given process PID.
func LoadConfiguration(ctx context.Context, rootPath string, proc *process.Process) (types.ResourceType, *DBConfig, bool) {
	resourceType, ok := GetProcResourceType(proc)
	if !ok {
		return "", nil, false
	}
	var conf *DBConfig
	switch resourceType {
	case types.ResourceTypeDbPostgresql:
		conf, ok = LoadPostgreSQLConfig(ctx, rootPath, proc)
	case types.ResourceTypeDbMongodb:
		conf, ok = LoadMongoDBConfig(ctx, rootPath, proc)
	case types.ResourceTypeDbCassandra:
		conf, ok = LoadCassandraConfig(ctx, rootPath, proc)
	default:
		ok = false
	}
	if !ok || conf == nil {
		return "", nil, false
	}
	return resourceType, conf, true
}

// LoadDBResourceFromPID loads and returns an optional DBResource associated
// with the given process PID.
func LoadDBResourceFromPID(ctx context.Context, pid int32) (*DBResource, bool) {
	proc, err := process.NewProcessWithContext(ctx, pid)
	if err != nil {
		return nil, false
	}

	resourceType, ok := GetProcResourceType(proc)
	if !ok {
		return nil, false
	}
	if !checkProcExe(proc) {
		return nil, false
	}

	containerID, _ := utils.GetProcessContainerID(pid)
	hostroot, ok := utils.GetProcessRootPath(pid)
	if !ok || !filepath.IsAbs(hostroot) {
		return nil, false
	}

	var conf *DBConfig
	switch resourceType {
	case types.ResourceTypeDbPostgresql:
		conf, ok = LoadPostgreSQLConfig(ctx, hostroot, proc)
	case types.ResourceTypeDbMongodb:
		conf, ok = LoadMongoDBConfig(ctx, hostroot, proc)
	case types.ResourceTypeDbCassandra:
		conf, ok = LoadCassandraConfig(ctx, hostroot, proc)
	default:
		ok = false
	}
	if !ok || conf == nil {
		return nil, false
	}
	return &DBResource{
		Type:        resourceType,
		ContainerID: string(containerID),
		Config:      *conf,
	}, true
}

func checkProcExe(proc *process.Process) bool {
	name, _ := proc.Name()
	exe, _ := proc.Exe()
	return name == filepath.Base(exe)
}

// LoadMongoDBConfig loads and extracts the MongoDB configuration data found
// on the system.
func LoadMongoDBConfig(ctx context.Context, hostroot string, proc *process.Process) (*DBConfig, bool) {
	configLocalPath := mongoDBConfigPath
	result := newDBConfig(ctx, proc)
	cmdline, _ := proc.CmdlineSlice()
	for i, arg := range cmdline {
		if arg == "--config" && i+1 < len(cmdline) {
			configLocalPath = filepath.Clean(cmdline[i+1])
			break
		}
	}
	result.ProcessFlags = make(map[string]string)
	foreachFlags(cmdline, func(k, v string) {
		if strings.HasPrefix(k, "--") {
			if _, redacted := mongoDBRedactedFlags[k]; redacted || strings.Contains(strings.ToLower(k), "password") {
				result.ProcessFlags[k] = "<redacted>"
			} else {
				result.ProcessFlags[k] = v
			}
		}
	})

	root, err := os.OpenRoot(hostroot)
	if err != nil {
		return nil, false
	}
	defer root.Close()

	configRaw, fi, err := utils.ReadProcessFileLimit(proc, root, configLocalPath, maxFileSize)
	if err != nil {
		return nil, false
	}
	var configData mongoDBConfig
	if err := yaml.Unmarshal(configRaw, &configData); err != nil {
		return nil, false
	}
	setConfigFileMetadata(&result, fi, configLocalPath, &configData)
	return &result, true
}

// LoadCassandraConfig loads and extracts the Cassandra configuration data
// found on the system.
func LoadCassandraConfig(ctx context.Context, hostroot string, proc *process.Process) (*DBConfig, bool) {
	result := newDBConfig(ctx, proc)
	var configData *cassandraDBConfig
	root, err := os.OpenRoot(hostroot)
	if err != nil {
		return nil, false
	}
	defer root.Close()
	matches := utils.RootedGlob(hostroot, cassandraConfigGlob)
	for _, configPath := range matches {
		configRaw, fi, err := utils.ReadProcessFileLimit(proc, root, configPath, maxFileSize)
		if err != nil {
			continue
		}
		if err := yaml.Unmarshal(configRaw, &configData); err == nil {
			setConfigFileMetadata(&result, fi, configPath, configData)
			break
		}
	}
	if configData == nil {
		return nil, false
	}
	logbackRaw, _, err := utils.ReadProcessFileLimit(proc, root, cassandraLogbackPath, maxFileSize)
	if err == nil {
		configData.LogbackFilePath = cassandraLogbackPath
		var logbackConfig cassandraLogback
		if err := xml.Unmarshal(logbackRaw, &logbackConfig); err == nil {
			configData.Logback = &logbackConfig
		}
	}
	return &result, true
}

// LoadPostgreSQLConfig loads and extracts the PostgreSQL configuration data found on the system.
func LoadPostgreSQLConfig(ctx context.Context, hostroot string, proc *process.Process) (*DBConfig, bool) {
	result := newDBConfig(ctx, proc)

	// Let's try to parse the -D command line argument containing the data
	// directory of PG. Configuration file may be located in this directory.

	var hintPath string
	cmdline, _ := proc.CmdlineSlice()
	for i, arg := range cmdline {
		if arg == "-D" && i+1 < len(cmdline) {
			hintPath = filepath.Join(cmdline[i+1], "postgresql.conf")
			break
		}
		if arg == "--config-file" && i+1 < len(cmdline) {
			hintPath = filepath.Clean(cmdline[i+1])
			break
		}
		if after, ok := strings.CutPrefix(arg, "--config-file="); ok {
			hintPath = filepath.Clean(after)
			break
		}
	}

	root, err := os.OpenRoot(hostroot)
	if err != nil {
		return nil, false
	}
	defer root.Close()

	configData := make(map[string]string)
	parseCmdline := func(configData map[string]string) {
		foreachFlags(cmdline, func(k, v string) {
			if name, ok := strings.CutPrefix(k, "--"); ok {
				if _, ok := postgresKnownConfigKeys[name]; ok {
					configData[name] = v
				}
			}
		})
	}

	configPath, ok := locatePGConfigFile(root, proc, hintPath)
	if !ok {
		// postgres can be setup without a configuration file.
		parseCmdline(configData)
		result.ConfigData = configData
		return &result, true
	}
	fi, ok := parsePGConfig(proc, root, configPath, configData, 0)
	if !ok {
		return nil, false
	}
	parseCmdline(configData)
	setConfigFileMetadata(&result, fi, configPath, configData)
	return &result, true
}

func newDBConfig(ctx context.Context, proc *process.Process) DBConfig {
	user, _ := proc.UsernameWithContext(ctx)
	name, _ := proc.NameWithContext(ctx)
	if user == "" {
		if uids, _ := proc.UidsWithContext(ctx); len(uids) > 0 {
			user = fmt.Sprintf("uid:%d", uids[0]) // RUID
		}
	}
	if user == "" {
		user = "<unknown>"
	}
	if name == "" {
		name = "<unknown>"
	}
	return DBConfig{
		ProcessUser: user,
		ProcessName: name,
	}
}

func setConfigFileMetadata(dbconfig *DBConfig, fi os.FileInfo, configPath string, configData interface{}) {
	fileUser := utils.GetFileUser(fi)
	fileGroup := utils.GetFileGroup(fi)
	fileMode := uint32(fi.Mode())
	dbconfig.ConfigFileUser = fileUser
	dbconfig.ConfigFileGroup = fileGroup
	dbconfig.ConfigFileMode = fileMode
	dbconfig.ConfigFilePath = configPath
	dbconfig.ConfigData = configData
}

func locatePGConfigFile(root *os.Root, proc *process.Process, hintPath string) (string, bool) {
	if hintPath != "" {
		if !filepath.IsAbs(hintPath) {
			cwd, _ := proc.Cwd()
			hintPath = filepath.Join(cwd, hintPath)
		}
		return hintPath, true
	}
	var pgConfigGlobs = []string{
		"/etc/postgresql/postgresql.conf",
		"/etc/postgresql/*/*/postgresql.conf",
		"/var/lib/pgsql/*/data/postgresql.conf",
		"/var/lib/pgsql/data/postgresql.conf",
		"/var/lib/postgresql/*/data/postgresql.conf",
	}
	for _, pattern := range pgConfigGlobs {
		files := utils.RootedGlob(root.Name(), pattern)
		if len(files) > 0 {
			return files[0], true
		}
	}
	return "", false
}

// parsePGConfig tries to load and parse the given configuration file path. It
// does not try to infer the inner types of the configuration and keep them as
// string just as configured in the document. The only applied transformation
// is on boolean values that are all normalized to on / off nomenclature.
// Integer, floats, durations or sizes are kept intact.
//
// references:
//   - https://www.postgresql.org/docs/current/config-setting.html
//   - https://github.com/postgres/postgres/blob/5abbd97fef6f18eef91bfed7d2057c39bd204702/src/backend/utils/misc/guc-file.l#L317-L349
func parsePGConfig(proc *process.Process, root *os.Root, configPath string, config map[string]string, includeDepth int) (os.FileInfo, bool) {
	if filepath.Ext(configPath) != ".conf" {
		return nil, false
	}

	// Let's protect ourselves from circular "includes" by limiting the number
	//	of possible include to 10. This is the same parameter as postgresql:
	//	https://github.com/postgres/postgres/blob/5abbd97fef6f18eef91bfed7d2057c39bd204702/src/include/utils/conffiles.h#L18
	if includeDepth > 10 {
		return nil, false
	}

	b, fi, err := utils.ReadProcessFileLimit(proc, root, configPath, maxFileSize)
	if err != nil {
		return nil, false
	}

	s := bufio.NewScanner(bytes.NewReader(b))
	s.Split(bufio.ScanLines)
	for s.Scan() {
		t := &pgConfLexer{
			buf: s.Bytes(),
		}
		if key, ok := t.next(); ok {
			val, _ := t.next()
			// 'include' directive allows to import another configuration file
			if key == "include" || key == "include_if_exists" {
				includedPath := val
				if !filepath.IsAbs(includedPath) {
					includedPath = filepath.Join(filepath.Dir(configPath), includedPath)
				}
				_, _ = parsePGConfig(proc, root, includedPath, config, includeDepth+1)
			} else if key == "include_dir" {
				includedPath := val
				if !filepath.IsAbs(includedPath) {
					includedPath = filepath.Join(filepath.Dir(configPath), includedPath)
				}
				glob := filepath.Join(includedPath, "*.conf")
				matches := utils.RootedGlob(root.Name(), glob)
				for _, includedPath := range matches {
					_, _ = parsePGConfig(proc, root, includedPath, config, includeDepth+1)
				}
			} else {
				if val == "=" { // skip optional equal token
					val, _ = t.next()
				}
				if _, ok := postgresKnownConfigKeys[key]; ok {
					if val == "on" || val == "true" || val == "yes" {
						config[key] = "on"
					} else if val == "off" || val == "false" || val == "no" {
						config[key] = "off"
					} else {
						config[key] = val
					}
				}
			}
		}
	}

	return fi, true
}

// Simple ASCII lexer for postgresql configuration files (aka. GUC)
type pgConfLexer struct {
	buf []byte
	pos int
}

func (t *pgConfLexer) next() (string, bool) {
	t.skipWhitespace()
	if t.pos >= len(t.buf) {
		return "", false
	}
	c := t.buf[t.pos]
	if c == '#' {
		return "", false
	} else if c == '=' {
		t.pos++
		return string(c), true
	} else if isIdentifierChar(c) {
		return t.scanIdentifier()
	} else if c == '\'' {
		return t.scanQuotedString()
	}
	return "", false
}

func (t *pgConfLexer) scanIdentifier() (string, bool) {
	from := t.pos
	for t.pos < len(t.buf) {
		c := t.buf[t.pos]
		if !isIdentifierChar(c) {
			break
		}
		t.pos++
	}
	if t.pos > from {
		return string(t.buf[from:t.pos]), true
	}
	return "", false
}

// reference: https://github.com/postgres/postgres/blob/5abbd97fef6f18eef91bfed7d2057c39bd204702/src/backend/utils/misc/guc-file.l#L641-L734
func (t *pgConfLexer) scanQuotedString() (string, bool) {
	t.pos++ // skipping the first quote
	var out strings.Builder
	for ; t.pos < len(t.buf); t.pos++ {
		c := t.buf[t.pos]
		// we can peek at the next character in the buffer for escape sequences
		if t.pos+1 < len(t.buf) {
			peeked := t.buf[t.pos+1]
			// backslash escapes
			if c == '\\' {
				t.pos++
				switch peeked {
				case 'b':
					out.WriteByte('\b')
				case 'f':
					out.WriteByte('\f')
				case 'n':
					out.WriteByte('\n')
				case 'r':
					out.WriteByte('\r')
				case 't':
					out.WriteByte('\t')
				case '0', '1', '2', '3', '4', '5', '6', '7':
					// skip octals
				default:
					out.WriteByte(peeked)
				}
				continue
			}
			// doubly quotes escapes
			if c == '\'' && peeked == '\'' {
				t.pos++
				out.WriteByte('\'')
				continue
			}
		}
		if c == '\'' {
			return out.String(), true
		}
		out.WriteByte(c)
	}
	return "", false
}

func (t *pgConfLexer) skipWhitespace() {
	for t.pos < len(t.buf) {
		c := t.buf[t.pos]
		if !isWhiteSpace(c) {
			break
		}
		t.pos++
	}
}

func isWhiteSpace(c uint8) bool {
	return c == ' ' || c == '\t' || c == '\r'
}

func isIdentifierChar(c uint8) bool {
	return (c >= 'a' && c <= 'z') ||
		(c >= 'A' && c <= 'Z') ||
		(c >= '0' && c <= '9') ||
		c == '-' ||
		c == '_' ||
		c == '.'
}

func foreachFlags(cmdline []string, f func(k, v string)) {
	if len(cmdline) <= 1 {
		return
	}
	cmdline = cmdline[1:]
	pendingFlagValue := false
	for i, arg := range cmdline {
		if strings.HasPrefix(arg, "--") && len(arg) > 2 {
			if pendingFlagValue {
				f(cmdline[i-1], "true")
			}
			pendingFlagValue = false
			parts := strings.SplitN(arg, "=", 2)
			if len(parts) == 2 {
				f(parts[0], parts[1])
			} else {
				f(parts[0], "")
				pendingFlagValue = true
			}
		} else if arg != "--" {
			if pendingFlagValue {
				pendingFlagValue = false
				f(cmdline[i-1], arg)
			} else {
				f(arg, "")
			}
		}
	}
	if pendingFlagValue {
		f(cmdline[len(cmdline)-1], "true")
	}
}
