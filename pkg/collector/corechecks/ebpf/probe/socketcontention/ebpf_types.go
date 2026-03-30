// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build ignore

package socketcontention

/*
#include "../../c/runtime/socket-contention-kern-user.h"
*/
import "C"

type ebpfSocketContentionStats C.struct_socket_contention_stats
type ebpfSocketContentionKey C.struct_socket_contention_key
type ebpfSocketLockIdentity C.struct_socket_lock_identity
