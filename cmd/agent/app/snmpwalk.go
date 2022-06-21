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

	"log"

	"github.com/gosnmp/gosnmp"
	"github.com/spf13/cobra"
)

const (
	defaultVersion = "1"
	defaultOID     = ""
	defaultPort    = 161

	// snmp v1 & v2c
	defaultCommunityString = "public"

	// TODO: snmp v3
	// defaultUserName        = ""
	// defaultAuthProtocol    = ""
	// defaultAuthKey         = ""
	// defaultPrivProtocol    = ""
	// defaultprivKey         = ""

	// general communication options
	defaultTimeout = 5
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

	// TODO: v3
	// user         string
	// authProtocol gosnmp.SnmpV3AuthProtocol
	// authKey      string
	// privProtocol gosnmp.SnmpV3PrivProtocol
	// privKey      string

	// TODO: communication
	retries int
	timeout int

	// TODO: debugging
	// pingDevice  bool
	// checkConfig bool

)

func init() {
	AgentCmd.AddCommand(snmpwalkCmd)
	snmpwalkCmd.Flags().StringVarP(&snmpVersion, "snmp-version", "v", defaultVersion, "Specify SNMP version to use")

	// snmp v1 or v2c specific
	snmpwalkCmd.Flags().StringVarP(&communityString, "community-string", "C", defaultCommunityString, "Set the community string for version 1 or 2c")

	// TODO: snmp v3 specific
	// snmpwalkCmd.Flags().StringVarP(&user, "user-name", "u", defaultUserName, "Set the user name for v3")
	// snmpwalkCmd.Flags().StringVarP(&authProtocol, "auth-protocol", "a", defaultAuthProtocol, "Set the authentication protocol for v3")
	// snmpwalkCmd.Flags().StringVarP(&authKey, "auth-key", "A", defaultAuthKey, "Set the authentication passphrase for v3")
	// snmpwalkCmd.Flags().StringVarP(&privProtocol, "priv-protocol", "x", defaultPrivProtocol, "Set the privacy protocol for v3")
	// snmpwalkCmd.Flags().StringVarP(&privKey, "priv-key", "X", defaultprivKey, "Set the privacy passphrase for v3")

	// general communication options
	snmpwalkCmd.Flags().IntVarP(&retries, "retries", "r", defaultRetries, "Set the number of retries")
	snmpwalkCmd.Flags().IntVarP(&timeout, "timeout", "t", defaultTimeout, "Set the request timeout (in seconds)")

	// TODO: debugging options
	// snmpwalkCmd.Flags().BoolVarP(&pingDevice, "ping", "P", false, "Ping the device before performing the snmpwalk") // connectivity check
	// snmpwalkCmd.Flags().BoolVarP(&checkConfig, "config", "", false, "Load device configuration") // config check

	snmpwalkCmd.SetArgs([]string{"ipAddress"})
}

var snmpwalkCmd = &cobra.Command{
	Use:   "snmpwalk ipAddress",
	Short: "Perform a snmpwalk",
	Long:  ``,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Get args
		if len(args) == 0 {
			fmt.Print("Missing argument: IP address\n")
			os.Exit(1)
		} else if len(args) == 1 {
			address = args[0]
			oid = defaultOID
		} else if len(args) == 2 {
			address = args[0]
			oid = args[1]
		} else {
			fmt.Printf("The number of arguments must be between 1 and 2. %d arguments were given.\n", len(args))
			os.Exit(1)
		}
		if strings.Contains(address, ":") {
			deviceIP = address[:strings.Index(address, ":")]
			value, _ = strconv.ParseUint(address[strings.Index(address, ":")+1:], 0, 16)
			port = uint16(value)
		} else {
			deviceIP = address
			port = defaultPort
		}
		// TODO: add authentication check
		// if communityString == "" && user == "" {
		// 	fmt.Printf("No authentication mechanism specified")
		// 	os.Exit(1)
		// }

		// Set the snmp version
		if snmpVersion == "1" {
			setVersion = gosnmp.Version1
		} else if snmpVersion == "2c" || (snmpVersion == "" && communityString != "") {
			setVersion = gosnmp.Version2c
			// TODO: v3
			// } else if snmpVersion == "3" || (snmpVersion == "" && user != "") {
			// 	setVersion = gosnmp.Version3
		} else {
			fmt.Printf("SNMP version not supported: %s, using default version 1.", snmpVersion)
			setVersion = gosnmp.Version1
		}

		// Set the default values
		if port == 0 {
			port = defaultPort
		}
		if timeout == 0 {
			timeout = defaultTimeout
		}
		if retries == 0 {
			retries = defaultRetries
		}

		// Set SNMP parameters
		snmp = gosnmp.GoSNMP{
			Target:    deviceIP,
			Port:      port,
			Community: communityString,
			Transport: "udp",
			Version:   setVersion,
			Timeout:   time.Duration(10 * time.Second), // Timeout better suited to walking
			Retries:   retries,
			// TODO: v3
			// SecurityParameters: &gosnmp.UsmSecurityParameters{
			// 	UserName:                 user,
			// 	AuthenticationProtocol:   authProtocol,
			// 	AuthenticationPassphrase: AuthKey,
			// 	PrivacyProtocol:          privProtocol,
			// 	PrivacyPassphrase:        privKey,
		}
		// Estbalish connection
		err := snmp.Connect()
		if err != nil {
			fmt.Printf("Connect err: %v\n", err)
			os.Exit(1)
		}
		defer snmp.Conn.Close()

		defer timeTrack(time.Now(), snmp.Version)

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
	default:
		fmt.Printf("TYPE %d: %d\n", pdu.Type, gosnmp.ToBigInt(pdu.Value))
	}

	return nil
}

func timeTrack(start time.Time, versionName gosnmp.SnmpVersion) {
	elapsed := time.Since(start)
	log.Printf("SnmpWalk for version %s took %s", versionName, elapsed)
}
