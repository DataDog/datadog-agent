package traps

import (
	"errors"
	"fmt"
	"time"

	"github.com/soniah/gosnmp"
)

// TrapListenerConfig contains configuration for an SNMP trap listener.
type TrapListenerConfig struct {
	Port            uint16 `mapstructure:"port"`
	Version         string `mapstructure:"version"`
	Timeout         int    `mapstructure:"timeout"`
	Retries         int    `mapstructure:"retries"`
	Community       string `mapstructure:"community"`
	User            string `mapstructure:"user"`
	AuthKey         string `mapstructure:"auth_key"`
	AuthProtocol    string `mapstructure:"auth_protocol"`
	PrivKey         string `mapstructure:"privacy_key"`
	PrivProtocol    string `mapstructure:"privacy_protocol"`
	ContextEngineID string `mapstructure:"context_engine_id"`
	ContextName     string `mapstructure:"context_name"`
}

// BuildParams returns a valid GoSNMP params structure from a listener configuration.
func (c *TrapListenerConfig) BuildParams() (*gosnmp.GoSNMP, error) {
	port := c.Port
	if port == 0 {
		port = 162
	}

	if c.Community == "" && c.User == "" {
		return nil, errors.New("No authentication mechanism specified")
	}

	var version gosnmp.SnmpVersion
	if c.Version == "1" {
		version = gosnmp.Version1
	} else if c.Version == "2" || (c.Version == "" && c.Community != "") {
		version = gosnmp.Version2c
	} else if c.Version == "3" || (c.Version == "" && c.User != "") {
		version = gosnmp.Version3
	} else {
		return nil, fmt.Errorf("Unsupported SNMP version: %s", c.Version)
	}

	var authProtocol gosnmp.SnmpV3AuthProtocol
	if c.AuthProtocol == "MD5" {
		authProtocol = gosnmp.MD5
	} else if c.AuthProtocol == "SHA" {
		authProtocol = gosnmp.SHA
	} else if c.AuthProtocol != "" {
		return nil, fmt.Errorf("Unsupported authentication protocol: %s", c.AuthProtocol)
	}

	var privProtocol gosnmp.SnmpV3PrivProtocol
	if c.PrivProtocol == "DES" {
		privProtocol = gosnmp.DES
	} else if c.PrivProtocol == "AES" {
		privProtocol = gosnmp.AES
	} else if c.PrivProtocol != "" {
		return nil, fmt.Errorf("Unsupported privacy protocol: %s", c.PrivProtocol)
	}

	msgFlags := gosnmp.NoAuthNoPriv
	if c.PrivKey != "" {
		msgFlags = gosnmp.AuthPriv
	} else if c.AuthKey != "" {
		msgFlags = gosnmp.AuthNoPriv
	}

	return &gosnmp.GoSNMP{
		Port:            port,
		Community:       c.Community,
		Transport:       "udp",
		Version:         version,
		Timeout:         time.Duration(c.Timeout) * time.Second,
		Retries:         c.Retries,
		MsgFlags:        msgFlags,
		ContextEngineID: c.ContextEngineID,
		ContextName:     c.ContextName,
		SecurityModel:   gosnmp.UserSecurityModel,
		SecurityParameters: &gosnmp.UsmSecurityParameters{
			UserName:                 c.User,
			AuthenticationProtocol:   authProtocol,
			AuthenticationPassphrase: c.AuthKey,
			PrivacyProtocol:          privProtocol,
			PrivacyPassphrase:        c.PrivKey,
		},
	}, nil
}
