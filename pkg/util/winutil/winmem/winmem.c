#include "winmem.h"

// Internal C callback that forwards to Go callback
static BOOL CALLBACK internalCallback(
    LPVOID pContext,
    PENUM_PAGE_FILE_INFORMATION pInfo,
    LPCWSTR lpFilename
) {
    // pContext contains the Go handle (as uintptr_t)
    GoHandle handle = (GoHandle)pContext;
    
    // Call back into Go, passing the handle
    // Go will convert the handle back to the actual Go value
    return PageFileCallback(handle, pInfo, lpFilename);
}

// Wrapper function that calls Windows API
// Returns ERROR_SUCCESS on success, or GetLastError() on failure
DWORD enumPageFilesWithHandle(GoHandle handle) {
    // Call Windows API with our C callback
    // Pass the Go handle as the context
    if (!EnumPageFilesW(internalCallback, (LPVOID)handle)) {
        return GetLastError();
    }
    return ERROR_SUCCESS;
}