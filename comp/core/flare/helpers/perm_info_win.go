// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

package helpers

import (
	"fmt"
	"os"
	"path"
	"strings"
	"unsafe"

	"github.com/DataDog/datadog-agent/pkg/util/winutil"
	"golang.org/x/sys/windows"
)

// filePermsInfo represents file rights on windows.
type aclInfo struct {
	userName      string
	deny          string
	aceFlags      string
	accessMask    string
	aceFlagsNum   uint8
	accessMaskNum uint32
	err           error
}

type filePermsInfo struct {
	path string
	mode string
	acls []*aclInfo
	err  error
}

const (
	DD_FILE_READ            = (windows.FILE_READ_DATA | windows.FILE_READ_ATTRIBUTES | windows.FILE_READ_EA)
	DD_FILE_WRITE           = (windows.FILE_WRITE_DATA | windows.FILE_WRITE_ATTRIBUTES | windows.FILE_WRITE_EA | windows.FILE_APPEND_DATA)
	DD_FILE_READ_EXEC       = (DD_FILE_READ | windows.FILE_EXECUTE)
	DD_FILE_READ_EXEC_WRITE = (DD_FILE_READ_EXEC | DD_FILE_WRITE)
	DD_FILE_MODIFY          = (DD_FILE_READ_EXEC_WRITE | windows.DELETE)
	DD_FILE_FULL            = (windows.STANDARD_RIGHTS_REQUIRED | windows.SYNCHRONIZE | 0x1FF)
)

func getFileDacl(fileName string) (*winutil.Acl, winutil.AclSizeInformation, error) {
	var aclInfo winutil.AclSizeInformation
	var fileDacl *winutil.Acl
	err := winutil.GetNamedSecurityInfo(fileName,
		winutil.SE_FILE_OBJECT,
		winutil.DACL_SECURITY_INFORMATION,
		nil,
		nil,
		&fileDacl,
		nil,
		nil)
	if err != nil {
		return nil, aclInfo, fmt.Errorf("cannot get security info for file '%s', error `%s`", fileName, err)
	}

	err = winutil.GetAclInformation(fileDacl, &aclInfo, winutil.AclSizeInformationEnum)
	if err != nil {
		return nil, aclInfo, fmt.Errorf("cannot get acl info for file '%s', error `%s`", fileName, err)
	}

	return fileDacl, aclInfo, err
}

// Currently we are supporting most common ACE types (denied and allowed)
// more could be added in the future
//
//	https://learn.microsoft.com/en-us/windows/win32/api/winnt/ns-winnt-ace_header
//	https://learn.microsoft.com/en-us/windows/win32/secauthz/ace
//
// ACCESS_ALLOWED_ACE and ACCESS_DENIED_ACE are the same and we can use winutil.AccessAllowedAce
// and quickly bailout if other ACE type is used. We may extend it in the future
func getAce(fileName string, acl *winutil.Acl, idx uint32) (*winutil.AccessAllowedAce, error) {
	var ace *winutil.AccessAllowedAce
	if err := winutil.GetAce(acl, idx, &ace); err != nil {
		return nil, fmt.Errorf("could not query a ACE on '%s': %s", fileName, err)
	}

	if ace.AceType != winutil.ACCESS_DENIED_ACE_TYPE &&
		ace.AceType != winutil.ACCESS_ALLOWED_ACE_TYPE {
		return nil, fmt.Errorf("unsupported AceType '%s': %x", fileName, ace.AceType)
	}

	return ace, nil
}

func sidToUserName(sid *windows.SID) string {
	user, domain, _, err := sid.LookupAccount("")
	if err == nil {
		if len(domain) > 0 {
			return fmt.Sprintf("%s\\%s", domain, user)
		}

		return user
	}

	return sid.String()
}

// This function attempts to generate the same rights abbreviation as built-in icacls.exe (when
// icacls.exe runs without arguments it will generate abbreviations legend). Compatibility is
// not complete, e.g. no attempts to interpret directory rights, however numeric values of mask
// are reported. More details can be found here
//
//	https://learn.microsoft.com/en-us/windows-server/administration/windows-commands/icacls
func accessMaskToStr(m uint32) string {
	if (m & DD_FILE_FULL) == DD_FILE_FULL {
		return "(F)"
	}
	if (m & DD_FILE_MODIFY) == DD_FILE_MODIFY {
		return "(M)"
	}
	if (m & DD_FILE_READ_EXEC_WRITE) == DD_FILE_READ_EXEC_WRITE {
		return "(RX,W)"
	}
	if (m & DD_FILE_READ_EXEC) == DD_FILE_READ_EXEC {
		return "(RX)"
	}
	if (m & DD_FILE_WRITE) == DD_FILE_WRITE {
		return "(W)"
	}
	if m == windows.GENERIC_ALL {
		return "(F)"
	}
	if m&(windows.GENERIC_READ|windows.GENERIC_WRITE|windows.GENERIC_EXECUTE) != 0 {
		rights := make([]string, 0, 3)
		if (m & windows.GENERIC_READ) == windows.GENERIC_READ {
			rights = append(rights, "GR")
		}
		if (m & windows.GENERIC_WRITE) == windows.GENERIC_WRITE {
			rights = append(rights, "GW")
		}
		if (m & windows.GENERIC_EXECUTE) == windows.GENERIC_EXECUTE {
			rights = append(rights, "GE")
		}
		return fmt.Sprintf("(%s)", strings.Join(rights, ","))
	}

	return ""
}

// This function attempts to generate the same rights abbreviation as built-in icacls.exe (when
// icacls.exe runs without arguments it will generate abbreviations legend). Numeric values of flags
// are reported. More details can be found here
//
//	https://learn.microsoft.com/en-us/windows-server/administration/windows-commands/icacls
func accessFlagToStr(f uint8) string {
	if (f & windows.OBJECT_INHERIT_ACE) == windows.OBJECT_INHERIT_ACE {
		return "(OI)"
	}
	if (f & windows.CONTAINER_INHERIT_ACE) == windows.CONTAINER_INHERIT_ACE {
		return "(CI)"
	}
	if (f & windows.NO_PROPAGATE_INHERIT_ACE) == windows.NO_PROPAGATE_INHERIT_ACE {
		return "(NP)"
	}
	if (f & windows.INHERIT_ONLY_ACE) == windows.INHERIT_ONLY_ACE {
		return "(IO)"
	}
	if (f & windows.INHERITED_ACE) == windows.INHERITED_ACE {
		return "(I)"
	}

	return ""
}

func denyToStr(aceType uint8) string {
	if aceType == winutil.ACCESS_DENIED_ACE_TYPE {
		return "(DENY)"
	}

	return ""
}

func (p permissionsInfos) addAgentExeFiles() {
	// Get Datadog bin directory (optional if err)
	installDir, err := winutil.GetProgramFilesDirForProduct("DataDog Agent")
	if err == nil {
		p.add(path.Join(installDir, "bin", "agent.exe"))
		p.add(path.Join(installDir, "bin", "Agent", "ddtray.exe"))
		p.add(path.Join(installDir, "bin", "Agent", "process-agent.exe"))
		p.add(path.Join(installDir, "bin", "Agent", "system-probe.exe"))
		p.add(path.Join(installDir, "bin", "Agent", "trace-agent.exe"))
	}
}

func (p permissionsInfos) add(filePath string) {
	info := filePermsInfo{
		path: filePath,
	}
	p[filePath] = &info

	fi, err := os.Stat(filePath)
	if err != nil {
		info.err = fmt.Errorf("could not stat file %s. error: %s", filePath, err)
		return
	}
	info.mode = fi.Mode().String()

	fileDacl, aclSizeInfo, err := getFileDacl(info.path)
	if err != nil {
		info.err = err
		return
	}

	info.acls = make([]*aclInfo, 0, aclSizeInfo.AceCount)
	for i := uint32(0); i < aclSizeInfo.AceCount; i++ {
		acl := aclInfo{}
		info.acls = append(info.acls, &acl)

		ace, err := getAce(info.path, fileDacl, i)
		if err != nil {
			info.err = err
			continue
		}

		acl.userName = sidToUserName((*windows.SID)(unsafe.Pointer(&ace.SidStart)))
		acl.deny = denyToStr(ace.AceType)
		acl.aceFlags = accessFlagToStr(ace.AceFlags)
		acl.aceFlagsNum = ace.AceFlags
		acl.accessMask = accessMaskToStr(ace.AccessMask)
		acl.accessMaskNum = ace.AccessMask
	}
}

// commit resolves the infos of every stacked files in the map
// and then writes the permissions.log file on the filesystem.
func (p permissionsInfos) commit() ([]byte, error) {

	var sb strings.Builder

	// These files are not explicitly copied but their privileges are "interesting"
	p.addAgentExeFiles()

	for _, info := range p {
		sb.WriteString("File: ")
		sb.WriteString(info.path)
		sb.WriteString("\n------------------\n")
		if info.err != nil {
			sb.WriteString(info.err.Error())
			sb.WriteString("\n\n")
			continue
		}

		sb.WriteString("Mode: ")
		sb.WriteString(info.mode)
		sb.WriteString("\n")

		for _, acl := range info.acls {
			if acl.err != nil {
				sb.WriteString(acl.err.Error())
				sb.WriteString("\n")
				continue
			}

			sb.WriteString(fmt.Sprintf("%s: %s%s%s [flags:0x%x, mask:0x%x]\n",
				acl.userName, acl.deny, acl.aceFlags, acl.accessMask,
				acl.aceFlagsNum, acl.accessMaskNum))
		}

		sb.WriteString("\n")
	}

	return []byte(sb.String()), nil
}
