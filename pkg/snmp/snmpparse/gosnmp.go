// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package snmpparse

import (
	"fmt"
	"github.com/DataDog/datadog-agent/comp/core/log/def"
	"github.com/DataDog/datadog-agent/comp/snmptraps/snmplog"
	"github.com/gosnmp/gosnmp"
	"time"
)

// NewSNMP validates an SNMPConfig and builds a GoSNMP from it.
func NewSNMP(conf *SNMPConfig, logger log.Component) (*gosnmp.GoSNMP, error) {
	// Communication options check
	if conf.Timeout == 0 {
		return nil, fmt.Errorf("timeout cannot be 0")
	}
	var version gosnmp.SnmpVersion
	var ok bool
	if conf.Version == "" {
		// Assume v3 if a username was set, otherwise assume v2c.
		if conf.Username != "" {
			version = gosnmp.Version3
		} else {
			version = gosnmp.Version2c
		}
	} else if version, ok = VersionOpts.GetVal(conf.Version); !ok {
		return nil, fmt.Errorf("SNMP version %q not supported; must be %s", conf.Version, VersionOpts.OptsStr())
	}

	// Set default community string if version 1 or 2c and no given community string
	if version != gosnmp.Version3 && conf.CommunityString == "" {
		conf.CommunityString = DefaultCommunityString
	}

	// Authentication check
	if version == gosnmp.Version3 && conf.Username == "" {
		return nil, fmt.Errorf("username is required for snmp v3")
	}

	port := conf.Port
	if port == 0 {
		port = DefaultPort
	}

	securityParams := &gosnmp.UsmSecurityParameters{}
	var msgFlags gosnmp.SnmpV3MsgFlags
	// Set v3 security parameters
	if version == gosnmp.Version3 {
		securityParams.UserName = conf.Username
		securityParams.AuthenticationPassphrase = conf.AuthKey
		securityParams.PrivacyPassphrase = conf.PrivKey

		if securityParams.AuthenticationProtocol, ok = AuthOpts.GetVal(conf.AuthProtocol); !ok {
			return nil, fmt.Errorf("authentication protocol %q not supported; must be %s", conf.AuthProtocol, AuthOpts.OptsStr())
		}

		if securityParams.PrivacyProtocol, ok = PrivOpts.GetVal(conf.PrivProtocol); !ok {
			return nil, fmt.Errorf("privacy protocol %q not supported; must be %s", conf.PrivProtocol, PrivOpts.OptsStr())
		}

		if conf.SecurityLevel == "" {
			msgFlags = gosnmp.NoAuthNoPriv
			if conf.PrivKey != "" {
				msgFlags = gosnmp.AuthPriv
			} else if conf.AuthKey != "" {
				msgFlags = gosnmp.AuthNoPriv
			}
		} else {
			var ok bool // can't use := below because it'll make a new msgFlags instead of setting the one in the parent scope.
			if msgFlags, ok = LevelOpts.GetVal(conf.SecurityLevel); !ok {
				return nil, fmt.Errorf("security level %q not supported; must be %s", conf.SecurityLevel, LevelOpts.OptsStr())
			}
		}
	}
	// Set SNMP parameters
	return &gosnmp.GoSNMP{
		Target:                  conf.IPAddress,
		Port:                    port,
		Community:               conf.CommunityString,
		Transport:               "udp",
		Version:                 version,
		Timeout:                 time.Duration(conf.Timeout * int(time.Second)),
		Retries:                 conf.Retries,
		SecurityModel:           gosnmp.UserSecurityModel,
		ContextName:             conf.Context,
		MsgFlags:                msgFlags,
		SecurityParameters:      securityParams,
		UseUnconnectedUDPSocket: conf.UseUnconnectedUDPSocket,
		Logger:                  gosnmp.NewLogger(snmplog.New(logger)),
	}, nil
}
