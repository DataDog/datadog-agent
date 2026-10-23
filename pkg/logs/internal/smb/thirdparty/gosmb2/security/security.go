package security

import (
	"encoding/binary"
	"errors"
	"fmt"
)

// Information selects the parts of a security descriptor to query or modify.
//
// The values are defined by [MS-DTYP] section 2.4.7.
type Information uint32

const (
	Owner Information = 0x00000001
	Group Information = 0x00000002
	DACL  Information = 0x00000004
	SACL  Information = 0x00000008
)

func (info Information) validate() error {
	if info == 0 || info&^(Owner|Group|DACL|SACL) != 0 {
		return errors.New("invalid security information")
	}
	return nil
}

// Descriptor contains the security descriptor components exposed by the API.
type Descriptor struct {
	Owner                  *SID
	Group                  *SID
	DACL                   *ACL
	SACL                   *ACL
	controlFlags           uint16
	resourceManagerControl byte
}

// Information returns the security information flags representing the components
// present in the descriptor.
func (d *Descriptor) Information() Information {
	if d == nil {
		return 0
	}
	var info Information
	if d.Owner != nil {
		info |= Owner
	}
	if d.Group != nil {
		info |= Group
	}
	if d.DACL != nil {
		info |= DACL
	}
	if d.SACL != nil {
		info |= SACL
	}
	return info
}

// ACL is an ordered access control list. Use NullACL for a NULL ACL; a regular
// &ACL{} represents an empty ACL.
type ACL struct {
	Revision             uint8
	Protected            bool
	AutoInheritRequested bool
	AutoInherited        bool
	ACEs                 []ACE
}

// NullACL is a convenient value for selecting a NULL ACL when setting a
// descriptor and is also returned for a requested NULL ACL. Treat it
// as read-only; a regular &ACL{} represents an empty ACL.
var NullACL = &ACL{}

// ACE is an access control entry. Unknown ACEs are preserved in Raw.
type ACE struct {
	Type  ACEType
	Flags ACEFlags
	Mask  AccessMask
	SID   *SID
	Raw   []byte
}

// ACEType identifies the layout and purpose of an ACE.
type ACEType uint8

// ACE types for file access control, defined by [MS-DTYP] section 2.4.4.1.
const (
	AccessAllowed ACEType = 0x00
	AccessDenied  ACEType = 0x01
)

// Other ACE types from [MS-DTYP] section 2.4.4.1 are used internally
// when parsing, validating, and formatting descriptors.
const (
	systemAudit                 ACEType = 0x02
	systemAlarm                 ACEType = 0x03 // Reserved.
	accessAllowedCompound       ACEType = 0x04 // Reserved.
	accessAllowedObject         ACEType = 0x05
	accessDeniedObject          ACEType = 0x06
	systemAuditObject           ACEType = 0x07
	systemAlarmObject           ACEType = 0x08 // Reserved.
	accessAllowedCallback       ACEType = 0x09
	accessDeniedCallback        ACEType = 0x0a
	accessAllowedCallbackObject ACEType = 0x0b
	accessDeniedCallbackObject  ACEType = 0x0c
	systemAuditCallback         ACEType = 0x0d
	systemAlarmCallback         ACEType = 0x0e // Reserved.
	systemAuditCallbackObject   ACEType = 0x0f
	systemAlarmCallbackObject   ACEType = 0x10 // Reserved.
	systemMandatoryLabel        ACEType = 0x11
	systemResourceAttribute     ACEType = 0x12
	systemScopedPolicyID        ACEType = 0x13
)

// Additional ACE types defined by the Windows SDK's winnt.h.
// Their payloads are retained in ACE.Raw.
// https://github.com/microsoft/win32metadata/blob/main/generation/WinSDK/RecompiledIdlHeaders/um/winnt.h
const (
	systemProcessTrustLabel ACEType = 0x14
	systemAccessFilter      ACEType = 0x15
)

// ACEFlags controls inheritance and auditing for an ACE.
type ACEFlags uint8

const (
	ObjectInherit      ACEFlags = 0x01
	ContainerInherit   ACEFlags = 0x02
	NoPropagateInherit ACEFlags = 0x04
	InheritOnly        ACEFlags = 0x08
	Inherited          ACEFlags = 0x10
)

// Audit and alarm flags defined by [MS-DTYP] section 2.4.4.1 and winnt.h.
const (
	successfulAccess ACEFlags = 0x40
	failedAccess     ACEFlags = 0x80
)

// critical marks an access-allowed ACE as critical, as defined by the
// Windows SDK's winnt.h.
const critical ACEFlags = 0x20

// The Windows SDK's access-filter flag shares its bit with successfulAccess;
// the ACE type determines its meaning.
const trustProtectedFilter ACEFlags = 0x40

// AccessMask specifies access rights granted, denied, or audited by an ACE.
type AccessMask uint32

// File and directory access rights defined by [MS-SMB2] section 2.2.13.1.
const (
	FileReadData        AccessMask = 0x00000001
	FileWriteData       AccessMask = 0x00000002
	FileAppendData      AccessMask = 0x00000004
	FileReadEA          AccessMask = 0x00000008
	FileWriteEA         AccessMask = 0x00000010
	FileExecute         AccessMask = 0x00000020
	FileDeleteChild     AccessMask = 0x00000040
	FileReadAttributes  AccessMask = 0x00000080
	FileWriteAttributes AccessMask = 0x00000100

	FileListDirectory   AccessMask = 0x00000001
	FileAddFile         AccessMask = 0x00000002
	FileAddSubdirectory AccessMask = 0x00000004
	FileTraverse        AccessMask = 0x00000020
)

// Standard and generic access rights defined by [MS-DTYP] section 2.4.3.
const (
	Delete               AccessMask = 0x00010000
	ReadControl          AccessMask = 0x00020000
	WriteDACL            AccessMask = 0x00040000
	WriteOwner           AccessMask = 0x00080000
	Synchronize          AccessMask = 0x00100000
	AccessSystemSecurity AccessMask = 0x01000000
	GenericAll           AccessMask = 0x10000000
	GenericExecute       AccessMask = 0x20000000
	GenericWrite         AccessMask = 0x40000000
	GenericRead          AccessMask = 0x80000000
)

// Common composite access masks for files.
const (
	FileAllAccess      AccessMask = 0x001f01ff
	FileGenericRead    AccessMask = 0x00120089
	FileGenericWrite   AccessMask = 0x00120116
	FileGenericExecute AccessMask = 0x001200a0
)

// Mandatory label access policy bits defined by [MS-DTYP] section 2.4.4.13.
const (
	mandatoryLabelNoWriteUp   AccessMask = 0x00000001
	mandatoryLabelNoReadUp    AccessMask = 0x00000002
	mandatoryLabelNoExecuteUp AccessMask = 0x00000004
)

const (
	securityDescriptorRevision                        = 1
	securityDescriptorSelfRelative             uint16 = 0x8000
	securityDescriptorDACLPresent              uint16 = 0x0004
	securityDescriptorSACLPresent              uint16 = 0x0010
	securityDescriptorDACLProtected            uint16 = 0x1000
	securityDescriptorSACLProtected            uint16 = 0x2000
	securityDescriptorDACLAutoInheritRequested uint16 = 0x0100
	securityDescriptorSACLAutoInheritRequested uint16 = 0x0200
	securityDescriptorDACLAutoInherited        uint16 = 0x0400
	securityDescriptorSACLAutoInherited        uint16 = 0x0800
	securityDescriptorRMControlValid           uint16 = 0x4000

	aclRevision   = 0x02
	aclRevisionDS = 0x04
)

func (d *Descriptor) validate() (saclSize, daclSize int, err error) {
	if d == nil {
		return 0, 0, errors.New("nil security descriptor")
	}
	if d.Information() == 0 {
		return 0, 0, errors.New("no selected component")
	}

	if err := d.Owner.validate(); err != nil {
		return 0, 0, fmt.Errorf("owner: %w", err)
	}
	if err := d.Group.validate(); err != nil {
		return 0, 0, fmt.Errorf("group: %w", err)
	}

	saclSize, err = d.SACL.validate(true)
	if err != nil {
		return 0, 0, fmt.Errorf("sacl: %w", err)
	}
	daclSize, err = d.DACL.validate(false)
	if err != nil {
		return 0, 0, fmt.Errorf("dacl: %w", err)
	}
	return saclSize, daclSize, nil
}

func (acl *ACL) validate(sacl bool) (int, error) {
	if acl == nil || acl == NullACL {
		return 0, nil
	}
	rev := acl.Revision
	if rev == 0 {
		rev = aclRevision
	}
	if rev != aclRevision && rev != aclRevisionDS {
		return 0, fmt.Errorf("invalid ACL revision")
	}
	if len(acl.ACEs) > 0xffff {
		return 0, fmt.Errorf("too many ACEs")
	}

	size := 8
	for i := range acl.ACEs {
		ace := &acl.ACEs[i]
		aceSize, err := ace.validate(rev, sacl)
		if err != nil {
			return 0, fmt.Errorf("ACE %d: %w", i, err)
		}
		if aceSize > 0xffff-size {
			return 0, fmt.Errorf("ACL is too large")
		}
		size += aceSize
	}
	return size, nil
}

func (ace *ACE) validate(revision uint8, sacl bool) (int, error) {
	if ace == nil {
		return 0, fmt.Errorf("nil ACE")
	}
	if !aceAllowedByRevision(ace.Type, revision) {
		return 0, fmt.Errorf("ACE type is invalid for ACL revision")
	}
	if aceIsDACLOnly(ace.Type) && sacl {
		return 0, fmt.Errorf("DACL ACE type is invalid for SACL")
	}
	if aceIsSACLOnly(ace.Type) && !sacl {
		return 0, fmt.Errorf("SACL ACE type is invalid for DACL")
	}
	switch ace.Type {
	case AccessAllowed, AccessDenied, systemAudit, systemMandatoryLabel, systemScopedPolicyID:
		if ace.Raw != nil || ace.SID == nil {
			return 0, fmt.Errorf("structured ACE has invalid raw or SID fields")
		}
		if err := ace.SID.validate(); err != nil {
			return 0, err
		}
		switch ace.Type {
		case systemMandatoryLabel:
			// [MS-DTYP] sections 2.4.4.13 and 2.4.4.13.1 require a
			// mandatory label SID with authority 16 and one recognized RID.
			if ace.SID.IdentifierAuthority != 16 || len(ace.SID.SubAuthority) != 1 {
				return 0, fmt.Errorf("invalid mandatory label SID")
			}
			switch ace.SID.SubAuthority[0] {
			case 0, 0x1000, 0x2000, 0x3000, 0x4000, 0x5000:
			default:
				return 0, fmt.Errorf("invalid mandatory label SID RID")
			}
		case systemScopedPolicyID:
			// [MS-DTYP] section 2.4.4.16 requires a zero access mask.
			if ace.Mask != 0 {
				return 0, fmt.Errorf("scoped policy ACE has non-zero mask")
			}
		}
		size := 8 + ace.SID.Size()
		if size > 0xffff || size&3 != 0 {
			return 0, fmt.Errorf("invalid ACE size")
		}
		return size, nil
	default:
		if len(ace.Raw) < 4 || len(ace.Raw) > 0xffff || len(ace.Raw)&3 != 0 {
			return 0, fmt.Errorf("invalid raw ACE size")
		}
		if ace.Raw[0] != byte(ace.Type) || ace.Raw[1] != byte(ace.Flags) || binary.LittleEndian.Uint16(ace.Raw[2:4]) != uint16(len(ace.Raw)) {
			return 0, fmt.Errorf("raw ACE header does not match fields")
		}
		if ace.Mask != 0 || ace.SID != nil {
			return 0, fmt.Errorf("raw ACE has structured fields")
		}
		return len(ace.Raw), nil
	}
}

func aceAllowedByRevision(aceType ACEType, revision uint8) bool {
	switch aceType {
	case accessAllowedObject, accessDeniedObject, systemAuditObject, systemAlarmObject,
		accessAllowedCallbackObject, accessDeniedCallbackObject, systemAuditCallbackObject,
		systemAlarmCallbackObject:
		return revision == aclRevisionDS
	default:
		return true
	}
}

func aceIsDACLOnly(aceType ACEType) bool {
	switch aceType {
	case AccessAllowed, AccessDenied, accessAllowedObject, accessDeniedObject, accessAllowedCallback,
		accessDeniedCallback, accessAllowedCallbackObject, accessDeniedCallbackObject:
		return true
	default:
		return false
	}
}

func aceIsSACLOnly(aceType ACEType) bool {
	switch aceType {
	case systemAudit, systemAuditObject, systemAuditCallback, systemAuditCallbackObject, systemMandatoryLabel,
		systemResourceAttribute, systemScopedPolicyID:
		return true
	default:
		return false
	}
}

func roundup4(n int) int {
	return (n + 3) &^ 3
}

// Size returns the wire size of the ACE in bytes.
func (ace *ACE) Size() int {
	if ace == nil {
		return 0
	}
	if ace.Raw != nil {
		return len(ace.Raw)
	}
	if ace.SID == nil {
		return 0
	}
	return 8 + ace.SID.Size()
}

// Encode encodes the ACE into its wire binary representation.
func (ace *ACE) Encode(p []byte) {
	if ace == nil || ace.Size() == 0 || len(p) < ace.Size() {
		return
	}
	if ace.Raw != nil {
		copy(p, ace.Raw)
		return
	}
	p[0] = byte(ace.Type)
	p[1] = byte(ace.Flags)
	binary.LittleEndian.PutUint16(p[2:4], uint16(ace.Size()))
	binary.LittleEndian.PutUint32(p[4:8], uint32(ace.Mask))
	ace.SID.Encode(p[8:])
}

// Size returns the wire size of the ACL in bytes, or 0 if acl is nil or NullACL.
func (acl *ACL) Size() int {
	if acl == nil || acl == NullACL {
		return 0
	}
	size := 8
	for i := range acl.ACEs {
		size += acl.ACEs[i].Size()
	}
	return size
}

// Encode encodes the ACL into its wire binary representation.
func (acl *ACL) Encode(p []byte) {
	size := acl.Size()
	if size == 0 || len(p) < size {
		return
	}
	rev := acl.Revision
	if rev == 0 {
		rev = aclRevision
	}
	p[0] = rev
	p[1] = 0
	binary.LittleEndian.PutUint16(p[2:4], uint16(size))
	binary.LittleEndian.PutUint16(p[4:6], uint16(len(acl.ACEs)))
	binary.LittleEndian.PutUint16(p[6:8], 0)
	off := 8
	for i := range acl.ACEs {
		acl.ACEs[i].Encode(p[off:])
		off += acl.ACEs[i].Size()
	}
}

// Encode encodes the descriptor into self-relative binary format.
// If no components are selected, it returns an error.
func (d *Descriptor) Encode() ([]byte, error) {
	saclSize, daclSize, err := d.validate()
	if err != nil {
		return nil, err
	}

	size := 20
	for _, n := range []int{d.Owner.Size(), d.Group.Size(), saclSize, daclSize} {
		if n == 0 {
			continue
		}
		size = roundup4(size)
		if uint64(size)+uint64(n) > uint64(^uint32(0)) {
			return nil, fmt.Errorf("security descriptor is too large")
		}
		size += n
	}
	if uint64(size) > uint64(^uint32(0)) {
		return nil, fmt.Errorf("security descriptor is too large")
	}

	p := make([]byte, size)
	ownerOffset, groupOffset, saclOffset, daclOffset := 0, 0, 0, 0
	off := 20
	if d.Owner != nil {
		off = roundup4(off)
		ownerOffset = off
		d.Owner.Encode(p[off:])
		off += d.Owner.Size()
	}
	if d.Group != nil {
		off = roundup4(off)
		groupOffset = off
		d.Group.Encode(p[off:])
		off += d.Group.Size()
	}
	if d.SACL != nil && d.SACL != NullACL {
		off = roundup4(off)
		saclOffset = off
		d.SACL.Encode(p[off:])
		off += saclSize
	}
	if d.DACL != nil && d.DACL != NullACL {
		off = roundup4(off)
		daclOffset = off
		d.DACL.Encode(p[off:])
	}

	p[0] = securityDescriptorRevision
	control := d.controlFlags | securityDescriptorSelfRelative
	control &^= securityDescriptorDACLPresent | securityDescriptorSACLPresent
	if d.DACL != NullACL {
		control &^= securityDescriptorDACLProtected | securityDescriptorDACLAutoInheritRequested | securityDescriptorDACLAutoInherited
	}
	if d.SACL != NullACL {
		control &^= securityDescriptorSACLProtected | securityDescriptorSACLAutoInheritRequested | securityDescriptorSACLAutoInherited
	}
	if control&securityDescriptorRMControlValid != 0 {
		p[1] = d.resourceManagerControl
	}
	if d.DACL != nil {
		control |= securityDescriptorDACLPresent
		if d.DACL.Protected {
			control |= securityDescriptorDACLProtected
		}
		if d.DACL.AutoInheritRequested {
			control |= securityDescriptorDACLAutoInheritRequested
		}
		if d.DACL.AutoInherited {
			control |= securityDescriptorDACLAutoInherited
		}
	}
	if d.SACL != nil {
		control |= securityDescriptorSACLPresent
		if d.SACL.Protected {
			control |= securityDescriptorSACLProtected
		}
		if d.SACL.AutoInheritRequested {
			control |= securityDescriptorSACLAutoInheritRequested
		}
		if d.SACL.AutoInherited {
			control |= securityDescriptorSACLAutoInherited
		}
	}
	binary.LittleEndian.PutUint16(p[2:4], control)
	binary.LittleEndian.PutUint32(p[4:8], uint32(ownerOffset))
	binary.LittleEndian.PutUint32(p[8:12], uint32(groupOffset))
	binary.LittleEndian.PutUint32(p[12:16], uint32(saclOffset))
	binary.LittleEndian.PutUint32(p[16:20], uint32(daclOffset))

	return p, nil
}

// DecodeDescriptor decodes a self-relative SECURITY_DESCRIPTOR binary buffer.
// If selection is specified, unselected components are set to nil. An absent
// ACL remains nil; a present NULL ACL is represented by NullACL.
func DecodeDescriptor(data []byte, selection ...Information) (*Descriptor, error) {
	if len(selection) > 1 {
		return nil, fmt.Errorf("security descriptor accepts at most one selection")
	}
	if len(selection) > 0 {
		if err := selection[0].validate(); err != nil {
			return nil, err
		}
	}
	if len(data) < 20 || data[0] != securityDescriptorRevision {
		return nil, fmt.Errorf("invalid security descriptor header")
	}
	control := binary.LittleEndian.Uint16(data[2:4])
	if control&securityDescriptorSelfRelative == 0 {
		return nil, fmt.Errorf("security descriptor is not self-relative")
	}

	owner, err := decodeSIDAt(data, binary.LittleEndian.Uint32(data[4:8]))
	if err != nil {
		return nil, fmt.Errorf("owner: %w", err)
	}
	group, err := decodeSIDAt(data, binary.LittleEndian.Uint32(data[8:12]))
	if err != nil {
		return nil, fmt.Errorf("group: %w", err)
	}
	sacl, err := decodeACLAt(data, binary.LittleEndian.Uint32(data[12:16]), control&securityDescriptorSACLPresent != 0, true)
	if err != nil {
		return nil, fmt.Errorf("sacl: %w", err)
	}
	dacl, err := decodeACLAt(data, binary.LittleEndian.Uint32(data[16:20]), control&securityDescriptorDACLPresent != 0, false)
	if err != nil {
		return nil, fmt.Errorf("dacl: %w", err)
	}

	d := &Descriptor{
		Owner:                  owner,
		Group:                  group,
		controlFlags:           control,
		resourceManagerControl: data[1],
	}
	if control&securityDescriptorSACLPresent != 0 {
		if sacl == nil {
			d.SACL = NullACL
		} else {
			sacl.Protected = control&securityDescriptorSACLProtected != 0
			sacl.AutoInheritRequested = control&securityDescriptorSACLAutoInheritRequested != 0
			sacl.AutoInherited = control&securityDescriptorSACLAutoInherited != 0
			d.SACL = sacl
		}
	}
	if control&securityDescriptorDACLPresent != 0 {
		if dacl == nil {
			d.DACL = NullACL
		} else {
			dacl.Protected = control&securityDescriptorDACLProtected != 0
			dacl.AutoInheritRequested = control&securityDescriptorDACLAutoInheritRequested != 0
			dacl.AutoInherited = control&securityDescriptorDACLAutoInherited != 0
			d.DACL = dacl
		}
	}
	if len(selection) > 0 {
		sel := selection[0]
		if sel&Owner == 0 {
			d.Owner = nil
		}
		if sel&Group == 0 {
			d.Group = nil
		}
		if sel&DACL == 0 {
			d.DACL = nil
		}
		if sel&SACL == 0 {
			d.SACL = nil
		}
	}
	return d, nil
}

func decodeSIDAt(data []byte, offset uint32) (*SID, error) {
	if offset == 0 {
		return nil, nil
	}
	if offset < 20 || offset&3 != 0 || uint64(offset)+8 > uint64(len(data)) {
		return nil, fmt.Errorf("invalid SID offset")
	}
	return decodeSID(data[offset:])
}

func decodeSID(data []byte) (*SID, error) {
	if len(data) < 8 {
		return nil, fmt.Errorf("invalid SID length")
	}
	revision := data[0]
	subAuthorityCount := int(data[1])
	if revision != 1 || subAuthorityCount > 15 {
		return nil, fmt.Errorf("invalid SID header")
	}
	if len(data) < 8+4*subAuthorityCount {
		return nil, fmt.Errorf("truncated SID")
	}
	authority := uint64(data[2])<<40 |
		uint64(data[3])<<32 |
		uint64(data[4])<<24 |
		uint64(data[5])<<16 |
		uint64(data[6])<<8 |
		uint64(data[7])

	subAuthorities := make([]uint32, subAuthorityCount)
	off := 8
	for i := range subAuthorities {
		subAuthorities[i] = binary.LittleEndian.Uint32(data[off : off+4])
		off += 4
	}
	return &SID{
		Revision:            revision,
		IdentifierAuthority: authority,
		SubAuthority:        subAuthorities,
	}, nil
}

func decodeACLAt(data []byte, offset uint32, present, sacl bool) (*ACL, error) {
	if offset == 0 {
		return nil, nil
	}
	if !present || offset < 20 || offset&3 != 0 || uint64(offset)+8 > uint64(len(data)) {
		return nil, fmt.Errorf("invalid ACL offset")
	}
	aclData := data[offset:]
	aclSize := uint64(binary.LittleEndian.Uint16(aclData[2:4]))
	if aclSize < 8 || aclSize > uint64(len(aclData)) {
		return nil, fmt.Errorf("invalid ACL size")
	}
	aclData = aclData[:aclSize]
	if aclData[0] != aclRevision && aclData[0] != aclRevisionDS || aclData[1] != 0 || binary.LittleEndian.Uint16(aclData[6:8]) != 0 {
		return nil, fmt.Errorf("invalid ACL header")
	}
	count := uint64(binary.LittleEndian.Uint16(aclData[4:6]))
	if count > (aclSize-8)/4 {
		return nil, fmt.Errorf("ACE count exceeds ACL size")
	}
	acl := &ACL{Revision: aclData[0], ACEs: make([]ACE, 0, count)}
	off := uint64(8)
	for range count {
		if off+4 > aclSize {
			return nil, fmt.Errorf("truncated ACE header")
		}
		aceSize := uint64(binary.LittleEndian.Uint16(aclData[off+2 : off+4]))
		if aceSize < 4 || aceSize&3 != 0 || off+aceSize > aclSize {
			return nil, fmt.Errorf("invalid ACE size")
		}
		aceData := aclData[off : off+aceSize]
		ace := ACE{Type: ACEType(aceData[0]), Flags: ACEFlags(aceData[1])}
		switch ace.Type {
		case AccessAllowed, AccessDenied, systemAudit, systemMandatoryLabel, systemScopedPolicyID:
			if aceSize < 8 {
				return nil, fmt.Errorf("truncated structured ACE")
			}
			ace.Mask = AccessMask(binary.LittleEndian.Uint32(aceData[4:8]))
			sid, err := decodeSID(aceData[8:])
			if err != nil {
				return nil, fmt.Errorf("invalid structured ACE SID")
			}
			// MS-DTYP 2.4.4.1 requires ignoring additional data within
			// non-callback ACEs. decodeSID already checks the SID bounds.
			ace.SID = sid
		default:
			if err := validateRawACEBody(aceData); err != nil {
				return nil, err
			}
			ace.Raw = append([]byte(nil), aceData...)
		}
		acl.ACEs = append(acl.ACEs, ace)
		off += aceSize
	}
	if off != aclSize {
		return nil, fmt.Errorf("ACL contains unparsed data")
	}
	if _, err := acl.validate(sacl); err != nil {
		return nil, err
	}
	return acl, nil
}
