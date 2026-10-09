// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"path"
	"slices"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/scrubber"
)

// redactedSecret replaces a non-empty credential wherever a config is printed or serialized.
const redactedSecret = "********"

// Bounds of a non-zero smb poll_interval. A shorter interval would turn the scan loop into a
// busy loop against the server; a sub-nanosecond or overflowing one would make the scan
// ticker panic.
const (
	SMBMinPollInterval = 100 * time.Millisecond
	SMBMaxPollInterval = time.Hour
)

// SMBConfig holds the connection settings of an smb source, which reads log files from an
// SMB 2/3 share (Windows, Samba, Azure Files) without mounting it.
//
// The password ends up in agent status, flares, inventory metadata and logs if anything prints
// or serializes it, and the agent status output is not scrubbed. Every way of printing or
// serializing an SMBConfig (fmt verbs, JSON, YAML) therefore redacts it; callers that need the
// credential read the Password field directly.
type SMBConfig struct {
	// Host is the server hostname or IP address, e.g. myacct.file.core.windows.net.
	Host string `mapstructure:"host" json:"host,omitempty" yaml:"host"`
	// Share is the share name, without directories.
	Share string `mapstructure:"share" json:"share,omitempty" yaml:"share"`
	// Username is the NTLM user. For Azure Files, it is the storage account name.
	Username string `mapstructure:"username" json:"username,omitempty" yaml:"username"`
	// Password is the NTLM password (for Azure Files, the storage account key). Use an ENC[]
	// handle so the secret backend resolves it.
	Password string `mapstructure:"password" json:"password,omitempty" yaml:"password"`
	// Domain is the optional NTLM domain.
	Domain string `mapstructure:"domain" json:"domain,omitempty" yaml:"domain"`
	// Port is the server port. 0 means the SMB default, 445; the launcher applies it.
	Port int `mapstructure:"port" json:"port,omitempty" yaml:"port"`
	// PollInterval is the delay between two scans of the share, in seconds. 0 means the
	// launcher default, 1 second; other values must be between SMBMinPollInterval and
	// SMBMaxPollInterval.
	PollInterval float64 `mapstructure:"poll_interval" json:"poll_interval,omitempty" yaml:"poll_interval"`
	// AllowSMB2 lets the Agent negotiate SMB 2.0.2 and 2.1 for servers that support nothing newer
	// (Windows Server 2008 R2 and older, old NAS appliances). The default is false: the Agent
	// offers SMB 3.0, 3.0.2 and 3.1.1 only, so that an attacker who alters the unauthenticated
	// negotiation cannot force a session that cannot be encrypted. It cannot be combined with
	// RequireEncryption.
	AllowSMB2 bool `mapstructure:"allow_smb2" json:"allow_smb2,omitempty" yaml:"allow_smb2"`
	// RequireEncryption makes the Agent refuse to read from a server that does not encrypt the
	// session or the share. The default is false: the Agent signs every message, which protects
	// the content from tampering, but a server that does not enforce encryption sends the log
	// lines in the clear. Enable it when the lines can hold secrets or personal data and the
	// path to the server is not trusted.
	RequireEncryption bool `mapstructure:"require_encryption" json:"require_encryption,omitempty" yaml:"require_encryption"`
}

// smbConfigFields has the fields and tags of SMBConfig but none of its methods, so
// marshalling it does not recurse into the redacting marshalers below.
type smbConfigFields SMBConfig

func (c SMBConfig) redacted() smbConfigFields {
	fields := smbConfigFields(c)
	if fields.Password != "" {
		fields.Password = redactedSecret
	}
	return fields
}

// String implements fmt.Stringer. The password is redacted.
func (c SMBConfig) String() string {
	r := c.redacted()
	return fmt.Sprintf("{Host: %q, Share: %q, Username: %q, Password: %q, Domain: %q, Port: %d, PollInterval: %v, AllowSMB2: %t, RequireEncryption: %t}",
		r.Host, r.Share, r.Username, r.Password, r.Domain, r.Port, r.PollInterval, r.AllowSMB2, r.RequireEncryption)
}

// GoString implements fmt.GoStringer, used by %#v (the logs config parser prints parsed
// configs with it). The password is redacted.
func (c SMBConfig) GoString() string {
	return "config.SMBConfig" + c.String()
}

// Format implements fmt.Formatter so that every verb, not only %v, %s and %#v, prints the
// redacted form.
func (c SMBConfig) Format(f fmt.State, verb rune) {
	if verb == 'v' && f.Flag('#') {
		_, _ = io.WriteString(f, c.GoString())
		return
	}
	_, _ = io.WriteString(f, c.String())
}

// MarshalJSON implements json.Marshaler. The password is redacted, so PublicJSON (sent to the
// backend as inventory metadata) can embed the whole block.
func (c SMBConfig) MarshalJSON() ([]byte, error) {
	return json.Marshal(c.redacted())
}

// MarshalYAML implements yaml.Marshaler. The password is redacted.
func (c SMBConfig) MarshalYAML() (interface{}, error) {
	return c.redacted(), nil
}

// validateSMB checks the smb block. Its errors are shown in agent status and sent as inventory
// metadata, so they never include the password, and never include the host either (a
// misconfigured host can carry user info).
func (c *LogsConfig) validateSMB() error {
	if c.Type != SMBType {
		if c.SMB != nil {
			return fmt.Errorf("smb configuration is only supported for %s sources, got %s", SMBType, c.Type)
		}
		return nil
	}
	s := c.SMB
	if s == nil {
		return errors.New("smb source must have an smb block with host, share and username")
	}

	switch {
	case s.Host == "":
		return errors.New("smb source must have a host")
	case strings.ContainsAny(s.Host, "/\\@[] \t") || strings.Count(s.Host, ":") == 1:
		return errors.New("smb host must be a bare hostname or IP address: no scheme, user info, path or port (use the port setting)")
	case s.Share == "":
		return errors.New("smb source must have a share")
	case strings.ContainsAny(s.Share, "/\\"):
		return errors.New("smb share must be a share name only; put directories in path")
	case s.Username == "":
		return errors.New("smb source must have a username")
	case scrubber.IsEnc(s.Password):
		return errors.New("smb password is an unresolved ENC[] secret handle; configure the secret backend (secret_backend_command) and check `agent secret`")
	case s.Port < 0 || s.Port > math.MaxUint16:
		return fmt.Errorf("smb port must be between 0 and %d (0 means the default, 445), got %d", math.MaxUint16, s.Port)
	case s.AllowSMB2 && s.RequireEncryption:
		return errors.New("smb require_encryption needs SMB 3 and cannot be combined with allow_smb2")
	case s.PollInterval != 0 && (math.IsNaN(s.PollInterval) || s.PollInterval < SMBMinPollInterval.Seconds() || s.PollInterval > SMBMaxPollInterval.Seconds()):
		return fmt.Errorf("smb poll_interval must be 0 (the default, 1 second) or between %v and %v seconds, got %v",
			SMBMinPollInterval.Seconds(), SMBMaxPollInterval.Seconds(), s.PollInterval)
	}

	if c.Path == "" {
		return errors.New("smb source must have a path")
	}
	if err := validateSMBPattern("path", c.Path); err != nil {
		return err
	}
	for _, exclude := range c.ExcludePaths {
		if err := validateSMBPattern("exclude_paths entry", exclude); err != nil {
			return err
		}
	}

	switch c.TailingMode {
	case "", "beginning", "end":
		return nil
	default:
		return fmt.Errorf("invalid start_position %q for smb path %q (supported: beginning, end)", c.TailingMode, c.Path)
	}
}

// validateSMBPattern checks a glob pattern of an smb source, its path or an exclude_paths entry:
// relative to the share root, with '/' separators, and without the '..' elements and NUL bytes
// that the SMB client refuses so that no path can leave the share.
func validateSMBPattern(setting, pattern string) error {
	switch {
	case strings.HasPrefix(pattern, "/"):
		return fmt.Errorf("smb %s %q must be relative to the share root", setting, pattern)
	case strings.Contains(pattern, "\\"):
		return fmt.Errorf("smb %s %q must use '/' separators", setting, pattern)
	case strings.ContainsRune(pattern, 0):
		return fmt.Errorf("smb %s %q must not contain NUL bytes", setting, pattern)
	case slices.Contains(strings.Split(pattern, "/"), ".."):
		return fmt.Errorf("smb %s %q must not contain '..' elements", setting, pattern)
	}
	if _, err := path.Match(pattern, ""); err != nil {
		return fmt.Errorf("smb %s %q is not a valid glob pattern: %v", setting, pattern, err)
	}
	return nil
}
