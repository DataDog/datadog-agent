// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package probe holds probe related files
package probe

import (
	"strings"

	"github.com/DataDog/datadog-agent/comp/etw"
	etwimpl "github.com/DataDog/datadog-agent/comp/etw/impl"
)

const (
	idProcessStart = uint16(1)
	idProcessStop  = uint16(2)
	idThreadStart  = uint16(3)
	idThreadStop   = uint16(4)
	idImageLoad    = uint16(5)
	idImageUnload  = uint16(6)

	idCpuBasePriorityChange = uint16(7)
	idCpuPriorityChange     = uint16(8)
	idPagePriorityChange    = uint16(9)
	idIoPriorityChange      = uint16(10)
	idProcessFreezeStart    = uint16(11)
	idProcessFreezeStop     = uint16(12)
	idJobStart              = uint16(13)
	idJobTerminateStop      = uint16(14)
	idProcessRundown        = uint16(15)
)

/*
  <template tid="ImageLoadArgs">
      <data name="ImageBase" inType="win:Pointer"/>
      <data name="ImageSize" inType="win:Pointer"/>
      <data name="ProcessID" inType="win:UInt32"/>
      <data name="ImageCheckSum" inType="win:UInt32"/>
      <data name="TimeDateStamp" inType="win:UInt32"/>
      <data name="DefaultBase" inType="win:Pointer"/>
      <data name="ImageName" inType="win:UnicodeString"/>
     </template>
*/

type imageLoadArgs struct {
	etw.DDEventHeader
	imageBase     fileObjectPointer
	ImageSize     fileObjectPointer
	processID     uint32
	imageCheckSum uint32
	timeDateStamp uint32
	defaultBase   fileObjectPointer
	imageName     string
}

func (wp *WindowsProbe) parseImageLoadArgs(e *etw.DDEventRecord) (*imageLoadArgs, error) {
	ila := &imageLoadArgs{
		DDEventHeader: e.EventHeader,
	}
	data := etwimpl.GetUserData(e)

	ila.imageBase = fileObjectPointer(data.GetUint64(0))
	ila.ImageSize = fileObjectPointer(data.GetUint64(8))
	ila.processID = data.GetUint32(16)
	ila.imageCheckSum = data.GetUint32(20)
	ila.timeDateStamp = data.GetUint32(24)
	ila.defaultBase = fileObjectPointer(data.GetUint64(28))
	ila.imageName, _, _, _ = data.ParseUnicodeString(36)

	return ila, nil
}

func (ila *imageLoadArgs) String() string {
	var output strings.Builder

	output.WriteString("Image Load: Name: " + ila.imageName + "\n")
	return output.String()
}
