// This file is licensed under the MIT License.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build windows

#include <windows.h>
#include <lm.h>
#include <stdlib.h>
#include <ntsecapi.h>

// getLocalUserGroups retrieves local groups for a given username
// Returns a comma-separated string of group names, or NULL on error
// The returned string must be freed by the caller
char* getLocalUserGroups(const char* username, int* error_code) {
    LOCALGROUP_USERS_INFO_0 *local_groups = NULL;
    DWORD entries_read = 0, total_entries = 0;
    char* result = NULL;
    
    // Initialize error code to success
    if (error_code) {
        *error_code = 0;
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
    
    // Call NetUserGetLocalGroups
    NET_API_STATUS status = NetUserGetLocalGroups(
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
            NetApiBufferFree(local_groups);
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
        NetApiBufferFree(local_groups);
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
        NetApiBufferFree(local_groups);
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
    
    NetApiBufferFree(local_groups);
    return result;
}

// getLocalAccountRights retrieves account rights for a given username
// Returns a comma-separated string of right names, or NULL on error
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
    
    // Step 1: Lookup SID
    if (!LookupAccountNameW(NULL, wusername, sid, &sid_size, domain_name, &domain_size, &sid_type)) {
        if (error_code) {
            *error_code = GetLastError();
        }
        free(wusername);
        return NULL;
    }
    
    free(wusername);
    
    // Step 2: Open LSA Policy
    LSA_OBJECT_ATTRIBUTES object_attributes = {0};
    LSA_HANDLE policy_handle;
    
    NTSTATUS status = LsaOpenPolicy(
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
    
    // Step 3: Enumerate Rights
    PLSA_UNICODE_STRING rights = NULL;
    ULONG count = 0;
    
    status = LsaEnumerateAccountRights(policy_handle, sid, &rights, &count);
    
    if (status != 0) {
        if (error_code) {
            *error_code = status;
        }
        LsaClose(policy_handle);
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
        
        // Free the rights buffer
        LsaFreeMemory(rights);
    }
    
    // Close the policy handle
    LsaClose(policy_handle);
    
    return result;
}