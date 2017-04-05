package cpu

import "encoding/binary"

const SYSTEM_LOGICAL_PROCESSOR_INFORMATION_SIZE = 32

func getSystemLogicalProcessorInformationSize() int {
	return SYSTEM_LOGICAL_PROCESSOR_INFORMATION_SIZE
}
func byteArrayToProcessorStruct(data []byte) (info SYSTEM_LOGICAL_PROCESSOR_INFORMATION) {
	info.ProcessorMask = uintptr(binary.LittleEndian.Uint64(data))
	info.Relationship = int(binary.LittleEndian.Uint64(data[8:]))
	copy(info.dataunion[0:16], data[16:32])
	return
}
