// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package snmp implements 'agent snmp'.
package snmp

import (
	"encoding/hex"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/gosnmp/gosnmp"
	"github.com/spf13/cobra"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/cmd/agent/command"
	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/log"
	utilFunc "github.com/DataDog/datadog-agent/pkg/snmp/gosnmplib"
	parse "github.com/DataDog/datadog-agent/pkg/snmp/snmpparse"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

const (
	defaultVersion = ""
	defaultOID     = ""
	defaultPort    = 161

	// snmp v1 & v2c
	defaultCommunityString = "public"

	// snmp v3
	defaultUserName      = ""
	defaultAuthProtocol  = ""
	defaultAuthKey       = ""
	defaultPrivProtocol  = ""
	defaultPrivKey       = ""
	defaultContext       = ""
	defaultSecurityLevel = ""

	// general communication options
	defaultTimeout                 = 10 // Timeout better suited to walking
	defaultRetries                 = 3
	defaultUseUnconnectedUDPSocket = false
)

// cliParams are the command-line arguments for this subcommand
type cliParams struct {
	*command.GlobalParams

	// args are the positional command-line arguments
	args []string

	// cmd is the cobra Command, used to show help
	cmd *cobra.Command

	// general
	snmpVersion string

	// v1 & v2c
	communityString string

	// v3
	user          string
	authProt      string
	authKey       string
	privProt      string
	privKey       string
	snmpContext   string
	securityLevel string

	// communication
	retries              int
	timeout              int
	unconnectedUDPSocket bool
}

// Commands returns a slice of subcommands for the 'agent' command.
func Commands(globalParams *command.GlobalParams) []*cobra.Command {
	cliParams := &cliParams{
		GlobalParams: globalParams,
	}
	snmpWalkCmd := &cobra.Command{
		Use:   "walk <IP Address>[:Port] [OID] [OPTIONS]",
		Short: "Perform a snmpwalk, if only a valid IP address is provided with the oid then the agent default snmp config will be used",
		Long:  ``,
		RunE: func(cmd *cobra.Command, args []string) error {
			cliParams.args = args
			cliParams.cmd = cmd
			return fxutil.OneShot(snmpwalk,
				fx.Supply(cliParams),
				fx.Supply(core.BundleParams{
					ConfigParams: config.NewAgentParamsWithSecrets(globalParams.ConfFilePath),
					LogParams:    log.LogForOneShot(command.LoggerName, "off", true)}),
				core.Bundle,
			)
		},
	}

	snmpWalkCmd.Flags().StringVarP(&cliParams.snmpVersion, "snmp-version", "v", defaultVersion, "Specify SNMP version to use")

	// snmp v1 or v2c specific
	snmpWalkCmd.Flags().StringVarP(&cliParams.communityString, "community-string", "C", "", "Set the community string")

	// snmp v3 specific
	snmpWalkCmd.Flags().StringVarP(&cliParams.authProt, "auth-protocol", "a", defaultAuthProtocol, "Set authentication protocol (MD5|SHA|SHA-224|SHA-256|SHA-384|SHA-512)")
	snmpWalkCmd.Flags().StringVarP(&cliParams.authKey, "auth-key", "A", defaultAuthKey, "Set authentication protocol pass phrase")
	snmpWalkCmd.Flags().StringVarP(&cliParams.securityLevel, "security-level", "l", defaultSecurityLevel, "set security level (noAuthNoPriv|authNoPriv|authPriv)")
	snmpWalkCmd.Flags().StringVarP(&cliParams.snmpContext, "context", "N", defaultContext, "Set context name")
	snmpWalkCmd.Flags().StringVarP(&cliParams.user, "user-name", "u", defaultUserName, "Set security name")
	snmpWalkCmd.Flags().StringVarP(&cliParams.privProt, "priv-protocol", "x", defaultPrivProtocol, "Set privacy protocol (DES|AES|AES192|AES192C|AES256|AES256C)")
	snmpWalkCmd.Flags().StringVarP(&cliParams.privKey, "priv-key", "X", defaultPrivKey, "Set privacy protocol pass phrase")

	// general communication options
	snmpWalkCmd.Flags().IntVarP(&cliParams.retries, "retries", "r", defaultRetries, "Set the number of retries")
	snmpWalkCmd.Flags().IntVarP(&cliParams.timeout, "timeout", "t", defaultTimeout, "Set the request timeout (in seconds)")
	snmpWalkCmd.Flags().BoolVar(&cliParams.unconnectedUDPSocket, "use-unconnected-udp-socket", defaultUseUnconnectedUDPSocket, "If specified, changes net connection to be unconnected UDP socket")

	snmpWalkCmd.SetArgs([]string{})

	snmpCmd := &cobra.Command{
		Use:   "snmp",
		Short: "Snmp tools",
		Long:  ``,
	}
	snmpCmd.AddCommand(snmpWalkCmd)

	return []*cobra.Command{snmpCmd}
}

func snmpwalk(config config.Component, cliParams *cliParams) error {
	var (
		address      string
		deviceIP     string
		oid          string
		port         uint16
		value        uint64
		setVersion   gosnmp.SnmpVersion
		authProtocol gosnmp.SnmpV3AuthProtocol
		privProtocol gosnmp.SnmpV3PrivProtocol
		msgFlags     gosnmp.SnmpV3MsgFlags
	)
	// Get args
	if len(cliParams.args) == 0 {
		fmt.Print("Missing argument: IP address\n")
		cliParams.cmd.Help() //nolint:errcheck
		os.Exit(1)
		return nil
	} else if len(cliParams.args) == 1 {
		address = cliParams.args[0]
		oid = defaultOID
	} else if len(cliParams.args) == 2 {
		address = cliParams.args[0]
		oid = cliParams.args[1]
	} else {
		fmt.Printf("The number of arguments must be between 1 and 2. %d arguments were given.\n", len(cliParams.args))
		cliParams.cmd.Help() //nolint:errcheck
		os.Exit(1)
		return nil
	}
	if strings.Contains(address, ":") {
		deviceIP = address[:strings.Index(address, ":")]
		value, _ = strconv.ParseUint(address[strings.Index(address, ":")+1:], 0, 16)
		port = uint16(value)
	} else {
		//If the customer provides only 1 argument : the ip_address
		//We check the ip address configuration in the agent runtime and we use it for the snmpwalk
		deviceIP = address
		//Allow the possibility to pass the config file as an argument to the command
		snmpConfigList, err := parse.GetConfigCheckSnmp()
		instance := parse.GetIPConfig(deviceIP, snmpConfigList)
		if err != nil {
			fmt.Printf("Couldn't parse the SNMP config : %v \n", err)
		}
		if instance.IPAddress != "" {
			cliParams.snmpVersion = instance.Version
			port = instance.Port

			// v1 & v2c
			cliParams.communityString = instance.CommunityString

			// v3
			cliParams.user = instance.Username
			cliParams.authProt = instance.AuthProtocol
			cliParams.authKey = instance.AuthKey
			cliParams.privProt = instance.PrivProtocol
			cliParams.privKey = instance.PrivKey
			cliParams.snmpContext = instance.Context

			// communication
			cliParams.retries = instance.Retries
			if instance.Timeout != 0 {
				cliParams.timeout = instance.Timeout
			}
		}
	}
	if port == 0 {
		port = defaultPort
	}

	// Communication options check
	if cliParams.timeout == 0 {
		fmt.Printf("Timeout can not be 0 \n")
		cliParams.cmd.Help() //nolint:errcheck
		os.Exit(1)
		return nil
	}

	// Authentication check
	if cliParams.communityString == "" && cliParams.user == "" {
		// Set default community string if version 1 or 2c and no given community string
		if cliParams.snmpVersion == "1" || cliParams.snmpVersion == "2c" {
			cliParams.communityString = defaultCommunityString
		} else {
			fmt.Printf("No authentication mechanism specified \n")
			cliParams.cmd.Help() //nolint:errcheck
			os.Exit(1)
			return nil
		}
	}

	// Set the snmp version
	if cliParams.snmpVersion == "1" {
		setVersion = gosnmp.Version1
	} else if cliParams.snmpVersion == "2c" || (cliParams.snmpVersion == "" && cliParams.communityString != "") {
		setVersion = gosnmp.Version2c
	} else if cliParams.snmpVersion == "3" || (cliParams.snmpVersion == "" && cliParams.user != "") {
		setVersion = gosnmp.Version3
	} else {
		fmt.Printf("SNMP version not supported: %s, using default version 2c. \n", cliParams.snmpVersion) // match default version of the core check
		setVersion = gosnmp.Version2c
	}

	// Set v3 security parameters
	if setVersion == gosnmp.Version3 {
		// Authentication Protocol
		switch strings.ToLower(cliParams.authProt) {
		case "":
			authProtocol = gosnmp.NoAuth
		case "md5":
			authProtocol = gosnmp.MD5
		case "sha":
			authProtocol = gosnmp.SHA
		case "sha224", "sha-224":
			authProtocol = gosnmp.SHA224
		case "sha256", "sha-256":
			authProtocol = gosnmp.SHA256
		case "sha384", "sha-384":
			authProtocol = gosnmp.SHA384
		case "sha512", "sha-512":
			authProtocol = gosnmp.SHA512
		default:
			fmt.Printf("Unsupported authentication protocol: %s \n", cliParams.authProt)
			cliParams.cmd.Help() //nolint:errcheck
			os.Exit(1)
			return nil
		}

		// Privacy Protocol
		switch strings.ToLower(cliParams.privProt) {
		case "":
			privProtocol = gosnmp.NoPriv
		case "des":
			privProtocol = gosnmp.DES
		case "aes":
			privProtocol = gosnmp.AES
		case "aes192":
			privProtocol = gosnmp.AES192
		case "aes192c":
			privProtocol = gosnmp.AES192C
		case "aes256":
			privProtocol = gosnmp.AES256
		case "aes256c":
			privProtocol = gosnmp.AES256C
		default:
			fmt.Printf("Unsupported privacy protocol: %s \n", cliParams.privProt)
			cliParams.cmd.Help() //nolint:errcheck
			os.Exit(1)
			return nil
		}

		// MsgFlags
		switch strings.ToLower(cliParams.securityLevel) {
		case "":
			msgFlags = gosnmp.NoAuthNoPriv
			if cliParams.privKey != "" {
				msgFlags = gosnmp.AuthPriv
			} else if cliParams.authKey != "" {
				msgFlags = gosnmp.AuthNoPriv
			}

		case "noauthnopriv":
			msgFlags = gosnmp.NoAuthNoPriv
		case "authpriv":
			msgFlags = gosnmp.AuthPriv
		case "authnopriv":
			msgFlags = gosnmp.AuthNoPriv
		default:
			fmt.Printf("Unsupported security level: %s \n", cliParams.securityLevel)
			cliParams.cmd.Help() //nolint:errcheck
			os.Exit(1)
			return nil
		}
	}
	// Set SNMP parameters
	snmp := gosnmp.GoSNMP{
		Target:    deviceIP,
		Port:      port,
		Community: cliParams.communityString,
		Transport: "udp",
		Version:   setVersion,
		Timeout:   time.Duration(cliParams.timeout * int(time.Second)),
		Retries:   cliParams.retries,
		// v3
		SecurityModel: gosnmp.UserSecurityModel,
		ContextName:   cliParams.snmpContext,
		MsgFlags:      msgFlags,
		SecurityParameters: &gosnmp.UsmSecurityParameters{
			UserName:                 cliParams.user,
			AuthenticationProtocol:   authProtocol,
			AuthenticationPassphrase: cliParams.authKey,
			PrivacyProtocol:          privProtocol,
			PrivacyPassphrase:        cliParams.privKey,
		},
		UseUnconnectedUDPSocket: cliParams.unconnectedUDPSocket,
	}
	// Establish connection
	err := snmp.Connect()
	if err != nil {
		fmt.Printf("Connect err: %v\n", err)
		os.Exit(1)
		return nil
	}
	defer snmp.Conn.Close()

	// Perform a snmpwalk using Walk for all versions
	err = snmp.Walk(oid, printValue)
	if err != nil {
		fmt.Printf("Walk Error: %v\n", err)
		os.Exit(1)
	}

	return nil
}

func printValue(pdu gosnmp.SnmpPDU) error {

	fmt.Printf("%s = ", pdu.Name)

	switch pdu.Type {
	case gosnmp.OctetString:
		b := pdu.Value.([]byte)
		if !utilFunc.IsStringPrintable(b) {
			var strBytes []string
			for _, bt := range b {
				strBytes = append(strBytes, strings.ToUpper(hex.EncodeToString([]byte{bt})))
			}
			fmt.Print("Hex-STRING: " + strings.Join(strBytes, " ") + "\n")
		} else {
			fmt.Printf("STRING: %s\n", string(b))
		}
	case gosnmp.ObjectIdentifier:
		fmt.Printf("OID: %s\n", pdu.Value)
	case gosnmp.TimeTicks:
		fmt.Print(pdu.Value, "\n")
	case gosnmp.Counter32:
		fmt.Printf("Counter 32: %d\n", pdu.Value.(uint))
	case gosnmp.Counter64:
		fmt.Printf("Counter 64: %d\n", pdu.Value.(uint64))
	case gosnmp.Integer:
		fmt.Printf("INTEGER: %d\n", pdu.Value.(int))
	case gosnmp.Gauge32:
		fmt.Printf("Gauge 32: %d\n", pdu.Value.(uint))
	case gosnmp.IPAddress:
		fmt.Printf("IpAddress: %s\n", pdu.Value.(string))
	default:
		fmt.Printf("TYPE %d: %d\n", pdu.Type, gosnmp.ToBigInt(pdu.Value))
	}

	return nil
}
