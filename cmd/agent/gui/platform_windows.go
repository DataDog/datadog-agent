package gui

import (
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	
	"github.com/hectane/go-acl"
	"github.com/kardianos/osext"
	"golang.org/x/sys/windows"

)
var (
	wellKnownSidStrings = map[string]string {
		"Administrators": "S-1-5-32-544",
		"System": "S-1-5-18",
		"Users": "S-1-5-32-545",
	}
	wellKnownSids = make(map[string]*windows.SID)
)

func init() {
	
	for key, val := range wellKnownSidStrings {
		sid, err := windows.StringToSid(val)
		if err == nil {
			wellKnownSids[key] = sid
		} 
	}
}

// restarts the agent using the windows service manager
func restart() error {
	here, _ := osext.ExecutableFolder()
	cmd := exec.Command(filepath.Join(here, "agent"), "restart-service")
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	err := cmd.Start()
	if err != nil {
		return fmt.Errorf("Failed to restart the agent. Error: %v", err)
	}

	return nil
}

// writes auth token(s) to a file with the same permissions as datadog.yaml
func saveAuthToken(token string) error {

	err := ioutil.WriteFile(authTokenPath, []byte(token), 0755)
	if err == nil {
		err = acl.Apply(
			authTokenPath,
			true, // replace the file permissions
			false, // don't inherit
			acl.GrantSid(windows.GENERIC_ALL, wellKnownSids["Administrators"]),
			acl.GrantSid(windows.GENERIC_ALL, wellKnownSids["System"]))
		
	}
	return err
}
