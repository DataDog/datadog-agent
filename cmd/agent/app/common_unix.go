// +build dragonfly freebsd linux nacl netbsd openbsd solaris

package ddagentmain

var configPaths = []string{
	"/etc/dd-agent",
	distPath,
}
