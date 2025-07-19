// This file is licensed under the MIT License.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build windows

#ifndef USERS_H_INCLUDED
#define USERS_H_INCLUDED

#include <stdlib.h>

#ifdef __cplusplus
extern "C" {
#endif

// getLocalUserGroups retrieves local groups for a given username
// Returns a comma-separated string of group names, or NULL on error
// The returned string must be freed by the caller
// error_code will be set to the specific error code if an error occurs (can be NULL if not needed)
char* getLocalUserGroups(const char* username, int* error_code);

// getLocalAccountRights retrieves account rights for a given username
// Returns a comma-separated string of right names, or NULL on error
// The returned string must be freed by the caller
// error_code will be set to the specific error code if an error occurs (can be NULL if not needed)
char* getLocalAccountRights(const char* username, int* error_code);

#ifdef __cplusplus
}
#endif

#endif // USERS_H_INCLUDED 