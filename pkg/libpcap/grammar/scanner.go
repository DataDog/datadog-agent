// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package grammar

import (
	"fmt"
	"net"
	"strconv"
	"strings"
	"unicode"
)

// Scanner tokenizes a BPF filter expression string.
// It implements the yyLexer interface expected by goyacc.
type Scanner struct {
	input string
	pos   int
	err   string
}

// NewScanner creates a scanner for the given filter expression.
func NewScanner(input string) *Scanner {
	return &Scanner{input: input}
}

// Error is called by the parser on syntax errors.
func (s *Scanner) Error(msg string) {
	s.err = msg
}

// Err returns the last error message, if any.
func (s *Scanner) Err() string {
	return s.err
}

// Lex returns the next token and fills lval with its value.
// Returns 0 at end of input.
func (s *Scanner) Lex(lval *yySymType) int {
	// Skip whitespace
	for s.pos < len(s.input) {
		c := s.input[s.pos]
		if c == ' ' || c == '\t' || c == '\r' || c == '\n' {
			s.pos++
		} else {
			break
		}
	}

	if s.pos >= len(s.input) {
		return 0 // EOF
	}

	c := s.input[s.pos]

	// IPv6 addresses starting with :: (must check before operators)
	if c == ':' && s.pos+1 < len(s.input) && s.input[s.pos+1] == ':' {
		// Look ahead: :: followed by hex digit or end-of-input means IPv6
		if s.pos+2 >= len(s.input) || isHexDigit(s.input[s.pos+2]) {
			return s.scanIPv6(lval)
		}
	}

	// Two-character operators
	if s.pos+1 < len(s.input) {
		two := s.input[s.pos : s.pos+2]
		switch two {
		case ">=":
			s.pos += 2
			return GEQ
		case "<=":
			s.pos += 2
			return LEQ
		case "!=":
			s.pos += 2
			return NEQ
		case "==":
			s.pos += 2
			return int('=')
		case "<<":
			s.pos += 2
			return LSH
		case ">>":
			s.pos += 2
			return RSH
		case "&&":
			s.pos += 2
			return AND
		case "||":
			s.pos += 2
			return OR
		}
	}

	// Single-character operators
	if strings.ContainsRune("+-*/%:[]!<>()&|^=", rune(c)) {
		s.pos++
		return int(c)
	}

	// AID: $hex
	if c == '$' && s.pos+1 < len(s.input) {
		start := s.pos
		s.pos++ // skip $
		for s.pos < len(s.input) && isHexDigit(s.input[s.pos]) {
			s.pos++
		}
		lval.s = s.input[start:s.pos]
		return AID
	}

	// Escaped identifier: \something
	if c == '\\' {
		s.pos++ // skip backslash
		start := s.pos
		for s.pos < len(s.input) && !isSpace(s.input[s.pos]) &&
			s.input[s.pos] != '!' && s.input[s.pos] != '(' &&
			s.input[s.pos] != ')' {
			s.pos++
		}
		lval.s = s.input[start:s.pos]
		return ID
	}

	// Numbers, dotted addresses, identifiers, keywords
	if isDigit(c) || c == '.' || isAlpha(c) || c == '_' || c == '-' {
		return s.scanWord(lval)
	}

	// Unknown character
	s.pos++
	return LEX_ERROR
}

// scanIPv6 scans an IPv6 address starting with "::".
func (s *Scanner) scanIPv6(lval *yySymType) int {
	start := s.pos
	// Consume characters valid in IPv6: hex digits, colons, dots (for ::ffff:1.2.3.4)
	for s.pos < len(s.input) {
		c := s.input[s.pos]
		if isHexDigit(c) || c == ':' || c == '.' {
			s.pos++
		} else {
			break
		}
	}
	word := s.input[start:s.pos]
	if ip := net.ParseIP(word); ip != nil {
		lval.s = word
		if ip.To4() == nil {
			return HID6
		}
		return HID
	}
	// Not a valid IP — fallback: return just the ":"
	s.pos = start + 1
	return int(':')
}

// scanWord handles keywords, identifiers, numbers, and addresses.
func (s *Scanner) scanWord(lval *yySymType) int {
	start := s.pos

	// First pass: collect without colons (alphanumeric, dots, hyphens, underscores)
	for s.pos < len(s.input) {
		c := s.input[s.pos]
		if isAlphaNum(c) || c == '.' || c == '-' || c == '_' {
			s.pos++
		} else {
			break
		}
	}

	// If followed by ':', this might be a MAC address (xx:xx:...) or IPv6.
	// Extend the scan to include colons and subsequent hex characters only if
	// it looks like a MAC or IPv6 pattern.
	if s.pos < len(s.input) && s.input[s.pos] == ':' {
		// Speculatively extend to include colon-separated tokens
		saved := s.pos
		for s.pos < len(s.input) {
			c := s.input[s.pos]
			if isAlphaNum(c) || c == ':' || c == '.' {
				s.pos++
			} else {
				break
			}
		}
		extended := s.input[start:s.pos]
		// Keep the extension only if it's a valid MAC or IPv6 address
		if isMAC(extended) || isIPv6(extended) {
			// Use extended word
		} else {
			// Not a MAC or IPv6 — revert to before the colon
			s.pos = saved
		}
	}

	word := s.input[start:s.pos]

	// Check keywords first (exact match)
	if tok, ok := keywordToken(word); ok {
		return s.handleKeyword(tok, word, lval)
	}

	// Try MAC address (xx:xx:xx:xx:xx:xx or xx-xx-xx-xx-xx-xx or xxxx.xxxx.xxxx)
	if isMAC(word) {
		lval.s = word
		return EID
	}

	// Try IPv6 address
	if strings.Contains(word, "::") || isIPv6(word) {
		if ip := net.ParseIP(word); ip != nil && ip.To4() == nil {
			lval.s = word
			return HID6
		}
	}

	// Try dotted IPv4 address (N.N, N.N.N, or N.N.N.N)
	if isDottedAddr(word) {
		lval.s = word
		return HID
	}

	// Try pure number
	if isNumeric(word) {
		n, err := parseNumber(word)
		if err != nil {
			s.Error(fmt.Sprintf("number %s: %v", word, err))
			return LEX_ERROR
		}
		lval.h = n
		return NUM
	}

	// Identifier
	lval.s = word
	return ID
}

// handleKeyword processes a keyword match and returns the appropriate token.
func (s *Scanner) handleKeyword(tok int, word string, lval *yySymType) int {
	switch tok {
	case NUM:
		// Keywords that return NUM with a specific value (ICMP types, TCP flags, etc.)
		lval.h = numericKeywordValue(word)
		return NUM
	default:
		return tok
	}
}

// keywords maps keyword strings to token types.
var keywords = map[string]int{
	"dst":        DST,
	"src":        SRC,
	"host":       HOST,
	"gateway":    GATEWAY,
	"net":        NET,
	"mask":       NETMASK,
	"port":       PORT,
	"portrange":  PORTRANGE,
	"less":       LESS,
	"greater":    GREATER,
	"proto":      PROTO,
	"protochain": PROTOCHAIN,
	"byte":       CBYTE,

	"link":  LINK,
	"ether": LINK,
	"ppp":   LINK,
	"slip":  LINK,
	"fddi":  LINK,
	"tr":    LINK,
	"wlan":  LINK,

	"arp":  ARP,
	"rarp": RARP,
	"ip":   IP,
	"sctp": SCTP,
	"tcp":  TCP,
	"udp":  UDP,
	"icmp": ICMP,
	"igmp": IGMP,
	"igrp": IGRP,
	"pim":  PIM,
	"vrrp": VRRP,
	"carp": CARP,

	"ip6":    IPV6,
	"icmp6":  ICMPV6,
	"ah":     AH,
	"esp":    ESP,

	"atalk":  ATALK,
	"aarp":   AARP,
	"decnet": DECNET,
	"lat":    LAT,
	"sca":    SCA,
	"moprc":  MOPRC,
	"mopdl":  MOPDL,

	"iso":   ISO,
	"esis":  ESIS,
	"es-is": ESIS,
	"isis":  ISIS,
	"is-is": ISIS,
	"l1":    L1,
	"l2":    L2,
	"iih":   IIH,
	"lsp":   LSP,
	"snp":   SNP,
	"csnp":  CSNP,
	"psnp":  PSNP,
	"clnp":  CLNP,
	"stp":   STP,
	"ipx":   IPX,
	"netbeui": NETBEUI,

	"broadcast": TK_BROADCAST,
	"multicast": TK_MULTICAST,

	"and": AND,
	"or":  OR,
	"not": int('!'),

	"len":    LEN,
	"length": LEN,

	"inbound":  INBOUND,
	"outbound": OUTBOUND,
	"ifindex":  IFINDEX,

	"vlan":   VLAN,
	"mpls":   MPLS,
	"pppoed": PPPOED,
	"pppoes": PPPOES,
	"geneve": GENEVE,

	"lane":        LANE,
	"llc":         LLC,
	"metac":       METAC,
	"bcc":         BCC,
	"oam":         OAM,
	"oamf4":       OAMF4,
	"oamf4ec":     OAMF4EC,
	"oamf4sc":     OAMF4SC,
	"sc":          SC,
	"ilmic":       ILMIC,
	"vpi":         VPI,
	"vci":         VCI,
	"connectmsg":  CONNECTMSG,
	"metaconnect": METACONNECT,

	"on":         PF_IFNAME,
	"ifname":     PF_IFNAME,
	"rset":       PF_RSET,
	"ruleset":    PF_RSET,
	"rnr":        PF_RNR,
	"rulenum":    PF_RNR,
	"srnr":       PF_SRNR,
	"subrulenum": PF_SRNR,
	"reason":     PF_REASON,
	"action":     PF_ACTION,

	"fisu":  FISU,
	"lssu":  LSSU,
	"lsu":   LSSU,
	"msu":   MSU,
	"hfisu": HFISU,
	"hlssu": HLSSU,
	"hmsu":  HMSU,
	"sio":   SIO,
	"opc":   OPC,
	"dpc":   DPC,
	"sls":   SLS,
	"hsio":  HSIO,
	"hopc":  HOPC,
	"hdpc":  HDPC,
	"hsls":  HSLS,

	"radio": RADIO,

	"type":      TYPE,
	"subtype":   SUBTYPE,
	"direction": DIR,
	"dir":       DIR,
	"address1":  ADDR1,
	"addr1":     ADDR1,
	"address2":  ADDR2,
	"addr2":     ADDR2,
	"address3":  ADDR3,
	"addr3":     ADDR3,
	"address4":  ADDR4,
	"addr4":     ADDR4,
	"ra":        RA,
	"ta":        TA,

	// ICMP type names → NUM
	"icmptype":          NUM,
	"icmpcode":          NUM,
	"icmp-echoreply":    NUM,
	"icmp-unreach":      NUM,
	"icmp-sourcequench": NUM,
	"icmp-redirect":     NUM,
	"icmp-echo":         NUM,
	"icmp-routeradvert": NUM,
	"icmp-routersolicit": NUM,
	"icmp-timxceed":     NUM,
	"icmp-paramprob":    NUM,
	"icmp-tstamp":       NUM,
	"icmp-tstampreply":  NUM,
	"icmp-ireq":         NUM,
	"icmp-ireqreply":    NUM,
	"icmp-maskreq":      NUM,
	"icmp-maskreply":    NUM,

	// ICMPv6 type names → NUM
	"icmp6type": NUM,
	"icmp6code": NUM,
	"icmp6-destinationunreach":        NUM,
	"icmp6-packettoobig":              NUM,
	"icmp6-timeexceeded":              NUM,
	"icmp6-parameterproblem":          NUM,
	"icmp6-echo":                      NUM,
	"icmp6-echoreply":                 NUM,
	"icmp6-multicastlistenerquery":    NUM,
	"icmp6-multicastlistenerreportv1": NUM,
	"icmp6-multicastlistenerdone":     NUM,
	"icmp6-routersolicit":             NUM,
	"icmp6-routeradvert":              NUM,
	"icmp6-neighborsolicit":           NUM,
	"icmp6-neighboradvert":            NUM,
	"icmp6-redirect":                  NUM,
	"icmp6-routerrenum":               NUM,
	"icmp6-nodeinformationquery":      NUM,
	"icmp6-nodeinformationresponse":   NUM,
	"icmp6-ineighbordiscoverysolicit": NUM,
	"icmp6-ineighbordiscoveryadvert":  NUM,
	"icmp6-multicastlistenerreportv2": NUM,
	"icmp6-homeagentdiscoveryrequest": NUM,
	"icmp6-homeagentdiscoveryreply":   NUM,
	"icmp6-mobileprefixsolicit":       NUM,
	"icmp6-mobileprefixadvert":        NUM,
	"icmp6-certpathsolicit":           NUM,
	"icmp6-certpathadvert":            NUM,
	"icmp6-multicastrouteradvert":     NUM,
	"icmp6-multicastroutersolicit":    NUM,
	"icmp6-multicastrouterterm":       NUM,

	// TCP flags → NUM
	"tcpflags": NUM,
	"tcp-fin":  NUM,
	"tcp-syn":  NUM,
	"tcp-rst":  NUM,
	"tcp-push": NUM,
	"tcp-ack":  NUM,
	"tcp-urg":  NUM,
	"tcp-ece":  NUM,
	"tcp-cwr":  NUM,
}

// numericKeywords maps keyword names to their numeric values (for keywords that return NUM).
var numericKeywords = map[string]uint32{
	"icmptype": 0, "icmpcode": 1,
	"icmp-echoreply": 0, "icmp-unreach": 3, "icmp-sourcequench": 4,
	"icmp-redirect": 5, "icmp-echo": 8, "icmp-routeradvert": 9,
	"icmp-routersolicit": 10, "icmp-timxceed": 11, "icmp-paramprob": 12,
	"icmp-tstamp": 13, "icmp-tstampreply": 14, "icmp-ireq": 15,
	"icmp-ireqreply": 16, "icmp-maskreq": 17, "icmp-maskreply": 18,

	"icmp6type": 0, "icmp6code": 1,
	"icmp6-destinationunreach": 1, "icmp6-packettoobig": 2,
	"icmp6-timeexceeded": 3, "icmp6-parameterproblem": 4,
	"icmp6-echo": 128, "icmp6-echoreply": 129,
	"icmp6-multicastlistenerquery": 130, "icmp6-multicastlistenerreportv1": 131,
	"icmp6-multicastlistenerdone": 132,
	"icmp6-routersolicit": 133, "icmp6-routeradvert": 134,
	"icmp6-neighborsolicit": 135, "icmp6-neighboradvert": 136,
	"icmp6-redirect": 137, "icmp6-routerrenum": 138,
	"icmp6-nodeinformationquery": 139, "icmp6-nodeinformationresponse": 140,
	"icmp6-ineighbordiscoverysolicit": 141, "icmp6-ineighbordiscoveryadvert": 142,
	"icmp6-multicastlistenerreportv2": 143,
	"icmp6-homeagentdiscoveryrequest": 144, "icmp6-homeagentdiscoveryreply": 145,
	"icmp6-mobileprefixsolicit": 146, "icmp6-mobileprefixadvert": 147,
	"icmp6-certpathsolicit": 148, "icmp6-certpathadvert": 149,
	"icmp6-multicastrouteradvert": 151,
	"icmp6-multicastroutersolicit": 152, "icmp6-multicastrouterterm": 153,

	"tcpflags": 13,
	"tcp-fin": 0x01, "tcp-syn": 0x02, "tcp-rst": 0x04, "tcp-push": 0x08,
	"tcp-ack": 0x10, "tcp-urg": 0x20, "tcp-ece": 0x40, "tcp-cwr": 0x80,
}

func keywordToken(word string) (int, bool) {
	tok, ok := keywords[word]
	return tok, ok
}

func numericKeywordValue(word string) uint32 {
	v, ok := numericKeywords[word]
	if ok {
		return v
	}
	return 0
}

// isMAC checks if a string looks like a MAC address.
func isMAC(s string) bool {
	_, err := net.ParseMAC(s)
	return err == nil
}

// isIPv6 checks if a string could be an IPv6 address.
func isIPv6(s string) bool {
	// Must contain at least one colon and be parseable as IP
	if !strings.Contains(s, ":") {
		return false
	}
	ip := net.ParseIP(s)
	return ip != nil && ip.To4() == nil
}

// isDottedAddr checks if a string is a dotted numeric address (N.N, N.N.N, or N.N.N.N).
func isDottedAddr(s string) bool {
	parts := strings.Split(s, ".")
	if len(parts) < 2 || len(parts) > 4 {
		return false
	}
	for _, p := range parts {
		if !isNumeric(p) {
			return false
		}
	}
	return true
}

// isNumeric checks if a string is a valid number (decimal, 0x hex, or 0 octal).
func isNumeric(s string) bool {
	if len(s) == 0 {
		return false
	}
	if len(s) >= 2 && s[0] == '0' && (s[1] == 'x' || s[1] == 'X') {
		for _, c := range s[2:] {
			if !isHexRune(c) {
				return false
			}
		}
		return len(s) > 2
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

// parseNumber parses a numeric string (decimal, hex with 0x prefix, octal with 0 prefix).
func parseNumber(s string) (uint32, error) {
	var n uint64
	var err error
	if len(s) >= 2 && s[0] == '0' && (s[1] == 'x' || s[1] == 'X') {
		n, err = strconv.ParseUint(s[2:], 16, 32)
	} else if len(s) > 1 && s[0] == '0' {
		n, err = strconv.ParseUint(s[1:], 8, 32)
	} else {
		n, err = strconv.ParseUint(s, 10, 32)
	}
	if err != nil {
		return 0, err
	}
	return uint32(n), nil
}

func isDigit(c byte) bool    { return c >= '0' && c <= '9' }
func isAlpha(c byte) bool    { return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') }
func isAlphaNum(c byte) bool { return isAlpha(c) || isDigit(c) }
func isHexDigit(c byte) bool { return isDigit(c) || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F') }
func isHexRune(c rune) bool  { return unicode.IsDigit(c) || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F') }
func isSpace(c byte) bool    { return c == ' ' || c == '\t' || c == '\n' || c == '\r' }
