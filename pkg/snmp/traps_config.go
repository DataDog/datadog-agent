package snmp

import (
	"errors"
	"fmt"
	"time"

	"github.com/soniah/gosnmp"
)

// TrapConfig contains configuration for SNMP traps listeners.
type TrapConfig struct {
	BindHost        string
	Port            uint16
	Version         string
	Timeout         int
	Retries         int
	Community       string
	User            string
	AuthKey         string
	AuthProtocol    string
	PrivKey         string
	PrivProtocol    string
	ContextEngineID string
	ContextName     string
}

// BuildParams returns a valid GoSNMP struct to start making queries
func (c *TrapConfig) BuildParams() (*gosnmp.GoSNMP, error) {
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
		return nil, fmt.Errorf("SNMP version not supported: %s", c.Version)
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
		SecurityModel:   gosnmp.UserSecurityModel,
		MsgFlags:        msgFlags,
		ContextEngineID: c.ContextEngineID,
		ContextName:     c.ContextName,
		SecurityParameters: &gosnmp.UsmSecurityParameters{
			UserName:                 c.User,
			AuthenticationProtocol:   authProtocol,
			AuthenticationPassphrase: c.AuthKey,
			PrivacyProtocol:          privProtocol,
			PrivacyPassphrase:        c.PrivKey,
		},
	}, nil
}
