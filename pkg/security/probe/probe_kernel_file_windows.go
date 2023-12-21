// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package probe holds probe related files
package probe

import (
	etwutil "github.com/DataDog/datadog-agent/pkg/util/winutil/etw"
)

const (
	idNameCreate       = uint16(10)
	idNameDelete       = uint16(11)
	idCreate           = uint16(12)
	idCleanup          = uint16(13)
	idClose            = uint16(14)
	idRead             = uint16(15)
	idWrite            = uint16(16)
	idSetInformation   = uint16(17)
	idSetDelete        = uint16(18)
	idRename           = uint16(19)
	idDirEnum          = uint16(20)
	idFlush            = uint16(21)
	idQueryInformation = uint16(22)
	idFSCTL            = uint16(23)
	idOperationEnd     = uint16(24)
	idDirNotify        = uint16(25)
	idDeletePath       = uint16(26)
	idRenamePath       = uint16(27)
	idSetLinkPath      = uint16(28)
	idRename29         = uint16(29)
	idCreateNewFile    = uint16(30)
)

type createArgs struct {
	irp      uint64 // actually a pointer
	threadid uint64 // actually a pointer
	fileObject uint64 // pointer
	createOptions uint32 
	createAttributes uint32
	shareAccess uint32
	fileName string
}

func parseCreateArgs(data []byte) (*createArgs, error) {
	ca := &createArgs{}
	filenameOffset := 36
	// for now, just interested in path
	ca.fileName, _, _, _ = etwutil.ParseUnicodeString(data, filenameOffset)

	return ca, nil
}
