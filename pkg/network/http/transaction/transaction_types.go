// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build ignore
// +build ignore

package http

/*
#include "../../ebpf/c/tracer.h"
#include "../../ebpf/c/tags-types.h"
#include "../../ebpf/c/http-types.h"
*/
import "C"

type ebpfHttpTx C.http_transaction_t