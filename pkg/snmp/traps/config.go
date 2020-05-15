package traps

import (
	"errors"
	"fmt"

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
	PrivKey      string `mapstructure:"privacy_key"`
	PrivProtocol string `mapstructure:"privacy_protocol"`
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
	if c.Version == "1" {
		version = gosnmp.Version1
	} else if c.Version == "2c" || (c.Version == "" && c.Community != "") {
		version = gosnmp.Version2c
	} else if c.Version == "3" || (c.Version == "" && c.User != "") {
		version = gosnmp.Version3
	} else {
		return nil, fmt.Errorf("Unsupported version: %s (possible values are '1', '2c' and '3')", c.Version)
	}

	var authProtocol gosnmp.SnmpV3AuthProtocol
	if c.AuthProtocol == "MD5" {
		authProtocol = gosnmp.MD5
	} else if c.AuthProtocol == "SHA" {
		authProtocol = gosnmp.SHA
	} else if c.AuthProtocol == "" {
		authProtocol = gosnmp.SHA
	} else {
		return nil, fmt.Errorf("Unsupported authentication protocol: %s (possible values are 'MD5' and 'SHA')", c.AuthProtocol)
	}

	var privProtocol gosnmp.SnmpV3PrivProtocol
	if c.PrivProtocol == "DES" {
		privProtocol = gosnmp.DES
	} else if c.PrivProtocol == "AES" {
		privProtocol = gosnmp.AES
	} else if c.PrivProtocol == "" {
		privProtocol = gosnmp.AES
	} else {
		return nil, fmt.Errorf("Unsupported privacy protocol: %s (possible values are 'DES' and 'AES')", c.PrivProtocol)
	}

	msgFlags := gosnmp.NoAuthNoPriv
	if c.PrivKey != "" {
		if c.AuthKey == "" {
			return nil, errors.New("`auth_key` is required when `priv_key` is set")
		}
		msgFlags = gosnmp.AuthPriv
	} else if c.AuthKey != "" {
		msgFlags = gosnmp.AuthNoPriv
	}

	securityParams := &gosnmp.UsmSecurityParameters{
		UserName:                 c.User,
		AuthenticationProtocol:   authProtocol,
		AuthenticationPassphrase: c.AuthKey,
		PrivacyProtocol:          privProtocol,
		PrivacyPassphrase:        c.PrivKey,
	}

	params := &gosnmp.GoSNMP{
		Port:               port,
		Community:          c.Community,
		Transport:          "udp",
		Version:            version,
		MsgFlags:           msgFlags,
		SecurityModel:      gosnmp.UserSecurityModel,
		SecurityParameters: securityParams,
	}

	return params, nil
}
