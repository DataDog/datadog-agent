// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package profile

import (
	"regexp"
)

type CmdOption func(*PlainCommand)

func Expect(exp string) CmdOption {
	re := regexp.MustCompile(exp)
	return func(pc *PlainCommand) {
		pc.Validator.Require = append(pc.Validator.Require, re)
	}
}

func ExpectNot(exp string) CmdOption {
	re := regexp.MustCompile(exp)
	return func(pc *PlainCommand) {
		pc.Validator.Reject = append(pc.Validator.Reject, re)
	}
}

func MkCommand(command string, options ...CmdOption) *PlainCommand {
	cmd := &PlainCommand{
		Command:   command,
		Validator: Validator{},
	}
	for _, opt := range options {
		opt(cmd)
	}
	return cmd
}

// RedactionOption configures a RedactionRule built by MkRedaction.
type RedactionOption func(*RedactionRule)

// WithReplacement overrides the default replacement string ("$1 <secret hidden>").
func WithReplacement(replacement string) RedactionOption {
	return func(r *RedactionRule) {
		r.Replacement = replacement
	}
}

// WithMultiline sets Multiline to true on the RedactionRule.
func WithMultiline() RedactionOption {
	return func(r *RedactionRule) {
		r.Multiline = true
	}
}

func MkRedaction(regex string, opts ...RedactionOption) RedactionRule {
	r := RedactionRule{
		Regex:       regexp.MustCompile(regex),
		Replacement: "$1 <secret hidden>",
	}
	for _, opt := range opts {
		opt(&r)
	}
	return r
}

// Names of the built-in NCM device profiles in DefaultProfiles.
const (
	ProfileAOSCX    ProfileName = "aoscx"
	ProfileAOSW     ProfileName = "aosw"
	ProfileCiscoASA ProfileName = "cisco-asa"
	ProfileCiscoIOS ProfileName = "cisco-ios"
	ProfileDellOS10 ProfileName = "dellos10"
	ProfileEOS      ProfileName = "eos"
	ProfileFortiOS  ProfileName = "fortios"
	ProfileJunos    ProfileName = "junos"
	ProfileNXOS     ProfileName = "nxos"
	ProfilePanOS    ProfileName = "pan-os"
	ProfileTMOS     ProfileName = "tmos"
)

// DefaultProfiles is the built-in set of NCM device profiles, keyed by profile name.
var DefaultProfiles = Map{
	ProfileAOSCX: {
		Name: ProfileAOSCX,
		Commands: CommandSet{
			Verify:     MkCommand("show system", Expect(`(AOS|ArubaOS)-CX Version`)),
			GetRunning: MkCommand("show running-config", Expect(`!Version (.*)?`)),
			GetStartup: MkCommand("show startup-config", Expect(`!Version (.*)?`)),
			GetVersion: MkCommand("show version"),
		},
		Preprocessing: []RedactionRule{
			MkRedaction(`.*configuration:\s*!\s*!Version.*\s*(?:!export-password:.*\s)?`, WithReplacement(""), WithMultiline()),
		},
		Redactions: []RedactionRule{
			MkRedaction(`^(snmp-server community) \S+(.*)`),
			MkRedaction(`^(snmp-server host \S+) \S+(.*)`),
			MkRedaction(`^(radius-server host \S+ key) \S+(.*)`),
			MkRedaction(`^(radius-server key).*`),
			MkRedaction(`^(tacacs-server host \S+ key) \S+(.*)`),
			MkRedaction(`^(tacacs-server key).*`),
			MkRedaction(`^(user \S+ group \S+ password) \S+(.*)`),
		},
	},

	ProfileAOSW: {
		Name: ProfileAOSW,
		Commands: CommandSet{
			Verify:     MkCommand("show version", Expect(`(Alcatel-Lucent Operating System-Wireless|AOS-W|AOS-10)`)),
			GetRunning: MkCommand("show running-config", Expect(`Building Configuration...`)),
			GetVersion: MkCommand("show version"),
		},
		Preprocessing: []RedactionRule{
			MkRedaction(`Building Configuration...\s*`, WithReplacement(""), WithMultiline()),
		},
		Redactions: []RedactionRule{
			MkRedaction(`(?m)^(secret) (\S+)\s?$`),
			MkRedaction(`(?m)^(enable secret) (\S+)\s?$`),
			MkRedaction(`(?mi)^(.*pre-share) (\S+)\s?$`),
			MkRedaction(`(?m)^(.*ipsec) (\S+)\s?$`),
			MkRedaction(`(?m)^(.*community) (\S+)\s?$`),
			MkRedaction(`( sha) (\S+)`),
			MkRedaction(`( des) (\S+)`),
			MkRedaction(`(?m)^(mobility-manager \S+ user \S+) (\S+)`),
			MkRedaction(`(?m)^(mgmt-user \S+ (?:root|guest-provisioning|network-operations|read-only|location-api-mgmt)) (\S+)\s?$`),
			MkRedaction(`(?m)^(mgmt-user \S+) (\S+)( (?:read-only|guest-mgmt))?\s?$`, WithReplacement("$1 <secret hidden> $3")),
			MkRedaction(`(?m)^(.*key) (\S+)\s?$`),
			MkRedaction(`(?m)^(vrrp-passphrase) (\S+)\s?$`),
			MkRedaction(`(?m)^(wpa-passphrase) (\S+)\s?$`),
			MkRedaction(`(?m)^(bkup-passwords) (\S+)\s?$`),
			MkRedaction(`(?m)^(ap-console-password) (\S+)\s?$`),
			MkRedaction(`(?m)^(virtual-controller-key) (\S+)\s?$`),
			MkRedaction(`community (.*?)\s*$`, WithReplacement("community <secret hidden>")),
			MkRedaction(`(?m)^(snmp-server host \S+ (?:trap )?version (?:v?[123]c?|v3)) (\S+)(.*)`, WithReplacement("$1 <secret hidden>$3")),
			MkRedaction(`(?m)^(vrrp \d+.*\n.*\n)(authentication) (\S+)`, WithReplacement("$1$2 <secret hidden>"), WithMultiline()),
		},
	},

	ProfileCiscoASA: {
		Name: ProfileCiscoASA,
		Commands: CommandSet{
			Verify:     MkCommand("show version", Expect("Cisco Adaptive Security Appliance Software Version")),
			GetRunning: MkCommand("more system:running-config", Expect(`ASA Version \d+\.\d+\(\d+\)`)),
			GetVersion: MkCommand("show version"),
		},
		Redactions: []RedactionRule{
			MkRedaction(`(?m)^(snmp-server community).*`),
			MkRedaction(`(?m)^(enable password) \S+( .*)?$`, WithReplacement("$1 <secret hidden>$2")),
			MkRedaction(`(?m)^(passwd) \S+( .*)?$`, WithReplacement("$1 <secret hidden>$2")),
			MkRedaction(`(?m)^(username \S+ password) \S+( .*)?$`, WithReplacement("$1 <secret hidden>$2")),
			MkRedaction(`(ikev[12] ((remote|local)-authentication )?pre-shared-key( hex)?) (\S+)`),
			MkRedaction(`(?m)^(crypto isakmp key) \S+( .*)?$`, WithReplacement("$1 <secret hidden>$2")),
			MkRedaction(`(?m)^(aaa-server \S+(?: \(\S+\))? host \S+\n(?: [^\n]+\n)* +key) \S+$`, WithMultiline()),
			MkRedaction(`ldap-login-password (\S+)`, WithReplacement("ldap-login-password <secret hidden>")),
			MkRedaction(`(?m)^snmp-server host (.*) community (\S+)`, WithReplacement("snmp-server host $1 community <secret hidden>")),
			MkRedaction(`(?m)^(failover key) .+`),
			MkRedaction(`(?m)^(\s+ospf message-digest-key \d+ md5) .+`),
			MkRedaction(`(?m)^(\s+ospf authentication-key) .+`),
			MkRedaction(`(?m)^(\s+neighbor \S+ password) .+`),
		},
	},

	ProfileCiscoIOS: {
		Name: ProfileCiscoIOS,
		Commands: CommandSet{
			Verify:     MkCommand("show version", Expect(`(Cisco IOS|Cisco Internetwork Operating System)`)),
			GetRunning: MkCommand("show running-config", Expect(`Building configuration...`), Expect(`Current configuration :`)),
			GetStartup: MkCommand("show startup-config", Expect(`Using (.*?) out of (.*?) bytes`)),
			GetVersion: MkCommand("show version"),
			PushConfig: []Command{
				&SCPCommand{
					RemoteCommand: "scp",
					Filepath:      "flash:/dd-rollback-config",
				},
				MkCommand("configure replace flash:/dd-rollback-config force", Expect("Rollback Done")),
				MkCommand("write", Expect("[OK]")),
			},
		},
		Preprocessing: []RedactionRule{
			MkRedaction(`(?m)^\s*Building configuration...\s*`, WithReplacement(""), WithMultiline()),
			MkRedaction(`Current configuration : (.*)\s*`, WithReplacement(""), WithMultiline()),
			MkRedaction(`(?m)(?:^!\s*$\s*)?^! Last configuration change at .*?$\s*(?:^!\s*$\s*)?`, WithReplacement(""), WithMultiline()),
			MkRedaction(`(?m)(?:^!\s*$\s*)?^! NVRAM config last updated at .*$\s*(?:^!\s*$\s*)?`, WithReplacement(""), WithMultiline()),
			MkRedaction(`(?s)^\s*Using \d+ out of \d+ bytes\s*`, WithReplacement(""), WithMultiline()),
		},
		Redactions: []RedactionRule{
			MkRedaction(`(?m)^(snmp-server community).*`),
			MkRedaction(`(?m)^(snmp-server host \S+( vrf \S+)?( informs?)?( version (1|2c))?) +\S+( .*)?$`, WithReplacement("$1 <secret hidden>$6")),
			MkRedaction(`(?m)^(username .+ (password|secret) \d) .+`),
			MkRedaction(`(?m)^(enable (password|secret)( level \d+)? \d) .+`),
			MkRedaction(`(?m)^( +(?:password|secret)) (?:\d )?\S+`),
			MkRedaction(`(?m)^(.*wpa-psk ascii \d) (\S+)`),
			MkRedaction(`(?m)^(.*key 7) (\d.+)`),
			MkRedaction(`(?m)^(tacacs-server (.+ )?key) .+`),
			MkRedaction(`(?m)^(crypto isakmp key) (\S+) (.*)`, WithReplacement("$1 <secret hidden> $3")),
			MkRedaction(`(?m)^( +ip ospf message-digest-key \d+ md5) .+`),
			MkRedaction(`(?m)^( +ip ospf authentication-key) .+`),
			MkRedaction(`(?m)^( +neighbor \S+ password) .+`),
			MkRedaction(`(?m)^( +vrrp \d+ authentication text) .+`),
			MkRedaction(`(?m)^( +standby \d+ authentication) .{1,8}$`),
			MkRedaction(`(?m)^( +standby \d+ authentication md5 key-string) .+?( timeout \d+)?$`, WithReplacement("$1 <secret hidden> $2")),
			MkRedaction(`(?m)^( +key-string) .+`),
			MkRedaction(`(?m)^((tacacs|radius) server [^\n]+\n( +[^\n]+\n)* +key) [^\n]+$`),
			MkRedaction(`(?m)^( +ppp (chap|pap) password \d) .+`),
			MkRedaction(`(?m)^( +security wpa psk set-key (?:ascii|hex) \d) (.*)$`),
			MkRedaction(`(?m)^( +dot1x username \S+ password \d) (.*)$`),
			MkRedaction(`(?m)^( +mgmtuser username \S+ password \d) (.*) (secret \d) (.*)$`, WithReplacement("$1 <secret hidden> $3 <secret hidden>")),
			MkRedaction(`(?m)^( +client \S+ server-key \d) (.*)$`),
			MkRedaction(`(?m)^( +domain-password) \S+ ?(.*)`, WithReplacement("$1 <secret hidden> $2")),
			MkRedaction(`(?m)^( +pre-shared-key).*`),
			MkRedaction(`(?m)^(.*server-key(?: \d)?) \S+`),
		},
		MetadataRules: []MetadataRule{
			{
				Type:   Timestamp,
				Regex:  regexp.MustCompile(`(?m)^! Last configuration change at (.*?)(?:\s+by \S+)?$`),
				Format: "15:04:05 MST Mon Jan 2 2006",
			},
			{
				Type:  ConfigSize,
				Regex: regexp.MustCompile(`Current configuration : (?P<Size>\d+)`),
			},
			{
				Type:  ConfigSize,
				Regex: regexp.MustCompile(`Using (?P<Size>\d+) out of (.*?) bytes`),
			},
		},
	},

	ProfileDellOS10: {
		Name: ProfileDellOS10,
		Commands: CommandSet{
			Verify:     MkCommand("show version", Expect(`(Dell EMC Networking|Dell Application Software)`)),
			GetRunning: MkCommand("show running-configuration", Expect(`! Version (.*)?`)),
			GetStartup: MkCommand("show startup-configuration", Expect(`(?m)^hostname\s+\S+`)),
		},
		Preprocessing: []RedactionRule{
			MkRedaction(`(?m)^! Version .*$\s*(?:^!\s*$\s*)?`, WithReplacement(""), WithMultiline()),
			MkRedaction(`(?m)^! Last configuration change at .*$\s*(?:^!\s*$\s*)?`, WithReplacement(""), WithMultiline()),
		},
		Redactions: []RedactionRule{
			MkRedaction(`(password )(\S+)`, WithReplacement("${1}<secret hidden>")),
		},
		MetadataRules: []MetadataRule{
			{
				Type:   Timestamp,
				Regex:  regexp.MustCompile(`(?m)^! Last configuration change at (.*)$`),
				Format: "Jan 2 15:04:05 2006",
			},
		},
	},

	ProfileEOS: {
		Name: ProfileEOS,
		Commands: CommandSet{
			Verify:     MkCommand("show version", Expect(`Arista .*`)),
			GetRunning: MkCommand("show running-config | no-more | exclude ! Time:", Expect(`! Command: show running-config`)),
			GetStartup: MkCommand("show startup-config | no-more | exclude ! Time:", Expect(`! Command: show startup-config`)),
			PushConfig: []Command{
				&SCPCommand{
					RemoteCommand: "scp",
					Filepath:      "/tmp/dd-rollback-config",
				},
				MkCommand("configure replace file:/tmp/dd-rollback-config", ExpectNot("%")),
				MkCommand("write", Expect(`Copy completed successfully`)),
				// TODO should we be deleting the file after?
				// MkCommand("delete file:/tmp/dd-rollback-config"),
			},
		},
		Preprocessing: []RedactionRule{
			MkRedaction(`(?m)^! Command:.*$\n(?:^! device:.*$\n)?(?:^!$\n)*(?:^! boot system.*$\n)?(?:^!$\n)*`, WithReplacement(""), WithMultiline()),
			MkRedaction(`(?m)^! Command: show startup-config.*$\n`, WithReplacement(""), WithMultiline()),
			MkRedaction(`(?m)^! Startup-config last modified at.*$\n`, WithReplacement(""), WithMultiline()),
			// Note that the multiline flag means this only matches a ! at the very beginning of the config.
			MkRedaction(`^!\n`, WithReplacement(""), WithMultiline()),
		},
		Redactions: []RedactionRule{
			MkRedaction(`^(snmp-server community).*`),
			MkRedaction(`(secret \w+) (\S+).*`),
			MkRedaction(`(password \d+) (\S+).*`),
			MkRedaction(`^(service unsupported-transceiver).*`),
			MkRedaction(`^(tacacs-server key \d+).*`),
			MkRedaction(`^(radius-server .+ key \d) \S+`),
			MkRedaction(`( {6}key) ([0-9a-fA-F]+ 7) ([0-9a-fA-F]+).*`),
			MkRedaction(`(localized|auth (md5|sha\d{0,3})|priv (des|aes\d{0,3})) \S+`),
		},
		MetadataRules: []MetadataRule{
			{
				Type:   Timestamp,
				Regex:  regexp.MustCompile(`(?m)^! Startup-config last modified at\s+(.+?)\s+by`),
				Format: "Mon Jan 2 15:04:05 2006",
			},
			{
				Type:  Author,
				Regex: regexp.MustCompile(`(?m)^! Startup-config last modified at\s+.*\s+by\s(.*)\s`),
			},
		},
	},

	ProfileFortiOS: {
		Name: ProfileFortiOS,
		Commands: CommandSet{
			Verify:     MkCommand("get system status", Expect(`Version: FortiGate`)),
			GetRunning: MkCommand("show full-configuration", Expect(`config (system|global|vdom)`)),
		},
		Redactions: []RedactionRule{
			MkRedaction(`^(#private-encryption-key=).+`),
			MkRedaction(`(set .+ ENC) .+`),
			MkRedaction(`(set (?:passwd|password|key|group-password|auth-password-l1|auth-password-l2|rsso|history0|history1))\s*( ENC)? .+`, WithReplacement("$1$2 <secret hidden>")),
			MkRedaction(`(set md5-key [0-9]+) .+`),
			MkRedaction(`(?s)(set private-key ).*?-+END (ENCRYPTED|RSA|OPENSSH) PRIVATE KEY-+\n?"$`, WithReplacement("$1<secret hidden>")),
			MkRedaction(`(?s)(set privatekey ).*?-+END (ENCRYPTED|RSA|OPENSSH) PRIVATE KEY-+\n?"$`, WithReplacement("$1<secret hidden>")),
			MkRedaction(`(?s)(set ca )"-+BEGIN.*?-+END CERTIFICATE-+"$`, WithReplacement("$1<secret hidden>")),
			MkRedaction(`(?s)(set csr ).*?-+END CERTIFICATE REQUEST-+"$`, WithReplacement("$1<secret hidden>")),
		},
	},

	ProfileJunos: {
		Name: ProfileJunos,
		Commands: CommandSet{
			Verify:     MkCommand("show version", Expect(`Junos:`)),
			GetRunning: MkCommand("show configuration | display omit", Expect(`version \d+\.\d+[^;]*;`)),
			GetVersion: MkCommand("show version"),
		},
		Redactions: []RedactionRule{
			MkRedaction(`(?m)^(\s*community) (\S+) (\{)`, WithReplacement("$1 <secret hidden> {")),
			MkRedaction(` \"\$\d\$\S+; ## SECRET-DATA`, WithReplacement(" <secret hidden>;")),
		},
		MetadataRules: []MetadataRule{
			{
				Type:   Timestamp,
				Regex:  regexp.MustCompile(`## Last commit: (.*) by`),
				Format: "2006-01-02 15:04:05 MST",
			},
			{
				Type:  Author,
				Regex: regexp.MustCompile(`(?m)^## Last commit: .* by (.*?)\s*$`),
			},
		},
	},

	ProfileNXOS: {
		Name: ProfileNXOS,
		Commands: CommandSet{
			Verify:     MkCommand("show version", Expect(`Cisco Nexus Operating System`)),
			GetRunning: MkCommand("show running-config", Expect(`!Command: show running-config`)),
			GetStartup: MkCommand("show startup-config", Expect(`!Command: show startup-config`)),
			GetVersion: MkCommand("show version"),
		},
		Preprocessing: []RedactionRule{
			MkRedaction(`!Command: show running-config\s*`, WithReplacement(""), WithMultiline()),
			MkRedaction(`(?m)^!Running configuration last done at:.*?$\s*(?:^!\s*$\s*)?`, WithReplacement(""), WithMultiline()),
			MkRedaction(`(?m)^!Time: .*?$\s*(?:^!\s*$\s*)?`, WithReplacement(""), WithMultiline()),
			MkRedaction(`!Command: show startup-config\s*`, WithReplacement(""), WithMultiline()),
			MkRedaction(`(?m)^!Startup config saved at: .*?$\s*(?:^!\s*$\s*)?`, WithReplacement(""), WithMultiline()),
		},
		Redactions: []RedactionRule{
			MkRedaction(`^(snmp-server community).*`),
			MkRedaction(`^(snmp-server user \S+ \S+ auth \S+) \S+( priv \S+) \S+(.*)`, WithReplacement("$1 <secret hidden>$2 <secret hidden>$3")),
			MkRedaction(`^(snmp-server host.*? )\S+( udp-port \d+)?$`, WithReplacement("$1<secret hidden>$2")),
			MkRedaction(`^(snmp-server mib community-map) \S+ ?(.*)`, WithReplacement("$1 <secret hidden> $2")),
			MkRedaction(`(password \d+) (\S+)`),
			MkRedaction(`^(radius-server .*key(?: \d+)?) \S+`),
			MkRedaction(`^(tacacs-server .*key(?: \d+)?) \S+`),
		},
		MetadataRules: []MetadataRule{
			{
				Type:   Timestamp,
				Regex:  regexp.MustCompile(`(?m)^!Running configuration last done at: (.*?)(?:\s+by \S+)?$`),
				Format: "Mon Jan _2 15:04:05 2006",
			},
			{
				Type:   Timestamp,
				Regex:  regexp.MustCompile(`(?m)^!Startup config saved at: (.*?)(?:\s+by \S+)?$`),
				Format: "Mon Jan _2 15:04:05 2006",
			},
		},
	},

	ProfilePanOS: {
		Name: ProfilePanOS,
		Commands: CommandSet{
			Verify:     MkCommand("show system info", Expect(`model: *PA-`)),
			GetRunning: MkCommand("show config running", Expect(`(?s)<config.*</config>`)),
			GetVersion: MkCommand("show system info"),
		},
		Redactions: []RedactionRule{
			MkRedaction(`<phash>.*?</phash>`, WithReplacement("<phash><secret hidden></phash>")),
		},
	},

	ProfileTMOS: {
		Name: ProfileTMOS,
		Commands: CommandSet{
			Verify:     MkCommand("cat /config/partitions/*/bigip*.conf", Expect(`(^sys global-settings\s*{)|(^ltm (node|pool|virtual) \S+ {)|(^#TMSH-VERSION: \S+)`)),
			GetRunning: MkCommand("cat /config/partitions/*/bigip*.conf", Expect(`(^sys global-settings\s*{)|(^ltm (node|pool|virtual) \S+ {)|(^#TMSH-VERSION: \S+)`)),
		},
		Redactions: []RedactionRule{
			MkRedaction(`^([\s\t]*)secret \S+`, WithReplacement("${1}secret <secret hidden>")),
			MkRedaction(`^([\s\t]*\S*)password \S+`, WithReplacement("${1}password <secret hidden>")),
			MkRedaction(`^([\s\t]*\S*)passphrase \S+`, WithReplacement("${1}passphrase <secret hidden>")),
			MkRedaction(`^(\s*)community \S+`, WithReplacement("${1}community <secret hidden>")),
			MkRedaction(`^(\s*)community-name \S+`, WithReplacement("${1}community-name <secret hidden>")),
			MkRedaction(`^([\s\t]*\S*)encrypted \S+$`, WithReplacement("${1}encrypted <secret hidden>")),
		},
	},
}
