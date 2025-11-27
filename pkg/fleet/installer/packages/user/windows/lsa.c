// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

#include <windows.h>
#include <ntstatus.h>
#include <ntsecapi.h>

// Retrieve private data from LSA
//
// https://learn.microsoft.com/en-us/windows/win32/api/ntsecapi/nf-ntsecapi-lsaretrieveprivatedata
NTSTATUS retrieve_private_data(const void* key, void** result, size_t* result_size) {
    NTSTATUS status = STATUS_UNSUCCESSFUL;
    WCHAR* key_copy = NULL;
    LSA_HANDLE lsa_handle = NULL;
    LSA_UNICODE_STRING lsa_key_name;
    PLSA_UNICODE_STRING lsa_secret_ptr = NULL;

    memset(&lsa_key_name, 0, sizeof(LSA_UNICODE_STRING));

    if (!key) {
        status = STATUS_INVALID_PARAMETER_1;
        goto done;
    }
    if (!result) {
        status = STATUS_INVALID_PARAMETER_2;
        goto done;
    }
    if (!result_size) {
        status = STATUS_INVALID_PARAMETER_3;
        goto done;
    }

    *result = NULL;
    *result_size = 0;

    // Duplicate the key to ensure we have a non-const copy
    key_copy = _wcsdup((const WCHAR*)key);
    if (!key_copy) {
        status = STATUS_NO_MEMORY;
        goto done;
    }

    LSA_OBJECT_ATTRIBUTES object_attributes;
    memset(&object_attributes, 0, sizeof(LSA_OBJECT_ATTRIBUTES));

    // Open LSA policy
    status = LsaOpenPolicy(NULL, &object_attributes, POLICY_GET_PRIVATE_INFORMATION, &lsa_handle);
    if (status != STATUS_SUCCESS) {
        goto done;
    }

    // Create LSA unicode string for the key
    lsa_key_name.Buffer = key_copy;
    // Specifies the length, in bytes, of the string pointed to by the Buffer member, not including the terminating null character, if any
    // https://learn.microsoft.com/en-us/windows/win32/api/lsalookup/ns-lsalookup-lsa_unicode_string
    lsa_key_name.Length = wcslen(key_copy) * sizeof(WCHAR);
    lsa_key_name.MaximumLength = lsa_key_name.Length + sizeof(WCHAR);

    // Retrieve private data
    status = LsaRetrievePrivateData(lsa_handle, &lsa_key_name, &lsa_secret_ptr);
    if (status != STATUS_SUCCESS) {
        goto done;
    }

    if (!lsa_secret_ptr || !lsa_secret_ptr->Buffer || lsa_secret_ptr->Length == 0) {
        // We expect STATUS_OBJECT_NAME_NOT_FOUND if the key is not found,
        // but we'll treat this unexpected case same as if it was an empty string.
        *result = NULL;
        *result_size = 0;
        status = STATUS_SUCCESS;
        goto done;
    }

    // Create a copy of the string because it may not be null-terminated and
    // UTF16PtrToString expects a null-terminated string.

    // Allocate memory for null-terminated string (lengths in bytes)
    USHORT lengthWithNullTerminator = lsa_secret_ptr->Length + sizeof(WCHAR);
    unsigned char* output = (unsigned char*)calloc(lengthWithNullTerminator, sizeof(unsigned char));
    if (!output) {
        status = STATUS_NO_MEMORY;
        goto done;
    }

    // Copy the string
    memcpy(output, lsa_secret_ptr->Buffer, lsa_secret_ptr->Length);
    *result = output;
    *result_size = lengthWithNullTerminator;
    status = STATUS_SUCCESS;

done:
    if (key_copy) {
        free(key_copy);
        key_copy = NULL;
    }
    if (lsa_secret_ptr) {
        // Clear the buffer to avoid leaking sensitive data
        if (lsa_secret_ptr->Buffer && lsa_secret_ptr->Length > 0) {
            memset(lsa_secret_ptr->Buffer, 0, lsa_secret_ptr->Length);
        }
        LsaFreeMemory(lsa_secret_ptr);
        lsa_secret_ptr = NULL;
    }
    if (lsa_handle) {
        LsaClose(lsa_handle);
        lsa_handle = NULL;
    }
    return status;
}

// Free result returned by retrieve_private_data
void free_private_data(void* result, size_t result_size) {
    if (result) {
        // Clear the buffer to avoid leaking sensitive data
        memset(result, 0, result_size);
        free(result);
    }
}
