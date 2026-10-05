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
	"strings"

	"github.com/DataDog/datadog-agent/pkg/util/scrubber"
)

// redactedSecret replaces a non-empty credential wherever a config is printed or serialized.
const redactedSecret = "********"

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
	// launcher default, 1 second.
	PollInterval float64 `mapstructure:"poll_interval" json:"poll_interval,omitempty" yaml:"poll_interval"`
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
	return fmt.Sprintf("{Host: %q, Share: %q, Username: %q, Password: %q, Domain: %q, Port: %d, PollInterval: %v}",
		r.Host, r.Share, r.Username, r.Password, r.Domain, r.Port, r.PollInterval)
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
	case s.PollInterval < 0 || math.IsNaN(s.PollInterval) || math.IsInf(s.PollInterval, 0):
		return fmt.Errorf("smb poll_interval must be a finite number of seconds >= 0 (0 means the default, 1), got %v", s.PollInterval)
	}

	switch {
	case c.Path == "":
		return errors.New("smb source must have a path")
	case strings.HasPrefix(c.Path, "/"):
		return fmt.Errorf("smb path %q must be relative to the share root", c.Path)
	case strings.Contains(c.Path, "\\"):
		return fmt.Errorf("smb path %q must use '/' separators", c.Path)
	}
	if _, err := path.Match(c.Path, ""); err != nil {
		return fmt.Errorf("smb path %q is not a valid glob pattern: %v", c.Path, err)
	}

	switch c.TailingMode {
	case "", "beginning", "end":
		return nil
	default:
		return fmt.Errorf("invalid start_position %q for smb path %q (supported: beginning, end)", c.TailingMode, c.Path)
	}
}
