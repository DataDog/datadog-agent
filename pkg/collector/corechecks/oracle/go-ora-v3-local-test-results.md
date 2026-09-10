# go-ora v3 local test results

Local runs of `dda inv -- oracle.test` (CI's Oracle job is blocked by incident-51958). Same
container, `main` vs this branch; only deltas attributable to the v2 -> v3 bump are listed.
Pre-existing failures on `main` (11g suite/initdb assume a CDB; `TestActiveSessionHistory` on
19c EE) are excluded.

| DB | image | main | this branch |
|---|---|---|---|
| 11g 11.2 XE | gvenzl/oracle-xe:11.2.0.2-slim | 38 pass / 18 fail | panic on connect |
| 12c 12.1 XE | truevoly/oracle-12c | 52 pass / 4 fail | panic on connect |
| 18c 18.4 XE | gvenzl/oracle-xe:18.4.0-slim | 56 pass / 0 fail | panic on connect |
| 19c 19.3 EE | banglamon/oracle193db:19.3.0-ee | 56 pass / 1 fail | 54 pass / 3 fail |
| 21c 21.3 XE | gvenzl/oracle-xe:21.3.0-slim | 56 pass / 0 fail | 55 pass / 1 fail |

## 1. Panic on connect, Oracle <= 18c

```
panic: runtime error: index out of range [3] with length 0
encoding/binary.bigEndian.Uint32(...)
go-ora/v3@v3.0.1/network.newAcceptPacketFromData({..., 0x29, 0x40})
	network/accept_packet.go:61
go-ora/v3@v3.0.1/network.(*Session).Connect
go-ora/v3@v3.0.1.(*Connection).OpenWithContext
```

`newAcceptPacketFromData` returns early only for `len(packetData) < 32`, then reads
`packetData[41:]` (`NegotiatedOptions2`). 11g/12c/18c send a 41-byte ACCEPT packet, so the first
connection panics and the suite aborts (`TestLongMultibyteQuery`, 4 pass / 1 fail). 19c and 21c
send longer packets and are unaffected.

## 2. CLOB out-parameter unsupported, 19c and 21c

`TestGetFullSqlText` -> `getFullSQLText` (`oracle_dictionary.go`), `go_ora.Out{Dest: *go_ora.Clob}`:

```
no parameter coder registered for go type types.Clob
```

v3 resolves CLOB through the coder maps populated in `OracleDriver`; connections opened via
`go_ora.NewConnection` do not get them. 19c additionally fails `TestChkRun`, which asserts on the
same code path.

## Reproducing

The task's docker lifecycle expects the internal image mirror; with a public image, start the DB
separately and skip it:

```
docker run -d -p 1521:1521 -e ORACLE_PASSWORD=datad0g gvenzl/oracle-xe:21.3.0-slim
SKIP_DOCKER=1 ORACLE_TEST_SERVICE_NAME=XE ORACLE_TEST_SYS_PASSWORD=datad0g dda inv -- oracle.test -v
```

11g and the 12c image above are non-CDB: create a local `datadog` user and run the `initdb.d`
scripts with `c##datadog` rewritten to `datadog` and `CONTAINER=ALL` dropped, then add
`ORACLE_TEST_USER=datadog ORACLE_TEST_PASSWORD=datadog`.
