// This file is licensed under the MIT License.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build windows

#ifndef WINUTIL_USERS_H
#define WINUTIL_USERS_H

#include <windows.h>

#ifdef __cplusplus
extern "C" {
#endif

// Custom error codes for when DLLs are not available
#define ERROR_DLL_NOT_AVAILABLE 0x80070002L // ERROR_FILE_NOT_FOUND
#define ERROR_FUNCTION_NOT_AVAILABLE 0x80070127L // ERROR_PROC_NOT_FOUND

// Main functionality functions
char* getLocalUserGroups(const char* username, int* error_code);
char* getLocalAccountRights(const char* username, int* error_code);

// API availability check functions
BOOL isNetApi32Available();
BOOL isAdvApi32Available();

// Cleanup function
void cleanupWinUtilLibraries();

#ifdef __cplusplus
}
#endif

#endif // WINUTIL_USERS_H 