//go:build linux || freebsd || netbsd || openbsd || solaris || dragonfly || aix

package logsetup

const defaultSyslogURI = "unixgram:///dev/log"
