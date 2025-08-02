// This file is licensed under the MIT License.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build windows

#include <windows.h>
#include <lm.h>
#include <stdlib.h>
#include <ntsecapi.h>

// Custom error codes for when DLLs are not available
#define ERROR_DLL_NOT_AVAILABLE 0x80070002L // ERROR_FILE_NOT_FOUND
#define ERROR_FUNCTION_NOT_AVAILABLE 0x80070127L // ERROR_PROC_NOT_FOUND

// Function pointer types for netapi32.dll
typedef NET_API_STATUS (WINAPI *NetUserGetLocalGroupsFunc)(
    LPCWSTR servername,
    LPCWSTR username,
    DWORD level,
    DWORD flags,
    LPBYTE *bufptr,
    DWORD prefmaxlen,
    LPDWORD entriesread,
    LPDWORD totalentries
);

typedef NET_API_STATUS (WINAPI *NetApiBufferFreeFunc)(LPVOID Buffer);

// Function pointer types for advapi32.dll
typedef BOOL (WINAPI *LookupAccountNameWFunc)(
    LPCWSTR lpSystemName,
    LPCWSTR lpAccountName,
    PSID Sid,
    LPDWORD cbSid,
    LPWSTR ReferencedDomainName,
    LPDWORD cchReferencedDomainName,
    PSID_NAME_USE peUse
);

typedef NTSTATUS (WINAPI *LsaOpenPolicyFunc)(
    PLSA_UNICODE_STRING SystemName,
    PLSA_OBJECT_ATTRIBUTES ObjectAttributes,
    ACCESS_MASK DesiredAccess,
    PLSA_HANDLE PolicyHandle
);

typedef NTSTATUS (WINAPI *LsaEnumerateAccountRightsFunc)(
    LSA_HANDLE PolicyHandle,
    PSID AccountSid,
    PLSA_UNICODE_STRING *UserRights,
    PULONG CountOfRights
);

typedef NTSTATUS (WINAPI *LsaFreeMemoryFunc)(PVOID Buffer);

typedef NTSTATUS (WINAPI *LsaCloseFunc)(LSA_HANDLE ObjectHandle);

// Global function pointers
static NetUserGetLocalGroupsFunc pNetUserGetLocalGroups = NULL;
static NetApiBufferFreeFunc pNetApiBufferFree = NULL;
static LookupAccountNameWFunc pLookupAccountNameW = NULL;
static LsaOpenPolicyFunc pLsaOpenPolicy = NULL;
static LsaEnumerateAccountRightsFunc pLsaEnumerateAccountRights = NULL;
static LsaFreeMemoryFunc pLsaFreeMemory = NULL;
static LsaCloseFunc pLsaClose = NULL;

// DLL handles
static HMODULE hNetapi32 = NULL;
static HMODULE hAdvapi32 = NULL;

// Initialization flags
static BOOL netapi32_initialized = FALSE;
static BOOL advapi32_initialized = FALSE;
static BOOL netapi32_available = FALSE;
static BOOL advapi32_available = FALSE;

// Simple initialization - no complex thread safety needed for diagnostic functions
// Initialize netapi32.dll function pointers
static BOOL initNetApi32() {
    if (netapi32_initialized) {
        return netapi32_available;
    }
    
    hNetapi32 = LoadLibraryW(L"netapi32.dll");
    if (!hNetapi32) {
        netapi32_available = FALSE;
        netapi32_initialized = TRUE;
        return FALSE;
    }
    
    pNetUserGetLocalGroups = (NetUserGetLocalGroupsFunc)GetProcAddress(hNetapi32, "NetUserGetLocalGroups");
    pNetApiBufferFree = (NetApiBufferFreeFunc)GetProcAddress(hNetapi32, "NetApiBufferFree");
    
    if (!pNetUserGetLocalGroups || !pNetApiBufferFree) {
        FreeLibrary(hNetapi32);
        hNetapi32 = NULL;
        netapi32_available = FALSE;
        netapi32_initialized = TRUE;
        return FALSE;
    }
    
    netapi32_available = TRUE;
    netapi32_initialized = TRUE;
    return TRUE;
}

// Initialize advapi32.dll function pointers
static BOOL initAdvApi32() {
    if (advapi32_initialized) {
        return advapi32_available;
    }
    
    hAdvapi32 = LoadLibraryW(L"advapi32.dll");
    if (!hAdvapi32) {
        advapi32_available = FALSE;
        advapi32_initialized = TRUE;
        return FALSE;
    }
    
    pLookupAccountNameW = (LookupAccountNameWFunc)GetProcAddress(hAdvapi32, "LookupAccountNameW");
    pLsaOpenPolicy = (LsaOpenPolicyFunc)GetProcAddress(hAdvapi32, "LsaOpenPolicy");
    pLsaEnumerateAccountRights = (LsaEnumerateAccountRightsFunc)GetProcAddress(hAdvapi32, "LsaEnumerateAccountRights");
    pLsaFreeMemory = (LsaFreeMemoryFunc)GetProcAddress(hAdvapi32, "LsaFreeMemory");
    pLsaClose = (LsaCloseFunc)GetProcAddress(hAdvapi32, "LsaClose");
    
    if (!pLookupAccountNameW || !pLsaOpenPolicy || !pLsaEnumerateAccountRights || 
        !pLsaFreeMemory || !pLsaClose) {
        FreeLibrary(hAdvapi32);
        hAdvapi32 = NULL;
        advapi32_available = FALSE;
        advapi32_initialized = TRUE;
        return FALSE;
    }
    
    advapi32_available = TRUE;
    advapi32_initialized = TRUE;
    return TRUE;
}

// Helper functions to check API availability
// Returns TRUE if netapi32.dll and required functions are available
BOOL isNetApi32Available() {
    return initNetApi32();
}

// Returns TRUE if advapi32.dll and required functions are available
BOOL isAdvApi32Available() {
    return initAdvApi32();
}

// Cleanup function to free loaded DLLs
// Should be called when the module is unloaded
void cleanupWinUtilLibraries() {
    // Clean up netapi32
    if (hNetapi32) {
        FreeLibrary(hNetapi32);
        hNetapi32 = NULL;
        pNetUserGetLocalGroups = NULL;
        pNetApiBufferFree = NULL;
        netapi32_initialized = FALSE;
        netapi32_available = FALSE;
    }
    
    // Clean up advapi32
    if (hAdvapi32) {
        FreeLibrary(hAdvapi32);
        hAdvapi32 = NULL;
        pLookupAccountNameW = NULL;
        pLsaOpenPolicy = NULL;
        pLsaEnumerateAccountRights = NULL;
        pLsaFreeMemory = NULL;
        pLsaClose = NULL;
        advapi32_initialized = FALSE;
        advapi32_available = FALSE;
    }
}

// getLocalUserGroups retrieves local groups that a user (local or domain) belongs to
// Returns a comma-separated string of local group names, or NULL on error
// The returned string must be freed by the caller
char* getLocalUserGroups(const char* username, int* error_code) {
    LOCALGROUP_USERS_INFO_0 *local_groups = NULL;
    DWORD entries_read = 0, total_entries = 0;
    char* result = NULL;
    
    // Initialize error code to success
    if (error_code) {
        *error_code = 0;
    }
    
    // Check if netapi32.dll is available
    if (!initNetApi32()) {
        if (error_code) {
            *error_code = ERROR_DLL_NOT_AVAILABLE;
        }
        return NULL;
    }
    
    // Convert username to wide string
    int wlen = MultiByteToWideChar(CP_UTF8, 0, username, -1, NULL, 0);
    if (wlen == 0) {
        if (error_code) {
            *error_code = GetLastError();
        }
        return NULL;
    }
    
    wchar_t* wusername = (wchar_t*)malloc(wlen * sizeof(wchar_t));
    if (!wusername) {
        if (error_code) {
            *error_code = ERROR_NOT_ENOUGH_MEMORY;
        }
        return NULL;
    }
    
    if (MultiByteToWideChar(CP_UTF8, 0, username, -1, wusername, wlen) == 0) {
        if (error_code) {
            *error_code = GetLastError();
        }
        free(wusername);
        return NULL;
    }
    
    // Call NetUserGetLocalGroups via function pointer
    NET_API_STATUS status = pNetUserGetLocalGroups(
        NULL, wusername, 0, LG_INCLUDE_INDIRECT,
        (LPBYTE*)&local_groups, MAX_PREFERRED_LENGTH,
        &entries_read, &total_entries
    );
    
    free(wusername);
    
    if (status != NERR_Success) {
        if (error_code) {
            *error_code = status;
        }
        return NULL;
    }
    
    if (entries_read == 0 || !local_groups) {
        if (local_groups) {
            pNetApiBufferFree(local_groups);
        }
        // This is not really an error, just no groups found
        if (error_code) {
            *error_code = 0;
        }
        return NULL;
    }
    
    // Calculate total length needed for result string
    size_t total_len = 0;
    for (DWORD i = 0; i < entries_read; i++) {
        if (local_groups[i].lgrui0_name) {
            int len = WideCharToMultiByte(CP_UTF8, 0, local_groups[i].lgrui0_name, -1, NULL, 0, NULL, NULL);
            if (len > 0) {
                total_len += len - 1; // -1 because len includes null terminator
                if (i < entries_read - 1) {
                    total_len += 1; // for comma
                }
            }
        }
    }
    
    if (total_len == 0) {
        pNetApiBufferFree(local_groups);
        if (error_code) {
            *error_code = 0;
        }
        return NULL;
    }
    
    // Allocate result string
    result = (char*)malloc(total_len + 1);
    if (!result) {
        if (error_code) {
            *error_code = ERROR_NOT_ENOUGH_MEMORY;
        }
        pNetApiBufferFree(local_groups);
        return NULL;
    }
    
    // Build result string
    char* current = result;
    for (DWORD i = 0; i < entries_read; i++) {
        if (local_groups[i].lgrui0_name) {
            int len = WideCharToMultiByte(CP_UTF8, 0, local_groups[i].lgrui0_name, -1, current, total_len + 1 - (current - result), NULL, NULL);
            if (len > 0) {
                current += len - 1; // -1 because len includes null terminator
                if (i < entries_read - 1) {
                    *current = ',';
                    current++;
                }
            }
        }
    }
    *current = '\0';
    
    pNetApiBufferFree(local_groups);
    return result;
}

// getLocalAccountRights retrieves account rights for a user (local or domain) on the local system
// Returns a comma-separated string of account right names, or NULL on error
// The returned string must be freed by the caller
char* getLocalAccountRights(const char* username, int* error_code) {
    SID sid[SECURITY_MAX_SID_SIZE];
    DWORD sid_size = SECURITY_MAX_SID_SIZE;
    wchar_t domain_name[256];
    DWORD domain_size = 256;
    SID_NAME_USE sid_type;
    char* result = NULL;
    
    // Initialize error code to success
    if (error_code) {
        *error_code = 0;
    }
    
    // Check if advapi32.dll is available
    if (!initAdvApi32()) {
        if (error_code) {
            *error_code = ERROR_DLL_NOT_AVAILABLE;
        }
        return NULL;
    }
    
    // Convert username to wide string
    int wlen = MultiByteToWideChar(CP_UTF8, 0, username, -1, NULL, 0);
    if (wlen == 0) {
        if (error_code) {
            *error_code = GetLastError();
        }
        return NULL;
    }
    
    wchar_t* wusername = (wchar_t*)malloc(wlen * sizeof(wchar_t));
    if (!wusername) {
        if (error_code) {
            *error_code = ERROR_NOT_ENOUGH_MEMORY;
        }
        return NULL;
    }
    
    if (MultiByteToWideChar(CP_UTF8, 0, username, -1, wusername, wlen) == 0) {
        if (error_code) {
            *error_code = GetLastError();
        }
        free(wusername);
        return NULL;
    }
    
    // Step 1: Lookup SID via function pointer
    if (!pLookupAccountNameW(NULL, wusername, sid, &sid_size, domain_name, &domain_size, &sid_type)) {
        if (error_code) {
            *error_code = GetLastError();
        }
        free(wusername);
        return NULL;
    }
    
    free(wusername);
    
    // Step 2: Open LSA Policy via function pointer
    LSA_OBJECT_ATTRIBUTES object_attributes = {0};
    LSA_HANDLE policy_handle;
    
    NTSTATUS status = pLsaOpenPolicy(
        NULL,
        &object_attributes,
        POLICY_LOOKUP_NAMES | POLICY_VIEW_LOCAL_INFORMATION,
        &policy_handle
    );
    
    if (status != 0) {
        if (error_code) {
            *error_code = status;
        }
        return NULL;
    }
    
    // Step 3: Enumerate Rights via function pointer
    PLSA_UNICODE_STRING rights = NULL;
    ULONG count = 0;
    
    status = pLsaEnumerateAccountRights(policy_handle, sid, &rights, &count);
    
    if (status != 0) {
        if (error_code) {
            *error_code = status;
        }
        pLsaClose(policy_handle);
        return NULL;
    }
    
    if (rights && count > 0) {
        // Calculate total length needed for result string
        size_t total_len = 0;
        for (ULONG i = 0; i < count; i++) {
            if (rights[i].Buffer && rights[i].Length > 0) {
                int len = WideCharToMultiByte(CP_UTF8, 0, rights[i].Buffer, rights[i].Length / 2, NULL, 0, NULL, NULL);
                if (len > 0) {
                    total_len += len; // len is exact character count, no null terminator included
                    if (i < count - 1) {
                        total_len += 1; // for comma
                    }
                }
            }
        }
        
        if (total_len > 0) {
            // Allocate result string
            result = (char*)malloc(total_len + 1);
            if (result) {
                // Build result string
                char* current = result;
                for (ULONG i = 0; i < count; i++) {
                    if (rights[i].Buffer && rights[i].Length > 0) {
                        int len = WideCharToMultiByte(CP_UTF8, 0, rights[i].Buffer, rights[i].Length / 2, current, total_len + 1 - (current - result), NULL, NULL);
                        if (len > 0) {
                            current += len; // len is exact character count, no null terminator included
                            if (i < count - 1) {
                                *current = ',';
                                current++;
                            }
                        }
                    }
                }
                *current = '\0';
            } else {
                if (error_code) {
                    *error_code = ERROR_NOT_ENOUGH_MEMORY;
                }
            }
        }
        
        // Free the rights buffer via function pointer
        pLsaFreeMemory(rights);
    }
    
    // Close the policy handle via function pointer
    pLsaClose(policy_handle);
    
    return result;
}