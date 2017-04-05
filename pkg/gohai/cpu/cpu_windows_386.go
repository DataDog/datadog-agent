package cpu

import "encoding/binary"

const SYSTEM_LOGICAL_PROCESSOR_INFORMATION_SIZE = 24

func getSystemLogicalProcessorInformationSize() int {
	return SYSTEM_LOGICAL_PROCESSOR_INFORMATION_SIZE
}
func byteArrayToProcessorStruct(data []byte) (info SYSTEM_LOGICAL_PROCESSOR_INFORMATION) {
	info.ProcessorMask = uintptr(binary.LittleEndian.Uint32(data))
	info.Relationship = int(binary.LittleEndian.Uint32(data[4:]))
	copy(info.dataunion[0:16], data[8:24])
	return
}
