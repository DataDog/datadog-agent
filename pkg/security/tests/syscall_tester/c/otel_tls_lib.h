// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// The otel-span-* commands, published by otel_tls_lib.c and reached by
// otel_tls_driver.c either through a startup link or through dlopen.

#ifndef OTEL_TLS_LIB_H
#define OTEL_TLS_LIB_H

int otel_span_open(int argc, char **argv);
int otel_span_open_wait(int argc, char **argv);
int otel_span_exec(int argc, char **argv);
int otel_span_fork_exec(int argc, char **argv);
int otel_span_fork_open(int argc, char **argv);

// Negative cases: a record the reader must reject, and no record at all.
int otel_span_open_invalid(int argc, char **argv);
int otel_span_open_null_ptr(int argc, char **argv);
int otel_span_exec_invalid(int argc, char **argv);
int otel_span_exec_null_ptr(int argc, char **argv);

#endif
