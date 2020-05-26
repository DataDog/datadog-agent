// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2020 Datadog, Inc.

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

// BuildVersion returns the GoSNMP version value from a string value.
func BuildVersion(value string) (gosnmp.SnmpVersion, error) {
	switch value {
	case "1":
		return gosnmp.Version1, nil
	case "2", "2c":
		return gosnmp.Version2c, nil
	case "3":
		return gosnmp.Version3, nil
	default:
		return 0, fmt.Errorf("Unsupported version: '%s' (possible values are '1', '2c' and '3')", value)
	}
}

// InferVersion infers the GoSNMP version value from a community string and username.
func InferVersion(community string, user string) (gosnmp.SnmpVersion, error) {
	if community != "" {
		return gosnmp.Version2c, nil
	}
	if user != "" {
		return gosnmp.Version3, nil
	}
	return 0, errors.New("Could not infer version: `community` and `user` are not set")
}

// BuildAuthProtocol returns the GoSNMP authentication protocol value from a string value.
func BuildAuthProtocol(value string) (gosnmp.SnmpV3AuthProtocol, error) {
	switch value {
	case "SHA":
		return gosnmp.SHA, nil
	case "MD5":
		return gosnmp.MD5, nil
	default:
		return 0, fmt.Errorf("Unsupported authentication protocol: '%s' (possible values are 'MD5' and 'SHA')", value)
	}
}

// BuildPrivProtocol returns the GoSNMP privacy protocol value from a string value.
func BuildPrivProtocol(value string) (gosnmp.SnmpV3PrivProtocol, error) {
	switch value {
	case "DES":
		return gosnmp.DES, nil
	case "AES":
		return gosnmp.AES, nil
	default:
		return 0, fmt.Errorf("Unsupported privacy protocol: '%s' (possible values are 'DES' and 'AES')", value)
	}
}

// BuildMsgFlags returns the GoSNMP message flags value for a listener configuration.
func BuildMsgFlags(authKey string, privKey string) (gosnmp.SnmpV3MsgFlags, error) {
	if privKey != "" {
		if authKey == "" {
			return 0, errors.New("`auth_key` is required when `priv_key` is set")
		}
		return gosnmp.AuthPriv, nil
	}
	if authKey != "" {
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

	var version gosnmp.SnmpVersion
	var err error
	if c.Version == "" {
		version, err = InferVersion(c.Community, c.User)
	} else {
		version, err = BuildVersion(c.Version)
	}
	if err != nil {
		return nil, err
	}

	/*
		FIXME: Depending on the auth/privacy protocol in use, there is a minimum length requirement on the passphrases (see https://tools.ietf.org/html/rfc2264#section-2.1).
		E.g. AES recommends 12+ characters (see https://www.ietf.org/rfc/rfc3826.txt).
		Besides, the SNMP protocol generally requires at least 8+ characters (see https://tools.ietf.org/html/rfc3414#section-11.2).
		We probably want to validate these constraints, otherwise hard-to-debug behaviors might happen.
	*/

	logger := &trapLogger{}

	securityParams := &gosnmp.UsmSecurityParameters{
		UserName: c.User,
		// NOTE: passing a logger here is critical, otherwise GoSNMP panics upon receiving a v3 trap due to a bug.
		Logger: logger,
	}

	if c.AuthProtocol != "" {
		authProtocol, err := BuildAuthProtocol(c.AuthProtocol)
		if err != nil {
			return nil, err
		}
		securityParams.AuthenticationProtocol = authProtocol
		securityParams.AuthenticationPassphrase = c.AuthKey
	}

	if c.PrivProtocol != "" {
		privProtocol, err := BuildPrivProtocol(c.PrivProtocol)
		if err != nil {
			return nil, err
		}
		securityParams.PrivacyProtocol = privProtocol
		securityParams.PrivacyPassphrase = c.PrivKey
	}

	msgFlags, err := BuildMsgFlags(c.AuthKey, c.PrivKey)
	if err != nil {
		return nil, err
	}

	// TODO: only set Community on SNMPv1/v2c, and MsgFlags/SecurityModel/SecurityParameters on SNMPv3.

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
