package app

import "os/exec"

// opens a browser window at the specified URL
func open(url string) error {
	return exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
}
