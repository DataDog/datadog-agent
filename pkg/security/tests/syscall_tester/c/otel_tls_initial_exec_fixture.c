// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

#include <stdint.h>

struct otel_thread_ctx_record {
    uint8_t trace_id[16];
    uint8_t span_id[8];
    uint8_t valid;
    uint8_t _reserved;
    uint16_t attrs_data_size;
};

__attribute__((visibility("default"), tls_model("initial-exec"))) __thread struct otel_thread_ctx_record *otel_thread_ctx_v1 = 0;

__attribute__((visibility("default"))) void otel_initial_exec_publish(struct otel_thread_ctx_record *record) {
    otel_thread_ctx_v1 = 0;
    __atomic_signal_fence(__ATOMIC_SEQ_CST);
    otel_thread_ctx_v1 = record;
    __atomic_signal_fence(__ATOMIC_SEQ_CST);
}

__attribute__((visibility("default"))) void otel_initial_exec_clear(void) {
    otel_thread_ctx_v1 = 0;
    __atomic_signal_fence(__ATOMIC_SEQ_CST);
}
