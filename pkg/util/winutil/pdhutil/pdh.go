// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.
//go:build windows

package pdhutil

import (
	"unsafe"

	"golang.org/x/sys/windows"
)

type (
	PDH_HQUERY   windows.Handle
	PDH_HCOUNTER windows.Handle
)

// PDH error codes.  Taken from latest PDHMSH.h in Windows SDK
const (

	//
	// Event Descriptors
	//
	PDH_CSTATUS_VALID_DATA                     = 0x0
	PDH_CSTATUS_NEW_DATA                       = 0x1
	PDH_CSTATUS_NO_MACHINE                     = 0x800007D0
	PDH_CSTATUS_NO_INSTANCE                    = 0x800007D1
	PDH_MORE_DATA                              = 0x800007D2
	PDH_CSTATUS_ITEM_NOT_VALIDATED             = 0x800007D3
	PDH_RETRY                                  = 0x800007D4
	PDH_NO_DATA                                = 0x800007D5
	PDH_CALC_NEGATIVE_DENOMINATOR              = 0x800007D6
	PDH_CALC_NEGATIVE_TIMEBASE                 = 0x800007D7
	PDH_CALC_NEGATIVE_VALUE                    = 0x800007D8
	PDH_DIALOG_CANCELLED                       = 0x800007D9
	PDH_END_OF_LOG_FILE                        = 0x800007DA
	PDH_ASYNC_QUERY_TIMEOUT                    = 0x800007DB
	PDH_CANNOT_SET_DEFAULT_REALTIME_DATASOURCE = 0x800007DC
	PDH_UNABLE_MAP_NAME_FILES                  = 0x80000BD5
	PDH_PLA_VALIDATION_WARNING                 = 0x80000BF3
	PDH_CSTATUS_NO_OBJECT                      = 0xC0000BB8
	PDH_CSTATUS_NO_COUNTER                     = 0xC0000BB9
	PDH_CSTATUS_INVALID_DATA                   = 0xC0000BBA
	PDH_MEMORY_ALLOCATION_FAILURE              = 0xC0000BBB
	PDH_INVALID_HANDLE                         = 0xC0000BBC
	PDH_INVALID_ARGUMENT                       = 0xC0000BBD
	PDH_FUNCTION_NOT_FOUND                     = 0xC0000BBE
	PDH_CSTATUS_NO_COUNTERNAME                 = 0xC0000BBF
	PDH_CSTATUS_BAD_COUNTERNAME                = 0xC0000BC0
	PDH_INVALID_BUFFER                         = 0xC0000BC1
	PDH_INSUFFICIENT_BUFFER                    = 0xC0000BC2
	PDH_CANNOT_CONNECT_MACHINE                 = 0xC0000BC3
	PDH_INVALID_PATH                           = 0xC0000BC4
	PDH_INVALID_INSTANCE                       = 0xC0000BC5
	PDH_INVALID_DATA                           = 0xC0000BC6
	PDH_NO_DIALOG_DATA                         = 0xC0000BC7
	PDH_CANNOT_READ_NAME_STRINGS               = 0xC0000BC8
	PDH_LOG_FILE_CREATE_ERROR                  = 0xC0000BC9
	PDH_LOG_FILE_OPEN_ERROR                    = 0xC0000BCA
	PDH_LOG_TYPE_NOT_FOUND                     = 0xC0000BCB
	PDH_NO_MORE_DATA                           = 0xC0000BCC
	PDH_ENTRY_NOT_IN_LOG_FILE                  = 0xC0000BCD
	PDH_DATA_SOURCE_IS_LOG_FILE                = 0xC0000BCE
	PDH_DATA_SOURCE_IS_REAL_TIME               = 0xC0000BCF
	PDH_UNABLE_READ_LOG_HEADER                 = 0xC0000BD0
	PDH_FILE_NOT_FOUND                         = 0xC0000BD1
	PDH_FILE_ALREADY_EXISTS                    = 0xC0000BD2
	PDH_NOT_IMPLEMENTED                        = 0xC0000BD3
	PDH_STRING_NOT_FOUND                       = 0xC0000BD4
	PDH_UNKNOWN_LOG_FORMAT                     = 0xC0000BD6
	PDH_UNKNOWN_LOGSVC_COMMAND                 = 0xC0000BD7
	PDH_LOGSVC_QUERY_NOT_FOUND                 = 0xC0000BD8
	PDH_LOGSVC_NOT_OPENED                      = 0xC0000BD9
	PDH_WBEM_ERROR                             = 0xC0000BDA
	PDH_ACCESS_DENIED                          = 0xC0000BDB
	PDH_LOG_FILE_TOO_SMALL                     = 0xC0000BDC
	PDH_INVALID_DATASOURCE                     = 0xC0000BDD
	PDH_INVALID_SQLDB                          = 0xC0000BDE
	PDH_NO_COUNTERS                            = 0xC0000BDF
	PDH_SQL_ALLOC_FAILED                       = 0xC0000BE0
	PDH_SQL_ALLOCCON_FAILED                    = 0xC0000BE1
	PDH_SQL_EXEC_DIRECT_FAILED                 = 0xC0000BE2
	PDH_SQL_FETCH_FAILED                       = 0xC0000BE3
	PDH_SQL_ROWCOUNT_FAILED                    = 0xC0000BE4
	PDH_SQL_MORE_RESULTS_FAILED                = 0xC0000BE5
	PDH_SQL_CONNECT_FAILED                     = 0xC0000BE6
	PDH_SQL_BIND_FAILED                        = 0xC0000BE7
	PDH_CANNOT_CONNECT_WMI_SERVER              = 0xC0000BE8
	PDH_PLA_COLLECTION_ALREADY_RUNNING         = 0xC0000BE9
	PDH_PLA_ERROR_SCHEDULE_OVERLAP             = 0xC0000BEA
	PDH_PLA_COLLECTION_NOT_FOUND               = 0xC0000BEB
	PDH_PLA_ERROR_SCHEDULE_ELAPSED             = 0xC0000BEC
	PDH_PLA_ERROR_NOSTART                      = 0xC0000BED
	PDH_PLA_ERROR_ALREADY_EXISTS               = 0xC0000BEE
	PDH_PLA_ERROR_TYPE_MISMATCH                = 0xC0000BEF
	PDH_PLA_ERROR_FILEPATH                     = 0xC0000BF0
	PDH_PLA_SERVICE_ERROR                      = 0xC0000BF1
	PDH_PLA_VALIDATION_ERROR                   = 0xC0000BF2
	PDH_PLA_ERROR_NAME_TOO_LONG                = 0xC0000BF4
	PDH_INVALID_SQL_LOG_FORMAT                 = 0xC0000BF5
	PDH_COUNTER_ALREADY_IN_QUERY               = 0xC0000BF6
	PDH_BINARY_LOG_CORRUPT                     = 0xC0000BF7
	PDH_LOG_SAMPLE_TOO_SMALL                   = 0xC0000BF8
	PDH_OS_LATER_VERSION                       = 0xC0000BF9
	PDH_OS_EARLIER_VERSION                     = 0xC0000BFA
	PDH_INCORRECT_APPEND_TIME                  = 0xC0000BFB
	PDH_UNMATCHED_APPEND_COUNTER               = 0xC0000BFC
	PDH_SQL_ALTER_DETAIL_FAILED                = 0xC0000BFD
	PDH_QUERY_PERF_DATA_TIMEOUT                = 0xC0000BFE
	MSG_Publisher_Name                         = 0x90000001
)

// dwFormat flag values
// taken from latest pdh.h in windows sdk
const (
	PDH_FMT_RAW          = uint32(0x00000010)
	PDH_FMT_ANSI         = uint32(0x00000020)
	PDH_FMT_UNICODE      = uint32(0x00000040)
	PDH_FMT_LONG         = uint32(0x00000100)
	PDH_FMT_DOUBLE       = uint32(0x00000200)
	PDH_FMT_LARGE        = uint32(0x00000400)
	PDH_FMT_NOSCALE      = uint32(0x00001000)
	PDH_FMT_1000         = uint32(0x00002000)
	PDH_FMT_NODATA       = uint32(0x00004000)
	PDH_FMT_NOCAP100     = uint32(0x00008000)
	PERF_DETAIL_COSTLY   = uint32(0x00010000)
	PERF_DETAIL_STANDARD = uint32(0x0000FFFF)
)
const (
	ERROR_SUCCESS = 0
)

const (
	CounterAllProcessPctProcessorTime   = `\Process(*)\% Processor Time`
	CounterAllProcessPctUserTime        = `\Process(*)\% User Time`
	CounterAllProcessPctPrivilegedTime  = `\Process(*)\% Privileged Time`
	CounterAllProcessVirtualBytesPeak   = `\Process(*)\Virtual Bytes Peak`
	CounterAllProcessVirtualBytes       = `\Process(*)\Virtual Bytes`
	CounterAllProcessPageFaultsPerSec   = `\Process(*)\Page Faults/sec`
	CounterAllProcessWorkingSetPeak     = `\Process(*)\Working Set Peak`
	CounterAllProcessWorkingSet         = `\Process(*)\Working Set`
	CounterAllProcessPageFileBytesPeak  = `\Process(*)\Page File Bytes Peak`
	CounterAllProcessPageFileBytes      = `\Process(*)\Page File Bytes`
	CounterAllProcessPrivateBytes       = `\Process(*)\Private Bytes`
	CounterAllProcessThreadCount        = `\Process(*)\Thread Count`
	CounterAllProcessPriorityBase       = `\Process(*)\Priority Base`
	CounterAllProcessElapsedTime        = `\Process(*)\Elapsed Time`
	CounterAllProcessPID                = `\Process(*)\ID Process`
	CounterAllProcessParentPID          = `\Process(*)\Creating Process ID`
	CounterAllProcessPoolPagedBytes     = `\Process(*)\Pool Paged Bytes`
	CounterAllProcessPoolNonpagedBytes  = `\Process(*)\Pool Nonpaged Bytes`
	CounterAllProcessHandleCount        = `\Process(*)\Handle Count`
	CounterAllProcessIOReadOpsPerSec    = `\Process(*)\IO Read Operations/sec`
	CounterAllProcessIOWriteOpsPerSec   = `\Process(*)\IO Write Operations/sec`
	CounterAllProcessIODataOpsPerSec    = `\Process(*)\IO Data Operations/sec`
	CounterAllProcessIOOtherOpsPerSec   = `\Process(*)\IO Other Operations/sec`
	CounterAllProcessIOReadBytesPerSec  = `\Process(*)\IO Read Bytes/sec`
	CounterAllProcessIOWriteBytesPerSec = `\Process(*)\IO Write Bytes/sec`
	CounterAllProcessIODataBytesPerSec  = `\Process(*)\IO Data Bytes/sec`
	CounterAllProcessIOOtherBytesPerSec = `\Process(*)\IO Other Bytes/sec`
	CounterAllProcessWorkingSetPrivate  = `\Process(*)\Working Set - Private`
)

// PDH_FMT_COUNTERVALUE_ITEM_LONG structure contains the instance name and formatted value of a PDH_FMT_COUNTERVALUE_LONG counter.
type PDH_FMT_COUNTERVALUE_ITEM_LONG struct {
	szName *uint8
	value  PDH_FMT_COUNTERVALUE_LONG
}

// PDH_FMT_COUNTERVALUE_ITEM_LARGE structure contains the instance name and formatted value of a PDH_FMT_COUNTERVALUE_LARGE counter.
type PDH_FMT_COUNTERVALUE_ITEM_LARGE struct {
	szName *uint8
	value  PDH_FMT_COUNTERVALUE_LARGE
}

// PDH_FMT_COUNTERVALUE_ITEM_DOUBLE structure contains the instance name and formatted value of a PDH_FMT_COUNTERVALUE_DOUBLE counter.
type PDH_FMT_COUNTERVALUE_ITEM_DOUBLE struct {
	szName *uint8
	value  PDH_FMT_COUNTERVALUE_DOUBLE
}

// PdhOpenQuery Creates a new query that is used to manage the collection of performance data.
/*
Parameters
szDataSource [in]
Null-terminated string that specifies the name of the log file from which to retrieve performance data. If NULL, performance data is collected from a real-time data source.

dwUserData [in]
User-defined value to associate with this query. To retrieve the user data later, call PdhGetCounterInfo and access the dwQueryUserData member of PDH_COUNTER_INFO.

phQuery [out]
Handle to the query. You use this handle in subsequent calls.
*/
func PdhOpenQuery(szDataSource uintptr, dwUserData uintptr, phQuery *PDH_HQUERY) uint32 {
	ret, _, _ := procPdhOpenQuery.Call(
		szDataSource,
		dwUserData,
		uintptr(unsafe.Pointer(phQuery)))

	return uint32(ret)
}

// PdhAddEnglishCounter adds the specified counter to the query
/*
Parameters
hQuery [in]
Handle to the query to which you want to add the counter. This handle is returned by the PdhOpenQuery function.
szFullCounterPath [in]
Null-terminated string that contains the counter path. For details on the format of a counter path, see Specifying a Counter Path. The maximum length of a counter path is PDH_MAX_COUNTER_PATH.
dwUserData [in]
User-defined value. This value becomes part of the counter information. To retrieve this value later, call the PdhGetCounterInfo function and access the dwUserData member of the PDH_COUNTER_INFO structure.
phCounter [out]
Handle to the counter that was added to the query. You may need to reference this handle in subsequent calls.
*/
func PdhAddEnglishCounter(hQuery PDH_HQUERY, szFullCounterPath string, dwUserData uintptr, phCounter *PDH_HCOUNTER) uint32 {
	ptxt, _ := windows.UTF16PtrFromString(szFullCounterPath)
	ret, _, _ := procPdhAddEnglishCounterW.Call(
		uintptr(hQuery),
		uintptr(unsafe.Pointer(ptxt)),
		dwUserData,
		uintptr(unsafe.Pointer(phCounter)))

	return uint32(ret)
}

/*
	pdhCollectQueryData
	  Collects the current raw data value for all counters in the specified query and updates the status code of each counter.

Parameters
hQuery [in, out]
Handle of the query for which you want to collect data. The PdhOpenQuery function returns this handle.
*/
func pdhCollectQueryData(hQuery PDH_HQUERY) uint32 {
	ret, _, _ := procPdhCollectQueryData.Call(uintptr(hQuery))

	return uint32(ret)
}

// PdhCloseQuery Closes all counters contained in the specified query, closes all handles related to the query, and frees all memory associated with the query.
func PdhCloseQuery(hQuery PDH_HQUERY) uint32 {
	ret, _, _ := procPdhCloseQuery.Call(uintptr(hQuery))

	return uint32(ret)
}

// PdhRemoveCounter removes a counter from a query
func PdhRemoveCounter(hCounter PDH_HCOUNTER) uint32 {
	ret, _, _ := procPdhRemoveCounter.Call(uintptr(hCounter))
	return uint32(ret)
}
