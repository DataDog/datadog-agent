#ifndef WINMEM_H
#define WINMEM_H

#include <windows.h>
#include <psapi.h>
#include <stdint.h>

#ifdef __cplusplus
extern "C" {
#endif

// Type for Go callback handle (opaque integer)
typedef uintptr_t GoHandle;

// Forward declaration of Go callback function
// This will be implemented in Go and called from C
extern BOOL PageFileCallback(
    GoHandle handle,
    PENUM_PAGE_FILE_INFORMATION pInfo,
    LPCWSTR lpFilename
);

// C wrapper that calls EnumPageFilesW with our callback
// Returns ERROR_SUCCESS on success, or GetLastError() on failure
DWORD enumPageFilesWithHandle(GoHandle handle);

#ifdef __cplusplus
}
#endif

#endif // WINMEM_H