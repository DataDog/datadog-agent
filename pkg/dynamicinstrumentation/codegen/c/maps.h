// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

#ifndef DI_MAPS_H
#define DI_MAPS_H

struct {
    __uint(type, BPF_MAP_TYPE_RINGBUF);
    __uint(max_entries, 1 << 24);
} events SEC(".maps");

struct {
    __uint(type, BPF_MAP_TYPE_ARRAY);
    __uint(key_size, sizeof(__u32));
    __uint(value_size, sizeof(char[PARAM_BUFFER_SIZE]));
    __uint(max_entries, 1);
} zeroval SEC(".maps");

struct {
    __uint(type, BPF_MAP_TYPE_STACK);
    __uint(max_entries, 2048);
    __type(value, __u64);
} param_stack SEC(".maps");

struct {
    __uint(type, BPF_MAP_TYPE_PERCPU_ARRAY);
    __uint(max_entries, 1);
    __type(key, __u32);
    __type(value, __u64[4000]);
} temp_storage_array SEC(".maps");

#endif