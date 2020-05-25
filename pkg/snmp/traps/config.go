package traps

import (
	"errors"
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/soniah/gosnmp"
)

// TrapListenerConfig contains configuration for an SNMP trap listener.
type TrapListenerConfig struct {
	Port         uint16 `mapstructure:"port"`
	Version      string `mapstructure:"version"`
	Community    string `mapstructure:"community"`
	User         string `mapstructure:"user"`
	AuthKey      string `mapstructure:"auth_key"`
	AuthProtocol string `mapstructure:"auth_protocol"`
	PrivKey      string `mapstructure:"priv_key"`
	PrivProtocol string `mapstructure:"priv_protocol"`
}

// GoSNMP logger interface implementation.
type trapLogger struct{}

func (x *trapLogger) Print(v ...interface{}) {
	log.Debug(v...)
}

func (x *trapLogger) Printf(format string, v ...interface{}) {
	log.Debugf(format, v...)
}

// BuildVersion returns the GoSNMP version value for a listener configuration.
func (c *TrapListenerConfig) BuildVersion() (gosnmp.SnmpVersion, error) {
	switch c.Version {
	case "1":
		return gosnmp.Version1, nil
	case "2", "2c":
		return gosnmp.Version2c, nil
	case "3":
		return gosnmp.Version3, nil
	case "":
		if c.Community != "" {
			return gosnmp.Version2c, nil
		}
		if c.User != "" {
			return gosnmp.Version3, nil
		}
		return 0, errors.New("One of `community` or `user` is required")
	default:
		return 0, fmt.Errorf("Unsupported version: %s (possible values are '1', '2c' and '3')", c.Version)
	}
}

// BuildAuthProtocol returns the GoSNMP authentication protocol value for a listener configuration.
func (c *TrapListenerConfig) BuildAuthProtocol() (gosnmp.SnmpV3AuthProtocol, error) {
	switch c.AuthProtocol {
	case "", "SHA":
		return gosnmp.SHA, nil
	case "MD5":
		return gosnmp.MD5, nil
	default:
		return 0, fmt.Errorf("Unsupported authentication protocol: %s (possible values are 'MD5' and 'SHA')", c.AuthProtocol)
	}
}

// BuildPrivProtocol returns the GoSNMP privacy protocol value for a listener configuration.
func (c *TrapListenerConfig) BuildPrivProtocol() (gosnmp.SnmpV3PrivProtocol, error) {
	switch c.PrivProtocol {
	case "", "DES":
		return gosnmp.DES, nil
	case "AES":
		return gosnmp.AES, nil
	default:
		return 0, fmt.Errorf("Unsupported privacy protocol: %s (possible values are 'DES' and 'AES')", c.PrivProtocol)
	}
}

// BuildMsgFlags returns the GoSNMP message flags value for a listener configuration.
func (c *TrapListenerConfig) BuildMsgFlags() (gosnmp.SnmpV3MsgFlags, error) {
	if c.PrivKey != "" {
		if c.AuthKey == "" {
			return 0, errors.New("`auth_key` is required when `priv_key` is set")
		}
		return gosnmp.AuthPriv, nil
	}
	if c.AuthKey != "" {
		return gosnmp.AuthNoPriv, nil
	}
	return gosnmp.NoAuthNoPriv, nil
}

// BuildParams returns a valid GoSNMP params structure from a listener configuration.
func (c *TrapListenerConfig) BuildParams() (*gosnmp.GoSNMP, error) {
	port := c.Port
	if port == 0 {
		port = 162
	}

	if c.Community == "" && c.User == "" {
		return nil, errors.New("One of `community` or `user` must be specified")
	}

	version, err := c.BuildVersion()
	if err != nil {
		return nil, err
	}

	/*
		FIXME: Depending on the auth/privacy protocol in use, there is a minimum length requirement on the passphrases (see https://tools.ietf.org/html/rfc2264#section-2.1).
		E.g. AES recommends 12+ characters (see https://www.ietf.org/rfc/rfc3826.txt).
		Besides, the SNMP protocol generally requires at least 8+ characters (see https://tools.ietf.org/html/rfc3414#section-11.2).
		We probably want to validate these constraints, otherwise hard-to-debug behaviors might happen.
	*/

	authProtocol, err := c.BuildAuthProtocol()
	if err != nil {
		return nil, err
	}

	privProtocol, err := c.BuildPrivProtocol()
	if err != nil {
		return nil, err
	}

	msgFlags, err := c.BuildMsgFlags()
	if err != nil {
		return nil, err
	}

	logger := &trapLogger{}

	securityParams := &gosnmp.UsmSecurityParameters{
		UserName:                 c.User,
		AuthenticationProtocol:   authProtocol,
		AuthenticationPassphrase: c.AuthKey,
		PrivacyProtocol:          privProtocol,
		PrivacyPassphrase:        c.PrivKey,
		// NOTE: passing a logger here is critical, otherwise GoSNMP panics upon receiving a v3 trap due to a bug.
		Logger: logger,
	}

	params := &gosnmp.GoSNMP{
		Port:               port,
		Community:          c.Community,
		Transport:          "udp",
		Version:            version,
		MsgFlags:           msgFlags,
		SecurityModel:      gosnmp.UserSecurityModel,
		SecurityParameters: securityParams,
		Logger:             logger,
	}

	return params, nil
}
