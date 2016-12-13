// +build dragonfly freebsd linux nacl netbsd openbsd solaris

package app

var configPaths = []string{
	"/etc/dd-agent",
	_distPath,
}
