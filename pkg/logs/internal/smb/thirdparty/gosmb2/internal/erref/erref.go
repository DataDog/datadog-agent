// Package erref holds the NTSTATUS codes defined in [MS-ERREF].
//
// Datadog patch: the upstream go:generate directive was removed because its
// generator (mkntstatus.go, which scrapes MSDN with goquery) is not vendored.
// ntstatus.go is kept exactly as generated upstream.
package erref
