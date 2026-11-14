// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//nolint:revive // TODO(DBM) Fix revive linter
/*
Package oracle provides the Oracle core check.

Each database operation has an independent query_timeout deadline, in seconds
(default 20, also used for nonpositive values). go-ora's socket timeout is an
internal fallback of exactly twice that duration. Extreme timeout values are
bounded so duration conversion, doubling, and godror's native millisecond
conversion cannot overflow.

Query contexts stay valid through fetching and row cleanup. go-ora v2.9.0 does
not watch later fetches, so the connector interrupts its sockets at the operation
deadline. This ends client I/O, not necessarily the server operation immediately.
The existing recovery helpers dispose of the affected pool before replacement;
subsequent collection steps use fresh contexts. Failed queries are not replayed,
and ordinary SQL and privilege errors do not trigger reconnects. Repeated failures
can still cause frequent logins and overlap with abandoned server operations.
*/
package oracle
