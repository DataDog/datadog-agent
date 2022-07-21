// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package app

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/gosnmp/gosnmp"
	"github.com/spf13/cobra"
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
	defaultContext       = "public"
	defaultSecurityLevel = ""

	// general communication options
	defaultTimeout = 10 // Timeout better suited to walking
	defaultRetries = 3
)

var (
	// general
	snmpVersion string
	address     string
	deviceIP    string
	oid         string
	port        uint16
	value       uint64
	setVersion  gosnmp.SnmpVersion
	snmp        gosnmp.GoSNMP

	// v1 & v2c
	communityString string

	// v3
	user          string
	authProtocol  gosnmp.SnmpV3AuthProtocol
	authProt      string
	authKey       string
	privProtocol  gosnmp.SnmpV3PrivProtocol
	privProt      string
	privKey       string
	snmpContext   string
	msgFlags      gosnmp.SnmpV3MsgFlags
	securityLevel string

	// communication
	retries int
	timeout int
)

var (
	snmpCmd = &cobra.Command{
		Use:   "snmp",
		Short: "Snmp tools",
		Long:  ``,
	}
)

func init() {

	snmpWalkCmd.Flags().StringVarP(&snmpVersion, "snmp-version", "v", defaultVersion, "Specify SNMP version to use")

	// snmp v1 or v2c specific
	snmpWalkCmd.Flags().StringVarP(&communityString, "community-string", "C", "", "Set the community string")

	// snmp v3 specific
	snmpWalkCmd.Flags().StringVarP(&authProt, "auth-protocol", "a", defaultAuthProtocol, "Set authentication protocol (MD5|SHA|SHA-224|SHA-256|SHA-384|SHA-512)")
	snmpWalkCmd.Flags().StringVarP(&authKey, "auth-key", "A", defaultAuthKey, "Set authentication protocol pass phrase")
	snmpWalkCmd.Flags().StringVarP(&securityLevel, "security-level", "l", defaultSecurityLevel, "set security level (noAuthNoPriv|authNoPriv|authPriv)")
	snmpWalkCmd.Flags().StringVarP(&snmpContext, "context", "N", defaultContext, "Set context name")
	snmpWalkCmd.Flags().StringVarP(&user, "user-name", "u", defaultUserName, "Set security name")
	snmpWalkCmd.Flags().StringVarP(&privProt, "priv-protocol", "x", defaultPrivProtocol, "Set privacy protocol (DES|AES|AES192|AES192C|AES256|AES256C)")
	snmpWalkCmd.Flags().StringVarP(&privKey, "priv-key", "X", defaultPrivKey, "Set privacy protocol pass phrase")

	// general communication options
	snmpWalkCmd.Flags().IntVarP(&retries, "retries", "r", defaultRetries, "Set the number of retries")
	snmpWalkCmd.Flags().IntVarP(&timeout, "timeout", "t", defaultTimeout, "Set the request timeout (in seconds)")

	snmpWalkCmd.SetArgs([]string{})

	// attach snmpWalk to snmp command
	snmpCmd.AddCommand(snmpWalkCmd)

	// attach the command to the root
	AgentCmd.AddCommand(snmpCmd)
}

var snmpWalkCmd = &cobra.Command{
	Use:   "walk <IP Address>[:Port] [OID] [OPTIONS]",
	Short: "Perform a snmpwalk",
	Long:  ``,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Get args
		if len(args) == 0 {
			fmt.Print("Missing argument: IP address\n")
			cmd.Help() //nolint:errcheck
			os.Exit(1)
			return nil
		} else if len(args) == 1 {
			address = args[0]
			oid = defaultOID
		} else if len(args) == 2 {
			address = args[0]
			oid = args[1]
		} else {
			fmt.Printf("The number of arguments must be between 1 and 2. %d arguments were given.\n", len(args))
			cmd.Help() //nolint:errcheck
			os.Exit(1)
			return nil
		}
		if strings.Contains(address, ":") {
			deviceIP = address[:strings.Index(address, ":")]
			value, _ = strconv.ParseUint(address[strings.Index(address, ":")+1:], 0, 16)
			port = uint16(value)
			if port == 0 {
				port = defaultPort
			}
		} else {
			deviceIP = address
			port = defaultPort
		}

		// Communication options check
		if timeout == 0 {
			fmt.Printf("Timeout can not be 0 \n")
			cmd.Help() //nolint:errcheck
			os.Exit(1)
			return nil
		}

		// Authentication check
		if communityString == "" && user == "" {
			// Set default community string if version 1 or 2c and no given community string
			if snmpVersion == "1" || snmpVersion == "2c" {
				communityString = defaultCommunityString
			} else {
				fmt.Printf("No authentication mechanism specified \n")
				cmd.Help() //nolint:errcheck
				os.Exit(1)
				return nil
			}
		}

		// Set the snmp version
		if snmpVersion == "1" {
			setVersion = gosnmp.Version1
		} else if snmpVersion == "2c" || (snmpVersion == "" && communityString != "") {
			setVersion = gosnmp.Version2c
		} else if snmpVersion == "3" || (snmpVersion == "" && user != "") {
			setVersion = gosnmp.Version3
		} else {
			fmt.Printf("SNMP version not supported: %s, using default version 2c. \n", snmpVersion) // match default version of the core check
			setVersion = gosnmp.Version2c
		}

		// Set v3 security parameters
		if setVersion == gosnmp.Version3 {
			// Authentication Protocol
			switch strings.ToLower(authProt) {
			case "":
				authProtocol = gosnmp.NoAuth
			case "md5":
				authProtocol = gosnmp.MD5
			case "sha":
				authProtocol = gosnmp.SHA
			case "sha224":
				authProtocol = gosnmp.SHA224
			case "sha256":
				authProtocol = gosnmp.SHA256
			case "sha384":
				authProtocol = gosnmp.SHA384
			case "sha512":
				authProtocol = gosnmp.SHA512
			default:
				fmt.Printf("Unsupported authentication protocol: %s \n", authProt)
				cmd.Help() //nolint:errcheck
				os.Exit(1)
				return nil
			}

			// Privacy Protocol
			switch strings.ToLower(privProt) {
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
				fmt.Printf("Unsupported privacy protocol: %s \n", privProt)
				cmd.Help() //nolint:errcheck
				os.Exit(1)
				return nil
			}

			// MsgFlags
			switch strings.ToLower(securityLevel) {
			case "":
				msgFlags = gosnmp.NoAuthNoPriv
				if privKey != "" {
					msgFlags = gosnmp.AuthPriv
				} else if authKey != "" {
					msgFlags = gosnmp.AuthNoPriv
				}

			case "noauthnopriv":
				msgFlags = gosnmp.NoAuthNoPriv
			case "authpriv":
				msgFlags = gosnmp.AuthPriv
			case "authnopriv":
				msgFlags = gosnmp.AuthNoPriv
			default:
				fmt.Printf("Unsupported security level: %s \n", securityLevel)
				cmd.Help() //nolint:errcheck
				os.Exit(1)
				return nil
			}
		}
		// Set SNMP parameters
		snmp = gosnmp.GoSNMP{
			Target:    deviceIP,
			Port:      port,
			Community: communityString,
			Transport: "udp",
			Version:   setVersion,
			Timeout:   time.Duration(timeout * int(time.Second)),
			Retries:   retries,
			// v3
			SecurityModel: gosnmp.UserSecurityModel,
			ContextName:   snmpContext,
			MsgFlags:      msgFlags,
			SecurityParameters: &gosnmp.UsmSecurityParameters{
				UserName:                 user,
				AuthenticationProtocol:   authProtocol,
				AuthenticationPassphrase: authKey,
				PrivacyProtocol:          privProtocol,
				PrivacyPassphrase:        privKey,
			},
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
	},
}

func printValue(pdu gosnmp.SnmpPDU) error {

	fmt.Printf("%s = ", pdu.Name)

	switch pdu.Type {
	case gosnmp.OctetString:
		b := pdu.Value.([]byte)
		fmt.Printf("STRING: %s\n", string(b))
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
