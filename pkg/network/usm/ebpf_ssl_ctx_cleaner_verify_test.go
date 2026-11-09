// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && bpf

package usm

import (
	"testing"
	"time"
	"unsafe"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/network/protocols/http"
	"github.com/DataDog/datadog-agent/pkg/network/usm/utils"
)

// Verifies that the dead-process cleaner reads the PID from the new ssl_sock_by_ctx key layout:
// entries of a dead PID (and their ssl_ctx_by_tuple entries) are removed, entries of a live PID stay.
func TestSSLSockByCtxDeadPidCleanerVerify(t *testing.T) {
	setNativeTLSPeriodicTerminatedProcessesScanInterval(t, time.Second)
	cfg := NewUSMEmptyConfig()
	cfg.EnableNativeTLSMonitoring = true
	cfg.EnableUSMEventStream = false
	utils.SkipIfTLSUnsupported(t, cfg)
	monitor := setupUSMTLSMonitor(t, cfg, reInitEventConsumer)
	require.NotNil(t, monitor)
	cleanProtocolMaps(t, "ssl", monitor.ebpfProgram.Manager.Manager)

	maps := getMaps(t, monitor.ebpfProgram.Manager.Manager, []string{sslSockByCtxMap, sslCtxByTupleMap})
	sockByCtx, ctxByTuple := maps[0], maps[1]
	require.Equal(t, uint32(unsafe.Sizeof(http.SSLCtxPidTGID{})), sockByCtx.KeySize())

	deadPid := uint32(findNonExistingPid(t))
	// the cleaner never counts its own process as alive, so use a separate process
	cmd, cancel := startDummyProgram(t)
	alivePid := uint32(cmd.Process.Pid)
	deadKey := http.SSLCtxPidTGID{Pid: deadPid, Ctx: 0x1111}
	aliveKey := http.SSLCtxPidTGID{Pid: alivePid, Ctx: 0x2222}
	var sock http.SslSock
	require.NoError(t, sockByCtx.Put(unsafe.Pointer(&deadKey), unsafe.Pointer(&sock)))
	require.NoError(t, sockByCtx.Put(unsafe.Pointer(&aliveKey), unsafe.Pointer(&sock)))
	deadTup := http.ConnTuple{Sport: 1111, Dport: 443, Pid: deadPid}
	aliveTup := http.ConnTuple{Sport: 2222, Dport: 443, Pid: alivePid}
	deadCtx, aliveCtx := uint64(0x1111), uint64(0x2222)
	require.NoError(t, ctxByTuple.Put(unsafe.Pointer(&deadTup), unsafe.Pointer(&deadCtx)))
	require.NoError(t, ctxByTuple.Put(unsafe.Pointer(&aliveTup), unsafe.Pointer(&aliveCtx)))

	require.EventuallyWithT(t, func(ct *assert.CollectT) {
		assert.False(ct, findKeyInMap(sockByCtx, deadKey), "dead pid entry still in ssl_sock_by_ctx")
		assert.False(ct, findKeyInMap(ctxByTuple, deadTup), "dead pid tuple still in ssl_ctx_by_tuple")
	}, 10*time.Second, 500*time.Millisecond)
	// a few more cleaner runs, the live entries must survive them
	time.Sleep(3 * time.Second)
	assert.True(t, findKeyInMap(sockByCtx, aliveKey), "live pid entry was removed from ssl_sock_by_ctx")
	assert.True(t, findKeyInMap(ctxByTuple, aliveTup), "live pid tuple was removed from ssl_ctx_by_tuple")

	// once that process exits, its entries go too
	cancel()
	_ = cmd.Wait()
	require.EventuallyWithT(t, func(ct *assert.CollectT) {
		assert.False(ct, findKeyInMap(sockByCtx, aliveKey), "exited pid entry still in ssl_sock_by_ctx")
		assert.False(ct, findKeyInMap(ctxByTuple, aliveTup), "exited pid tuple still in ssl_ctx_by_tuple")
	}, 10*time.Second, 500*time.Millisecond)
}
