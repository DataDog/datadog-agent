// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

#ifndef LSA_H
#define LSA_H

#include <windows.h>

// Retrieve private data from LSA
// Returns NTSTATUS, stores allocated string in result parameter and size in bytes
// Result must be freed with free_private_data() if function succeeds
NTSTATUS retrieve_private_data(const void* key, void** result, size_t* result_size);

// Free result returned by retrieve_private_data
void free_private_data(void* result, size_t result_size);

#endif // LSA_H 