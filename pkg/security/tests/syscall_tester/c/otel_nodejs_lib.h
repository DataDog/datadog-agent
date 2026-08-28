// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// The otel-node-span-* commands, published by otel_nodejs_lib.c and reached by
// otel_nodejs_driver.c either through a startup link or through dlopen.

#ifndef OTEL_NODEJS_LIB_H
#define OTEL_NODEJS_LIB_H

int otel_node_span_open(int argc, char **argv);
int otel_node_span_open_chained(int argc, char **argv);
int otel_node_span_open_deep(int argc, char **argv);
int otel_node_span_exec(int argc, char **argv);
int otel_node_span_fork_exec(int argc, char **argv);

// Negative cases: a record the reader must reject, an asynchronous context
// holding no context of ours, a frame our key is not in, and a thread the writer
// never ran on.
int otel_node_span_open_invalid(int argc, char **argv);
int otel_node_span_open_no_context(int argc, char **argv);
int otel_node_span_open_als_absent(int argc, char **argv);
int otel_node_span_open_no_writer(int argc, char **argv);

#endif
