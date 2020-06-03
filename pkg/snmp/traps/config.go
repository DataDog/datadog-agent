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
// YAML field tags provided for test marshalling purposes.
type TrapListenerConfig struct {
	Port         uint16 `mapstructure:"port" yaml:"port"`
	Version      string `mapstructure:"version" yaml:"version"`
	Community    string `mapstructure:"community" yaml:"community"`
	User         string `mapstructure:"user" yaml:"user"`
	AuthKey      string `mapstructure:"authentication_key" yaml:"authentication_key"`
	AuthProtocol string `mapstructure:"authentication_protocol" yaml:"authentication_protocol"`
	PrivKey      string `mapstructure:"privacy_key" yaml:"privacy_key"`
	PrivProtocol string `mapstructure:"privacy_protocol" yaml:"privacy_protocol"`
}

// trapLogger is a GoSNMP logger interface implementation.
type trapLogger struct {
	gosnmp.Logger
}

func (x *trapLogger) Print(v ...interface{}) {
	log.Debug(v...)
}

func (x *trapLogger) Printf(format string, v ...interface{}) {
	log.Debugf(format, v...)
}

// BuildVersion returns a GoSNMP version value from a string value.
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

// BuildAuthProtocol returns a GoSNMP authentication protocol value from a string value.
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

// BuildPrivProtocol returns a GoSNMP privacy protocol value from a string value.
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

func hasAuth(authProtocol string, authKey string) bool {
	return authProtocol != "" && authKey != ""
}

func hasPriv(privProtocol string, privKey string) bool {
	return privProtocol != "" && privKey != ""
}

// BuildMsgFlags returns a GoSNMP message flags value.
func BuildMsgFlags(authProtocol string, authKey string, privProtocol string, privKey string) gosnmp.SnmpV3MsgFlags {
	hasAuth := hasAuth(authProtocol, authKey)
	hasPriv := hasPriv(privProtocol, privKey)

	if hasAuth {
		if hasPriv {
			return gosnmp.AuthPriv
		}
		return gosnmp.AuthNoPriv
	}

	return gosnmp.NoAuthNoPriv
}

// BuildSecurityParams returns a GoSNMP user security parameters value
func BuildSecurityParams(user string, authProtocol string, authKey string, privProtocol string, privKey string) (*gosnmp.UsmSecurityParameters, error) {
	// Validation here is cumbersome, but necessary for good UX - passing inconsistent values to GoSNMP
	// would almost certainly result in panics with hard-to-understand errors.

	if user == "" {
		return nil, errors.New("`user` is required when using SNMPv3")
	}

	sp := &gosnmp.UsmSecurityParameters{
		UserName: user,
	}

	if authProtocol != "" && authKey == "" {
		return nil, errors.New("`authentication_key` is required when `authentication_protocol` is set")
	}

	if authKey != "" && authProtocol == "" {
		return nil, errors.New("`authentication_protocol` is required when `authentication_key` is set")
	}

	if privProtocol != "" && privKey == "" {
		return nil, errors.New("`privacy_key` is required when `privacy_protocol` is set")
	}

	if privKey != "" && privProtocol == "" {
		return nil, errors.New("`privacy_protocol` is required when `privacy_key` is set")
	}

	hasAuth := hasAuth(authProtocol, authKey)
	hasPriv := hasPriv(privProtocol, privKey)

	if hasPriv && !hasAuth {
		return nil, errors.New("Authentication is required when privacy is enabled")
	}

	if hasAuth {
		authProtocolValue, err := BuildAuthProtocol(authProtocol)
		if err != nil {
			return nil, err
		}
		sp.AuthenticationProtocol = authProtocolValue
		sp.AuthenticationPassphrase = authKey
	}

	if hasPriv {
		privProtocolValue, err := BuildPrivProtocol(privProtocol)
		if err != nil {
			return nil, err
		}
		sp.PrivacyProtocol = privProtocolValue
		sp.PrivacyPassphrase = privKey
	}

	return sp, nil
}

// BuildParams returns a valid GoSNMP params structure from a listener configuration.
func (c *TrapListenerConfig) BuildParams() (*gosnmp.GoSNMP, error) {
	if c.Port == 0 {
		return nil, errors.New("`port` is required")
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

	logger := &trapLogger{}

	params := &gosnmp.GoSNMP{
		Port:      c.Port,
		Transport: "udp",
		Version:   version,
		Logger:    logger,
	}

	if version == gosnmp.Version1 || version == gosnmp.Version2c {
		if c.Community == "" {
			return nil, errors.New("`community` is required when using SNMPv1 or SNMPv2c")
		}
		params.Community = c.Community
	}

	if version == gosnmp.Version3 {
		sp, err := BuildSecurityParams(c.User, c.AuthProtocol, c.AuthKey, c.PrivProtocol, c.PrivKey)
		if err != nil {
			return nil, err
		}
		sp.Logger = logger

		msgFlags := BuildMsgFlags(c.AuthProtocol, c.AuthKey, c.PrivProtocol, c.PrivKey)

		params.SecurityModel = gosnmp.UserSecurityModel
		params.SecurityParameters = sp
		params.MsgFlags = msgFlags
	}

	return params, nil
}
