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
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/compliance/utils"

	"github.com/shirou/gopsutil/v3/process"
	yaml "gopkg.in/yaml.v3"
)

const (
	maxFileSize = 1 * 1024 * 1024

	postgresqlResourceType = "db_postgresql"

	cassandraResourceType = "db_cassandra"
	cassandraLogbackPath  = "/etc/cassandra/logback.xml"
	cassandraConfigGlob   = "/etc/cassandra/cassandra.y?ml"

	mongoDBResourceType = "db_mongodb"
	mongoDBConfigPath   = "/etc/mongod.conf"
)

func relPath(hostroot, configPath string) string {
	if hostroot == "" {
		return configPath
	}
	path, err := filepath.Rel(hostroot, configPath)
	if err != nil {
		path = configPath
	}
	return filepath.Join("/", path)
}

func readFileLimit(name string) ([]byte, error) {
	f, err := os.Open(name)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	r := io.LimitReader(f, maxFileSize)
	b, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	return b, nil
}

// GetProcResourceType returns the type of database resource associated with
// the given process.
func GetProcResourceType(proc *process.Process) (string, bool) {
	name, _ := proc.Name()
	switch name {
	case "postgres":
		return postgresqlResourceType, true
	case "mongod":
		return mongoDBResourceType, true
	case "java":
		cmdline, _ := proc.CmdlineSlice()
		if len(cmdline) > 0 && cmdline[len(cmdline)-1] == "org.apache.cassandra.service.CassandraDaemon" {
			return cassandraResourceType, true
		}
	}
	return "", false
}

// LoadConfiguration loads and returns an optional DBResource associated with the
// given process PID.
func LoadConfiguration(ctx context.Context, rootPath string, proc *process.Process) (string, *DBConfig, bool) {
	resourceType, ok := GetProcResourceType(proc)
	if !ok {
		return "", nil, false
	}
	var conf *DBConfig
	switch resourceType {
	case postgresqlResourceType:
		conf, ok = LoadPostgreSQLConfig(ctx, rootPath, proc)
	case mongoDBResourceType:
		conf, ok = LoadMongoDBConfig(ctx, rootPath, proc)
	case cassandraResourceType:
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

	containerID, _ := utils.GetProcessContainerID(pid)
	hostroot, ok := utils.GetProcessRootPath(pid)
	if !ok {
		return nil, false
	}

	var conf *DBConfig
	switch resourceType {
	case postgresqlResourceType:
		conf, ok = LoadPostgreSQLConfig(ctx, hostroot, proc)
	case mongoDBResourceType:
		conf, ok = LoadMongoDBConfig(ctx, hostroot, proc)
	case cassandraResourceType:
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

// LoadMongoDBConfig loads and extracts the MongoDB configuration data found
// on the system.
func LoadMongoDBConfig(ctx context.Context, hostroot string, proc *process.Process) (*DBConfig, bool) {
	configLocalPath := mongoDBConfigPath
	var result DBConfig
	result.ProcessUser, _ = proc.UsernameWithContext(ctx)
	result.ProcessName, _ = proc.NameWithContext(ctx)

	cmdline, _ := proc.CmdlineSlice()
	for i, arg := range cmdline {
		if arg == "--config" && i+1 < len(cmdline) {
			configLocalPath = filepath.Clean(cmdline[i+1])
			break
		}
	}

	configPath := filepath.Join(hostroot, configLocalPath)
	fi, err := os.Stat(configPath)
	if err != nil || fi.IsDir() {
		result.ConfigFileUser = "<none>"
		result.ConfigFileGroup = "<none>"
		result.ConfigData = map[string]interface{}{}
		return &result, true
	}

	var configData mongoDBConfig
	result.ConfigFileUser = utils.GetFileUser(fi)
	result.ConfigFileGroup = utils.GetFileGroup(fi)
	result.ConfigFileMode = uint32(fi.Mode())
	result.ConfigFilePath = relPath(hostroot, configPath)
	configRaw, err := readFileLimit(configPath)
	if err != nil {
		return nil, false
	}
	if err := yaml.Unmarshal(configRaw, &configData); err != nil {
		return nil, false
	}
	result.ConfigData = &configData
	return &result, true
}

// LoadCassandraConfig loads and extracts the Cassandra configuration data
// found on the system.
func LoadCassandraConfig(ctx context.Context, hostroot string, proc *process.Process) (*DBConfig, bool) {
	var result DBConfig
	if proc != nil {
		result.ProcessUser, _ = proc.UsernameWithContext(ctx)
		result.ProcessName, _ = proc.NameWithContext(ctx)
	}

	var configData *cassandraDBConfig
	matches, _ := filepath.Glob(filepath.Join(hostroot, cassandraConfigGlob))
	for _, configPath := range matches {
		fi, err := os.Stat(configPath)
		if err != nil || fi.IsDir() {
			continue
		}
		result.ConfigFileUser = utils.GetFileUser(fi)
		result.ConfigFileGroup = utils.GetFileGroup(fi)
		result.ConfigFileMode = uint32(fi.Mode())
		result.ConfigFilePath = relPath(hostroot, configPath)
		configRaw, err := readFileLimit(configPath)
		if err != nil {
			continue
		}
		if err := yaml.Unmarshal(configRaw, &configData); err == nil {
			break
		}
	}

	if configData == nil {
		return nil, false
	}

	logback, err := readFileLimit(filepath.Join(hostroot, cassandraLogbackPath))
	if err == nil {
		configData.LogbackFilePath = cassandraLogbackPath
		configData.LogbackFileContent = string(logback)
	}
	result.ConfigData = configData
	return &result, true
}

// LoadPostgreSQLConfig loads and extracts the PostgreSQL configuration data found on the system.
func LoadPostgreSQLConfig(ctx context.Context, hostroot string, proc *process.Process) (*DBConfig, bool) {
	var result DBConfig

	// Let's try to parse the -D command line argument containing the data
	// directory of PG. Configuration file may be located in this directory.
	result.ProcessUser, _ = proc.UsernameWithContext(ctx)
	result.ProcessName, _ = proc.NameWithContext(ctx)

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
		if strings.HasPrefix(arg, "--config-file=") {
			hintPath = filepath.Clean(strings.TrimPrefix(arg, "--config-file="))
			break
		}
	}

	configPath, ok := locatePGConfigFile(hostroot, hintPath)
	if !ok {
		// postgres can be setup without a configuration file.
		result.ConfigFileUser = "<none>"
		result.ConfigFileGroup = "<none>"
		result.ConfigData = map[string]interface{}{}
		return &result, true
	}
	fi, err := os.Stat(filepath.Join(hostroot, configPath))
	if err != nil || fi.IsDir() {
		return nil, false
	}
	result.ConfigFileUser = utils.GetFileUser(fi)
	result.ConfigFileGroup = utils.GetFileGroup(fi)
	result.ConfigFileMode = uint32(fi.Mode())
	result.ConfigFilePath = configPath
	configData, ok := parsePGConfig(hostroot, configPath, 0)
	if ok {
		result.ConfigData = configData
		return &result, true
	}
	return nil, false
}

func locatePGConfigFile(hostroot, hintPath string) (string, bool) {
	var pgConfigGlobs = []string{
		"/etc/postgresql/postgresql.conf",
		"/etc/postgresql/*/*/postgresql.conf",
		"/var/lib/pgsql/*/data/postgresql.conf",
		"/var/lib/pgsql/data/postgresql.conf",
		"/var/lib/postgresql/*/data/postgresql.conf",
	}
	if hintPath != "" {
		pgConfigGlobs = append([]string{hintPath}, pgConfigGlobs...)
	}
	for _, pattern := range pgConfigGlobs {
		files, _ := filepath.Glob(filepath.Join(hostroot, pattern))
		if len(files) > 0 {
			return relPath(hostroot, files[0]), true
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
func parsePGConfig(hostroot, configPath string, includeDepth int) (map[string]interface{}, bool) {
	// Let's protect ourselves from circular "includes" by limiting the number
	//	of possible include to 10. This is the same parameter as postgresql:
	//	https://github.com/postgres/postgres/blob/5abbd97fef6f18eef91bfed7d2057c39bd204702/src/include/utils/conffiles.h#L18
	if includeDepth > 10 {
		return nil, false
	}
	if configPath == "" {
		return nil, false
	}

	b, err := readFileLimit(filepath.Join(hostroot, configPath))
	if err != nil {
		return nil, false
	}

	config := make(map[string]interface{})

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
				included, ok := parsePGConfig(hostroot, includedPath, includeDepth+1)
				if ok {
					for k, v := range included {
						config[k] = v
					}
				}
			} else if key == "include_dir" {
				includedPath := val
				if !filepath.IsAbs(includedPath) {
					includedPath = filepath.Join(filepath.Dir(configPath), includedPath)
				}
				glob := filepath.Join(hostroot, includedPath, "*.conf")
				matches, _ := filepath.Glob(glob)
				for _, match := range matches {
					includedFile := relPath(hostroot, match)
					included, ok := parsePGConfig(hostroot, includedFile, includeDepth+1)
					if ok {
						for k, v := range included {
							config[k] = v
						}
					}
				}
			} else {
				if val == "=" { // skip optional equal token
					val, _ = t.next()
				}
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

	return config, true
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
