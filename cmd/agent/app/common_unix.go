// +build dragonfly freebsd linux nacl netbsd openbsd solaris

package ddagentmain

var configPath = []string{
	"/etc/dd-agent",
	distPath,
}
