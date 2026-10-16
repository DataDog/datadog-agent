package security

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// SDDL syntax and token meanings follow [MS-DTYP] section 2.5.1.1;
// binary ACE types and flags follow section 2.4.4.1. The file access
// mask aliases are listed in section 2.5.1.1's object-specific rights table.
//
// This implements a subset of SDDL: object GUIDs, callback conditions,
// resource attributes, and machine- or domain-relative SID aliases are not supported.
// Windows-specific tokens follow the Windows SDK's sddl.h and winnt.h,
// as listed in https://learn.microsoft.com/en-us/windows/win32/secauthz/ace-strings.
// The NO_ACCESS_CONTROL token is not implemented here.

// String returns the SDDL representation of the security descriptor.
// NULL ACLs and raw ACEs are omitted, so the result is not a lossless encoding.
func (d *Descriptor) String() string {
	if d == nil {
		return ""
	}
	var b strings.Builder
	d.writeSDDL(&b)
	return b.String()
}

func (d *Descriptor) writeSDDL(b *strings.Builder) {
	if d.Owner != nil {
		b.WriteString("O:")
		writeSDDLSID(b, d.Owner)
	}
	if d.Group != nil {
		b.WriteString("G:")
		writeSDDLSID(b, d.Group)
	}
	if d.DACL != nil && d.DACL != NullACL {
		b.WriteString("D:")
		d.DACL.writeSDDL(b)
	}
	if d.SACL != nil && d.SACL != NullACL {
		b.WriteString("S:")
		d.SACL.writeSDDL(b)
	}
}

// String returns the SDDL representation of the ACL.
// NULL ACLs return an empty string; raw ACEs are omitted.
func (acl *ACL) String() string {
	if acl == nil || acl == NullACL {
		return ""
	}
	var b strings.Builder
	acl.writeSDDL(&b)
	return b.String()
}

func (acl *ACL) writeSDDL(b *strings.Builder) {
	if acl.Protected {
		b.WriteByte('P')
	}
	if acl.AutoInheritRequested {
		b.WriteString("AR")
	}
	if acl.AutoInherited {
		b.WriteString("AI")
	}
	for i := range acl.ACEs {
		acl.ACEs[i].writeSDDL(b)
	}
}

// String returns the SDDL representation of the ACE.
// Raw ACEs return an empty string.
func (ace *ACE) String() string {
	if ace == nil || ace.Raw != nil {
		return ""
	}
	var b strings.Builder
	ace.writeSDDL(&b)
	return b.String()
}

func (ace *ACE) writeSDDL(b *strings.Builder) {
	if ace == nil || ace.Raw != nil {
		return
	}
	b.WriteByte('(')
	ace.Type.writeSDDL(b)
	b.WriteByte(';')
	ace.Flags.writeSDDL(b, ace.Type)
	b.WriteByte(';')
	ace.Mask.writeSDDL(b)
	b.WriteString(";;;")
	writeSDDLSID(b, ace.SID)
	b.WriteByte(')')
}

func (t ACEType) writeSDDL(b *strings.Builder) {
	switch t {
	case AccessAllowed:
		b.WriteString("A")
	case AccessDenied:
		b.WriteString("D")
	case systemAudit:
		b.WriteString("AU")
	case systemAlarm:
		b.WriteString("AL")
	case accessAllowedObject:
		b.WriteString("OA")
	case accessDeniedObject:
		b.WriteString("OD")
	case systemAuditObject:
		b.WriteString("OU")
	case systemAlarmObject:
		b.WriteString("OL")
	case accessAllowedCallback:
		b.WriteString("XA")
	case accessDeniedCallback:
		b.WriteString("XD")
	// The numeric XU/ZA entries in [MS-DTYP] 2.5.1.1 conflict with
	// their type names and 2.4.4.1. Use the named types: the Windows SDK's
	// sddl.h defines XU as SDDL_CALLBACK_AUDIT and ZA as
	// SDDL_CALLBACK_OBJECT_ACCESS_ALLOWED.
	// https://github.com/microsoft/win32metadata/blob/main/generation/WinSDK/RecompiledIdlHeaders/shared/sddl.h
	case accessAllowedCallbackObject:
		b.WriteString("ZA")
	case systemAuditCallback:
		b.WriteString("XU")
	case systemMandatoryLabel:
		b.WriteString("ML")
	case systemResourceAttribute:
		b.WriteString("RA")
	// [MS-DTYP] sections 2.4.4.1 and 2.5.1.1 assign SP to
	// SYSTEM_SCOPED_POLICY_ID_ACE_TYPE (0x13); 0x12 is resource attribute.
	case systemScopedPolicyID:
		b.WriteString("SP")
	case systemProcessTrustLabel:
		b.WriteString("TL")
	case systemAccessFilter:
		b.WriteString("FL")
	default:
		b.WriteString("0x")
		b.WriteString(strconv.FormatUint(uint64(t), 16))
	}
}

func (flags ACEFlags) writeSDDL(b *strings.Builder, aceType ACEType) {
	if flags&ContainerInherit != 0 {
		b.WriteString("CI")
	}
	if flags&ObjectInherit != 0 {
		b.WriteString("OI")
	}
	if flags&NoPropagateInherit != 0 {
		b.WriteString("NP")
	}
	if flags&InheritOnly != 0 {
		b.WriteString("IO")
	}
	if flags&Inherited != 0 {
		b.WriteString("ID")
	}
	if flags&critical != 0 {
		b.WriteString("CR")
	}
	if flags&successfulAccess != 0 {
		if aceType == systemAccessFilter {
			b.WriteString("TP")
		} else {
			b.WriteString("SA")
		}
	}
	if flags&failedAccess != 0 {
		b.WriteString("FA")
	}
}

// writeSDDL uses file aliases for common file masks, generic and standard
// tokens when possible, and hexadecimal otherwise. Object-specific bits use
// hexadecimal because their meaning depends on the object or ACE type.
func (mask AccessMask) writeSDDL(b *strings.Builder) {
	switch mask {
	case FileAllAccess:
		b.WriteString("FA")
		return
	case FileGenericRead:
		b.WriteString("FR")
		return
	case FileGenericWrite:
		b.WriteString("FW")
		return
	case FileGenericExecute:
		b.WriteString("FX")
		return
	}

	const knownRightsMask = GenericRead | GenericWrite | GenericExecute | GenericAll |
		WriteOwner | WriteDACL | ReadControl | Delete

	if mask != 0 && (mask&^knownRightsMask) == 0 {
		if mask&GenericRead != 0 {
			b.WriteString("GR")
		}
		if mask&GenericWrite != 0 {
			b.WriteString("GW")
		}
		if mask&GenericExecute != 0 {
			b.WriteString("GX")
		}
		if mask&GenericAll != 0 {
			b.WriteString("GA")
		}
		if mask&WriteOwner != 0 {
			b.WriteString("WO")
		}
		if mask&WriteDACL != 0 {
			b.WriteString("WD")
		}
		if mask&ReadControl != 0 {
			b.WriteString("RC")
		}
		if mask&Delete != 0 {
			b.WriteString("SD")
		}
		return
	}

	b.WriteString("0x")
	b.WriteString(strconv.FormatUint(uint64(mask), 16))
}

func writeSDDLSID(b *strings.Builder, sid *SID) {
	if sid == nil {
		return
	}
	b.WriteString(sddlSID(sid))
}

func sddlSID(sid *SID) string {
	if sid == nil {
		return ""
	}
	if sid.Revision == 1 {
		switch sid.IdentifierAuthority {
		case 1:
			if len(sid.SubAuthority) == 1 && sid.SubAuthority[0] == 0 {
				return "WD" // S-1-1-0
			}
		case 3:
			if len(sid.SubAuthority) == 1 {
				switch sid.SubAuthority[0] {
				case 0:
					return "CO" // S-1-3-0
				case 1:
					return "CG" // S-1-3-1
				case 4:
					return "OW" // S-1-3-4
				}
			}
		case 5:
			if len(sid.SubAuthority) == 1 {
				switch sid.SubAuthority[0] {
				case 2:
					return "NU" // S-1-5-2
				case 4:
					return "IU" // S-1-5-4
				case 6:
					return "SU" // S-1-5-6
				case 7:
					return "AN" // S-1-5-7
				case 9:
					return "ED" // S-1-5-9
				case 10:
					return "PS" // S-1-5-10
				case 11:
					return "AU" // S-1-5-11
				case 12:
					return "RC" // S-1-5-12
				case 18:
					return "SY" // S-1-5-18
				case 19:
					return "LS" // S-1-5-19
				case 20:
					return "NS" // S-1-5-20
				case 33:
					return "WR" // S-1-5-33
				}
			} else if len(sid.SubAuthority) == 2 && sid.SubAuthority[0] == 32 {
				switch sid.SubAuthority[1] {
				case 544:
					return "BA" // S-1-5-32-544
				case 545:
					return "BU" // S-1-5-32-545
				case 546:
					return "BG" // S-1-5-32-546
				case 547:
					return "PU" // S-1-5-32-547
				case 548:
					return "AO" // S-1-5-32-548
				case 549:
					return "SO" // S-1-5-32-549
				case 550:
					return "PO" // S-1-5-32-550
				case 551:
					return "BO" // S-1-5-32-551
				case 552:
					return "RE" // S-1-5-32-552
				case 554:
					return "RU" // S-1-5-32-554
				case 555:
					return "RD" // S-1-5-32-555
				case 556:
					return "NO" // S-1-5-32-556
				case 558:
					return "MU" // S-1-5-32-558
				case 559:
					return "LU" // S-1-5-32-559
				case 568:
					return "IS" // S-1-5-32-568
				case 569:
					return "CY" // S-1-5-32-569
				case 573:
					return "ER" // S-1-5-32-573
				case 574:
					return "CD" // S-1-5-32-574
				case 575:
					return "RA" // S-1-5-32-575
				case 576:
					return "ES" // S-1-5-32-576
				case 577:
					return "MS" // S-1-5-32-577
				case 578:
					return "HA" // S-1-5-32-578
				case 579:
					return "AA" // S-1-5-32-579
				case 580:
					return "RM" // S-1-5-32-580
				case 584:
					return "HO" // S-1-5-32-584
				case 585:
					return "SH" // S-1-5-32-585
				}
			} else if len(sid.SubAuthority) == 6 && sid.SubAuthority[0] == 84 &&
				sid.SubAuthority[1] == 0 && sid.SubAuthority[2] == 0 &&
				sid.SubAuthority[3] == 0 && sid.SubAuthority[4] == 0 &&
				sid.SubAuthority[5] == 0 {
				return "UD" // S-1-5-84-0-0-0-0-0
			}
		case 15:
			if len(sid.SubAuthority) == 2 && sid.SubAuthority[0] == 2 && sid.SubAuthority[1] == 1 {
				return "AC" // S-1-15-2-1
			}
		case 16:
			if len(sid.SubAuthority) == 1 {
				switch sid.SubAuthority[0] {
				case 4096:
					return "LW" // S-1-16-4096
				case 8192:
					return "ME" // S-1-16-8192
				case 8448:
					return "MP" // S-1-16-8448
				case 12288:
					return "HI" // S-1-16-12288
				case 16384:
					return "SI" // S-1-16-16384
				}
			}
		case 18:
			if len(sid.SubAuthority) == 1 {
				switch sid.SubAuthority[0] {
				case 1:
					return "AS" // S-1-18-1
				case 2:
					return "SS" // S-1-18-2
				}
			}
		}
	}
	return sid.String()
}

// Fixed SID aliases follow [MS-DTYP] sections 2.4.2.4 and 2.5.1.1 and the
// Windows SDK sddl.h. Additional Windows SID values are documented at:
// https://learn.microsoft.com/en-us/windows/win32/secauthz/well-known-sids
// Machine- and domain-relative aliases require context and are excluded.
var tokenToSID = map[string]*SID{
	"WD": {Revision: 1, IdentifierAuthority: 1, SubAuthority: []uint32{0}},
	"CO": {Revision: 1, IdentifierAuthority: 3, SubAuthority: []uint32{0}},
	"CG": {Revision: 1, IdentifierAuthority: 3, SubAuthority: []uint32{1}},
	"OW": {Revision: 1, IdentifierAuthority: 3, SubAuthority: []uint32{4}},
	"NU": {Revision: 1, IdentifierAuthority: 5, SubAuthority: []uint32{2}},
	"IU": {Revision: 1, IdentifierAuthority: 5, SubAuthority: []uint32{4}},
	"SU": {Revision: 1, IdentifierAuthority: 5, SubAuthority: []uint32{6}},
	"AN": {Revision: 1, IdentifierAuthority: 5, SubAuthority: []uint32{7}},
	"ED": {Revision: 1, IdentifierAuthority: 5, SubAuthority: []uint32{9}},
	"PS": {Revision: 1, IdentifierAuthority: 5, SubAuthority: []uint32{10}},
	"AU": {Revision: 1, IdentifierAuthority: 5, SubAuthority: []uint32{11}},
	"RC": {Revision: 1, IdentifierAuthority: 5, SubAuthority: []uint32{12}},
	"SY": {Revision: 1, IdentifierAuthority: 5, SubAuthority: []uint32{18}},
	"LS": {Revision: 1, IdentifierAuthority: 5, SubAuthority: []uint32{19}},
	"NS": {Revision: 1, IdentifierAuthority: 5, SubAuthority: []uint32{20}},
	"WR": {Revision: 1, IdentifierAuthority: 5, SubAuthority: []uint32{33}},
	"UD": {Revision: 1, IdentifierAuthority: 5, SubAuthority: []uint32{84, 0, 0, 0, 0, 0}},
	"BA": {Revision: 1, IdentifierAuthority: 5, SubAuthority: []uint32{32, 544}},

	"BU": {Revision: 1, IdentifierAuthority: 5, SubAuthority: []uint32{32, 545}},
	"BG": {Revision: 1, IdentifierAuthority: 5, SubAuthority: []uint32{32, 546}},
	"PU": {Revision: 1, IdentifierAuthority: 5, SubAuthority: []uint32{32, 547}},
	"AO": {Revision: 1, IdentifierAuthority: 5, SubAuthority: []uint32{32, 548}},
	"SO": {Revision: 1, IdentifierAuthority: 5, SubAuthority: []uint32{32, 549}},
	"PO": {Revision: 1, IdentifierAuthority: 5, SubAuthority: []uint32{32, 550}},
	"BO": {Revision: 1, IdentifierAuthority: 5, SubAuthority: []uint32{32, 551}},
	"RE": {Revision: 1, IdentifierAuthority: 5, SubAuthority: []uint32{32, 552}},
	"RU": {Revision: 1, IdentifierAuthority: 5, SubAuthority: []uint32{32, 554}},
	"RD": {Revision: 1, IdentifierAuthority: 5, SubAuthority: []uint32{32, 555}},
	"NO": {Revision: 1, IdentifierAuthority: 5, SubAuthority: []uint32{32, 556}},
	"MU": {Revision: 1, IdentifierAuthority: 5, SubAuthority: []uint32{32, 558}},
	"LU": {Revision: 1, IdentifierAuthority: 5, SubAuthority: []uint32{32, 559}},
	"IS": {Revision: 1, IdentifierAuthority: 5, SubAuthority: []uint32{32, 568}},
	"CY": {Revision: 1, IdentifierAuthority: 5, SubAuthority: []uint32{32, 569}},
	"ER": {Revision: 1, IdentifierAuthority: 5, SubAuthority: []uint32{32, 573}},
	"CD": {Revision: 1, IdentifierAuthority: 5, SubAuthority: []uint32{32, 574}},
	"RA": {Revision: 1, IdentifierAuthority: 5, SubAuthority: []uint32{32, 575}},
	"ES": {Revision: 1, IdentifierAuthority: 5, SubAuthority: []uint32{32, 576}},
	"MS": {Revision: 1, IdentifierAuthority: 5, SubAuthority: []uint32{32, 577}},
	"HA": {Revision: 1, IdentifierAuthority: 5, SubAuthority: []uint32{32, 578}},
	"AA": {Revision: 1, IdentifierAuthority: 5, SubAuthority: []uint32{32, 579}},
	"RM": {Revision: 1, IdentifierAuthority: 5, SubAuthority: []uint32{32, 580}},
	"HO": {Revision: 1, IdentifierAuthority: 5, SubAuthority: []uint32{32, 584}},
	"SH": {Revision: 1, IdentifierAuthority: 5, SubAuthority: []uint32{32, 585}},
	"AC": {Revision: 1, IdentifierAuthority: 15, SubAuthority: []uint32{2, 1}},
	"LW": {Revision: 1, IdentifierAuthority: 16, SubAuthority: []uint32{4096}},
	"ME": {Revision: 1, IdentifierAuthority: 16, SubAuthority: []uint32{8192}},
	"MP": {Revision: 1, IdentifierAuthority: 16, SubAuthority: []uint32{8448}},
	"HI": {Revision: 1, IdentifierAuthority: 16, SubAuthority: []uint32{12288}},
	"SI": {Revision: 1, IdentifierAuthority: 16, SubAuthority: []uint32{16384}},
	"AS": {Revision: 1, IdentifierAuthority: 18, SubAuthority: []uint32{1}},
	"SS": {Revision: 1, IdentifierAuthority: 18, SubAuthority: []uint32{2}},
}

func parseSIDString(s string) (*SID, error) {
	if sid, ok := tokenToSID[s]; ok {
		subs := make([]uint32, len(sid.SubAuthority))
		copy(subs, sid.SubAuthority)
		return &SID{
			Revision:            sid.Revision,
			IdentifierAuthority: sid.IdentifierAuthority,
			SubAuthority:        subs,
		}, nil
	}
	switch s {
	case "LA", "LG", "DA", "DG", "DU", "DC", "DD", "CA", "SA", "EA",
		"PA", "RO", "CN", "AP", "KA", "EK", "RS":
		return nil, fmt.Errorf("SID alias %q requires a machine or domain SID; use a full S-1-... SID", s)
	}
	return ParseSID(s)
}

// ParseDescriptor parses an SDDL string into a Descriptor.
// It supports access-allowed, access-denied, audit, mandatory-label, and
// scoped-policy-ID ACEs. Object GUIDs, callback conditions, resource attributes,
// and NULL ACL tokens are not supported. Machine- and domain-relative SID
// aliases (such as LG and DA) must be supplied as full S-1-... SIDs.
func ParseDescriptor(sddl string) (*Descriptor, error) {
	sddl = strings.TrimSpace(sddl)
	if sddl == "" {
		return &Descriptor{}, nil
	}

	type section struct {
		tag string
		val string
	}

	var sections []section
	inParen := false
	start := 0
	currentTag := ""

	i := 0
	for i < len(sddl) {
		switch sddl[i] {
		case '(':
			inParen = true
			i++
		case ')':
			inParen = false
			i++
		default:
			if !inParen && i+1 < len(sddl) && sddl[i+1] == ':' {
				tag := sddl[i : i+1]
				if tag == "O" || tag == "G" || tag == "D" || tag == "S" {
					if currentTag != "" {
						sections = append(sections, section{tag: currentTag, val: sddl[start:i]})
					} else if i > 0 && strings.TrimSpace(sddl[:i]) != "" {
						return nil, fmt.Errorf("unexpected content before first tag: %q", sddl[:i])
					}
					currentTag = tag
					start = i + 2
					i += 2
					continue
				}
			}
			i++
		}
	}
	if inParen {
		return nil, errors.New("unmatched parenthesis in SDDL")
	}
	if currentTag != "" {
		sections = append(sections, section{tag: currentTag, val: sddl[start:]})
	} else {
		return nil, fmt.Errorf("invalid SDDL string: %q", sddl)
	}

	d := &Descriptor{}
	for _, sec := range sections {
		switch sec.tag {
		case "O":
			if d.Owner != nil {
				return nil, errors.New("duplicate owner in SDDL")
			}
			owner, err := parseSIDString(sec.val)
			if err != nil {
				return nil, fmt.Errorf("invalid owner SID: %w", err)
			}
			d.Owner = owner
		case "G":
			if d.Group != nil {
				return nil, errors.New("duplicate group in SDDL")
			}
			group, err := parseSIDString(sec.val)
			if err != nil {
				return nil, fmt.Errorf("invalid group SID: %w", err)
			}
			d.Group = group
		case "D":
			if d.DACL != nil {
				return nil, errors.New("duplicate DACL in SDDL")
			}
			dacl, err := parseSDDLACL(sec.val)
			if err != nil {
				return nil, fmt.Errorf("invalid DACL: %w", err)
			}
			d.DACL = dacl
		case "S":
			if d.SACL != nil {
				return nil, errors.New("duplicate SACL in SDDL")
			}
			sacl, err := parseSDDLACL(sec.val)
			if err != nil {
				return nil, fmt.Errorf("invalid SACL: %w", err)
			}
			d.SACL = sacl
		}
	}

	// Validate before exposing the descriptor: [MS-DTYP] section 2.4.4
	// defines ACE layouts and their ACL placement, which must remain encodable.
	if _, _, err := d.validate(); err != nil {
		return nil, fmt.Errorf("invalid security descriptor: %w", err)
	}
	return d, nil
}

// MustDescriptor parses an SDDL string into a Descriptor, panicking on error.
func MustDescriptor(s string) *Descriptor {
	d, err := ParseDescriptor(s)
	if err != nil {
		panic(err)
	}
	return d
}

func parseSDDLACL(s string) (*ACL, error) {
	acl := &ACL{Revision: aclRevision}
	flagsPart, acesPart, hasACEs := strings.Cut(s, "(")
	if hasACEs {
		acesPart = "(" + acesPart
	}

	for len(flagsPart) > 0 {
		if rest, ok := strings.CutPrefix(flagsPart, "P"); ok {
			acl.Protected = true
			flagsPart = rest
		} else if rest, ok := strings.CutPrefix(flagsPart, "AR"); ok {
			acl.AutoInheritRequested = true
			flagsPart = rest
		} else if rest, ok := strings.CutPrefix(flagsPart, "AI"); ok {
			acl.AutoInherited = true
			flagsPart = rest
		} else {
			return nil, fmt.Errorf("unrecognized ACL flag: %q", flagsPart)
		}
	}

	for len(acesPart) > 0 {
		if acesPart[0] != '(' {
			return nil, fmt.Errorf("expected '(' at start of ACE: %q", acesPart)
		}
		aceStr, rest, ok := strings.Cut(acesPart[1:], ")")
		if !ok {
			return nil, errors.New("unclosed '(' in ACE")
		}
		ace, err := parseSDDLACE(aceStr)
		if err != nil {
			return nil, err
		}
		acl.ACEs = append(acl.ACEs, *ace)
		acesPart = rest
	}

	return acl, nil
}

func parseSDDLACE(s string) (*ACE, error) {
	parts := strings.Split(s, ";")
	if len(parts) != 6 {
		return nil, fmt.Errorf("invalid ACE format: expected 6 fields, got %d", len(parts))
	}

	// [MS-DTYP] section 2.5.1.1 defines fields 4 and 5 as object GUIDs,
	// while section 2.4.4.3 defines the corresponding object ACE GUIDs and
	// presence flags; ACE has no representation for them, so reject them
	// rather than silently discarding them.
	if parts[3] != "" || parts[4] != "" {
		return nil, errors.New("unsupported object GUID fields in ACE")
	}

	aceType, err := parseSDDLType(parts[0])
	if err != nil {
		return nil, err
	}
	// Only these types have structured payloads supported by this parser.
	// Recognizing a type token does not imply support for its ACE layout.
	switch aceType {
	case AccessAllowed, AccessDenied, systemAudit, systemMandatoryLabel, systemScopedPolicyID:
	default:
		return nil, fmt.Errorf("unsupported ACE type: %q (0x%02x)", parts[0], byte(aceType))
	}

	flags, err := parseSDDLFlags(parts[1], aceType)
	if err != nil {
		return nil, err
	}

	mask, err := parseSDDLRights(parts[2])
	if err != nil {
		return nil, err
	}

	if parts[5] == "" {
		return nil, errors.New("missing SID in ACE")
	}
	sid, err := parseSIDString(parts[5])
	if err != nil {
		return nil, fmt.Errorf("invalid SID in ACE: %w", err)
	}

	return &ACE{
		Type:  aceType,
		Flags: flags,
		Mask:  mask,
		SID:   sid,
	}, nil
}

func parseSDDLType(s string) (ACEType, error) {
	switch s {
	case "A":
		return AccessAllowed, nil
	case "D":
		return AccessDenied, nil
	case "AU":
		return systemAudit, nil
	case "AL":
		return systemAlarm, nil
	case "OA":
		return accessAllowedObject, nil
	case "OD":
		return accessDeniedObject, nil
	case "OU":
		return systemAuditObject, nil
	case "OL":
		return systemAlarmObject, nil
	case "XA":
		return accessAllowedCallback, nil
	case "XD":
		return accessDeniedCallback, nil
	case "XU":
		// See ACEType.writeSDDL for the XU/ZA discrepancy in [MS-DTYP].
		return systemAuditCallback, nil
	case "ZA":
		return accessAllowedCallbackObject, nil
	case "ML":
		return systemMandatoryLabel, nil
	case "RA":
		return systemResourceAttribute, nil
	// [MS-DTYP] sections 2.4.4.1 and 2.5.1.1 assign SP to
	// SYSTEM_SCOPED_POLICY_ID_ACE_TYPE (0x13), not 0x12.
	case "SP":
		return systemScopedPolicyID, nil
	case "TL":
		return systemProcessTrustLabel, nil
	case "FL":
		return systemAccessFilter, nil
	default:
		if strings.HasPrefix(s, "0x") || strings.HasPrefix(s, "0X") {
			v, err := strconv.ParseUint(s[2:], 16, 8)
			if err != nil {
				return 0, fmt.Errorf("invalid ACE type: %q", s)
			}
			return ACEType(v), nil
		}
		return 0, fmt.Errorf("unsupported ACE type: %q", s)
	}
}

func parseSDDLFlags(s string, aceType ACEType) (ACEFlags, error) {
	var flags ACEFlags
	for len(s) > 0 {
		if len(s) < 2 {
			return 0, fmt.Errorf("invalid ACE flags: %q", s)
		}
		token := s[:2]
		s = s[2:]
		switch token {
		case "CI":
			flags |= ContainerInherit
		case "OI":
			flags |= ObjectInherit
		case "NP":
			flags |= NoPropagateInherit
		case "IO":
			flags |= InheritOnly
		case "ID":
			flags |= Inherited
		case "CR":
			// winnt.h restricts CRITICAL_ACE_FLAG to access-allowed types.
			switch aceType {
			case AccessAllowed, accessAllowedObject, accessAllowedCallback, accessAllowedCallbackObject:
			default:
				return 0, fmt.Errorf("ACE flag CR requires an access-allowed ACE")
			}
			flags |= critical
		case "SA", "FA":
			// winnt.h restricts these flags to audit and alarm ACEs,
			// including their object and callback variants.
			switch aceType {
			case systemAudit, systemAlarm, systemAuditObject, systemAlarmObject,
				systemAuditCallback, systemAlarmCallback, systemAuditCallbackObject, systemAlarmCallbackObject:
			default:
				return 0, fmt.Errorf("ACE flag %s requires an audit or alarm ACE", token)
			}
			if token == "SA" {
				flags |= successfulAccess
			} else {
				flags |= failedAccess
			}
		case "TP":
			if aceType != systemAccessFilter {
				return 0, errors.New("ACE flag TP requires an access-filter ACE")
			}
			flags |= trustProtectedFilter
		default:
			return 0, fmt.Errorf("unknown ACE flag: %q", token)
		}
	}
	return flags, nil
}

func parseSDDLRights(s string) (AccessMask, error) {
	if s == "" {
		return 0, nil
	}

	if s[0] >= '0' && s[0] <= '9' {
		v, err := strconv.ParseUint(s, 0, 32)
		if err != nil {
			return 0, fmt.Errorf("invalid numeric rights value: %q", s)
		}
		return AccessMask(v), nil
	}

	var mask AccessMask
	rem := s
	for len(rem) > 0 {
		if len(rem) < 2 {
			return 0, fmt.Errorf("invalid rights string: %q", s)
		}
		token := rem[:2]
		rem = rem[2:]
		switch token {
		case "FA":
			mask |= FileAllAccess
		case "FR":
			mask |= FileGenericRead
		case "FW":
			mask |= FileGenericWrite
		case "FX":
			mask |= FileGenericExecute
		case "KA":
			mask |= 0x000f003f
		case "KR", "KX":
			mask |= 0x00020019
		case "KW":
			mask |= 0x00020006
		case "NW":
			mask |= mandatoryLabelNoWriteUp
		case "NR":
			mask |= mandatoryLabelNoReadUp
		case "NX":
			mask |= mandatoryLabelNoExecuteUp
		case "GA":
			mask |= GenericAll
		case "GR":
			mask |= GenericRead
		case "GW":
			mask |= GenericWrite
		case "GX":
			mask |= GenericExecute
		case "RC":
			mask |= ReadControl
		case "SD":
			mask |= Delete
		case "WD":
			mask |= WriteDACL
		case "WO":
			mask |= WriteOwner
		case "CC":
			mask |= FileReadData
		case "DC":
			mask |= FileWriteData
		case "LC":
			mask |= FileAppendData
		case "SW":
			mask |= FileReadEA
		case "RP":
			mask |= FileWriteEA
		case "WP":
			mask |= FileExecute
		case "DT":
			mask |= FileDeleteChild
		case "LO":
			mask |= FileReadAttributes
		case "CR":
			mask |= FileWriteAttributes
		default:
			return 0, fmt.Errorf("unknown rights token: %q", token)
		}
	}
	return mask, nil
}
