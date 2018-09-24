// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

// +build secrets,windows

//
// Disclaimer:
//
// Until "datadog_secretuser" can be impersonated on windows by creating a
// specific access token and using it with os.exec:Command (>=go1.10) we have
// to manually create a process with "CreateProcessWithLogonW". This force us
// to duplicate quite a fair chunk of code from 'os.exec' package. This code
// duplication will be removed before the secrets feature goes out of beta.
//

package secrets

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"

	"github.com/DataDog/datadog-agent/pkg/util/winutil"
	"golang.org/x/sys/windows/registry"
)

var (
	advapi32                    = syscall.NewLazyDLL("advapi32.dll")
	procCreateProcessWithLogonW = advapi32.NewProc("CreateProcessWithLogonW")
)

const (
	// The user created at install time with low/no rights
	username             = "datadog_secretuser"
	passwordRegistryPath = "SOFTWARE\\Datadog\\Datadog Agent\\secrets"
	localDB              = "." // local account database
)

func getPasswordFromRegistry() (string, error) {
	k, err := registry.OpenKey(registry.LOCAL_MACHINE,
		passwordRegistryPath,
		registry.READ)
	if err != nil {
		if err == registry.ErrNotExist {
			return "", fmt.Errorf("Secret user password does not found in the registry")
		}
		return "", fmt.Errorf("can't read secrets user password from registry: %s", err)
	}
	defer k.Close()

	password, _, err := k.GetStringValue(username)
	if err != nil {
		return "", fmt.Errorf("Could not read password for secrets user from registry: %s", err)
	}
	return password, nil
}

func skipStdinCopyError(err error) bool {
	// Ignore ERROR_BROKEN_PIPE and ERROR_NO_DATA errors copying
	// to stdin if the program completed successfully otherwise.
	// See Issue 20445.
	const errorNoData = syscall.Errno(0xe8)
	pe, ok := err.(*os.PathError)
	return ok &&
		pe.Op == "write" && pe.Path == "|1" &&
		(pe.Err == syscall.ERROR_BROKEN_PIPE || pe.Err == errorNoData)
}

func setInputPipe(r io.Reader, goroutine *[]func() error, closeAfterStart, closeAfterWait *[]*os.File) error {
	pr, pw, err := os.Pipe()
	if err != nil {
		return fmt.Errorf("failed to create pipe: %s", err)
	}

	*goroutine = append(*goroutine, func() error {
		_, err := io.Copy(pw, r)
		if skipStdinCopyError(err) {
			err = nil
		}
		if err1 := pw.Close(); err == nil {
			err = err1
		}
		return err
	})
	*closeAfterStart = append(*closeAfterStart, pr)
	*closeAfterWait = append(*closeAfterWait, pw)
	return nil
}

func setOutputPipe(w io.Writer, goroutine *[]func() error, closeAfterStart, closeAfterWait *[]*os.File) error {
	pr, pw, err := os.Pipe()
	if err != nil {
		return fmt.Errorf("failed to create pipe: %s", err)
	}

	*goroutine = append(*goroutine, func() error {
		_, err := io.Copy(w, pr)
		pr.Close() // in case io.Copy stopped due to write error
		return err
	})
	*closeAfterStart = append(*closeAfterStart, pw)
	*closeAfterWait = append(*closeAfterWait, pr)
	return nil
}

func startProcessAsDatadogSecretUser(argv0 string, argv []string, attr *os.ProcAttr) (*os.Process, error) {
	argv0p, err := syscall.UTF16PtrFromString(argv0)
	if err != nil {
		return nil, fmt.Errorf("can't convert command string to UTF16: %s", err)
	}

	cmdLine := ""
	for _, v := range argv {
		if cmdLine != "" {
			cmdLine += " "
		}
		cmdLine += syscall.EscapeArg(v)
	}

	argvp, err := syscall.UTF16PtrFromString(cmdLine)
	if err != nil {
		return nil, fmt.Errorf("can't convert cmdLine string to UTF16: %s", err)
	}

	// Acquire the fork lock so that no other threads
	// create new fds that are not yet close-on-exec
	// before we fork.
	syscall.ForkLock.Lock()
	defer syscall.ForkLock.Unlock()

	p, _ := syscall.GetCurrentProcess()
	fd := make([]syscall.Handle, len(attr.Files))
	for i := range attr.Files {
		if attr.Files[i].Fd() > 0 {
			err := syscall.DuplicateHandle(p,
				syscall.Handle(attr.Files[i].Fd()),
				p,
				&fd[i],
				0,
				true,
				syscall.DUPLICATE_SAME_ACCESS,
			)
			if err != nil {
				return nil, fmt.Errorf("can't call DuplicateHandle to execute secretBackendCommand: %s", err)
			}
			defer syscall.CloseHandle(syscall.Handle(fd[i]))
		}
	}

	si := new(syscall.StartupInfo)
	si.Cb = uint32(unsafe.Sizeof(*si))
	si.Flags = syscall.STARTF_USESTDHANDLES
	si.StdInput = fd[0]
	si.StdOutput = fd[1]
	si.StdErr = fd[2]

	pi := new(syscall.ProcessInformation)

	password, err := getPasswordFromRegistry()
	if err != nil {
		return nil, err
	}

	usernamep, _ := syscall.UTF16PtrFromString(username)
	passwordp, _ := syscall.UTF16PtrFromString(password)
	localDBp, _ := syscall.UTF16PtrFromString(localDB)

	res, _, err := procCreateProcessWithLogonW.Call(
		uintptr(unsafe.Pointer(usernamep)),
		uintptr(unsafe.Pointer(localDBp)),
		uintptr(unsafe.Pointer(passwordp)),
		0, // logon flags
		uintptr(unsafe.Pointer(argv0p)),
		uintptr(unsafe.Pointer(argvp)),
		uintptr(unsafe.Pointer(nil)),
		uintptr(unsafe.Pointer(nil)), // let windows load datadog_secretuser env from it's profile
		uintptr(unsafe.Pointer(nil)), // current dir: same as the one from the datadog_agent
		uintptr(unsafe.Pointer(si)),
		uintptr(unsafe.Pointer(pi)),
	)

	if res == 0 {
		return nil, fmt.Errorf("error from CreateProcessWithLogonW: %s", err)
	}

	// the 'handle' attribute from os.Process is private so even if we have
	// the info in 'pi.Process' we need to use 'FindProcess' to be able to
	// return a os.Process struct (which avoid us duplicating even more code
	// from the os package).
	proc, err := os.FindProcess(int(pi.ProcessId))
	if err != nil {
		return nil, fmt.Errorf("error finding backend process: %s", err)
	}

	return proc, nil
}

func closeFileList(fileList []*os.File) {
	for _, f := range fileList {
		f.Close()
	}
}

func execCommand(inputPayload string) ([]byte, error) {
	if err := checkRights(secretBackendCommand); err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(),
		time.Duration(secretBackendTimeout)*time.Second)
	defer cancel()

	stdin := strings.NewReader(inputPayload)
	stdout := &limitBuffer{
		buf: &bytes.Buffer{},
		max: secretBackendOutputMaxSize,
	}
	stderr := &limitBuffer{
		buf: &bytes.Buffer{},
		max: secretBackendOutputMaxSize,
	}
	goroutine := []func() error{}
	closeAfterStart := []*os.File{}
	closeAfterWait := []*os.File{}

	// creating pipes for stdin, stdout and stderr
	if err := setInputPipe(stdin, &goroutine, &closeAfterStart, &closeAfterWait); err != nil {
		return nil, err
	}
	if err := setOutputPipe(stdout, &goroutine, &closeAfterStart, &closeAfterWait); err != nil {
		return nil, err
	}
	if err := setOutputPipe(stderr, &goroutine, &closeAfterStart, &closeAfterWait); err != nil {
		return nil, err
	}

	cmd := []string{secretBackendCommand}
	cmd = append(cmd, secretBackendArguments...)
	process, err := startProcessAsDatadogSecretUser(
		secretBackendCommand,
		cmd,
		&os.ProcAttr{Files: closeAfterStart},
	)
	if err != nil {
		closeFileList(closeAfterStart)
		closeFileList(closeAfterWait)
		return nil, err
	}
	closeFileList(closeAfterStart)

	// start read/write goroutines
	errch := make(chan error, len(goroutine))
	for _, fn := range goroutine {
		go func(fn func() error) {
			errch <- fn()
		}(fn)
	}

	waitDone := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			process.Kill()
		case <-waitDone:
		}
	}()

	state, err := process.Wait()
	close(waitDone)

	var copyError error
	for range goroutine {
		if errIO := <-errch; errIO != nil && copyError == nil {
			copyError = errIO
		}
	}

	closeFileList(closeAfterWait)

	if err != nil {
		return nil, fmt.Errorf("error while running '%s': %s", secretBackendCommand, err)
	} else if !state.Success() {
		if ctx.Err() == context.DeadlineExceeded {
			return nil, fmt.Errorf("error while running '%s': command timeout", secretBackendCommand)
		}
		return nil, fmt.Errorf("'%s' exited with failure status", secretBackendCommand)
	} else if copyError != nil {
		return nil, fmt.Errorf("error while running '%s': %s", secretBackendCommand, copyError)
	}
	return stdout.buf.Bytes(), nil
}

// checkRights check that the given filename has access controls set only for
// Administrator, Local System and datadog_secretuser.
func checkRights(filename string) error {
	if _, err := os.Stat(filename); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("secretBackendCommand %s does not exist", filename)
		}
	}

	var fileDacl *winutil.Acl
	err := winutil.GetNamedSecurityInfo(filename,
		winutil.SE_FILE_OBJECT,
		winutil.DACL_SECURITY_INFORMATION,
		nil,
		nil,
		&fileDacl,
		nil,
		nil)
	if err != nil {
		return fmt.Errorf("could not query ACLs for %s: %s", filename, err)
	}

	var aclSizeInfo winutil.AclSizeInformation
	err = winutil.GetAclInformation(fileDacl, &aclSizeInfo, winutil.AclSizeInformationEnum)
	if err != nil {
		return fmt.Errorf("could not query ACLs for %s: %s", filename, err)
	}

	// create the sids that are acceptable to us (local system account and
	// administrators group)
	var localSystem *windows.SID
	err = windows.AllocateAndInitializeSid(&windows.SECURITY_NT_AUTHORITY,
		1, // local system has 1 valid subauth
		windows.SECURITY_LOCAL_SYSTEM_RID,
		0, 0, 0, 0, 0, 0, 0,
		&localSystem)
	if err != nil {
		return fmt.Errorf("could not query Local System SID: %s", err)
	}
	defer windows.FreeSid(localSystem)

	var administrators *windows.SID
	err = windows.AllocateAndInitializeSid(&windows.SECURITY_NT_AUTHORITY,
		2, // administrators group has 2 valid subauths
		windows.SECURITY_BUILTIN_DOMAIN_RID,
		windows.DOMAIN_ALIAS_RID_ADMINS,
		0, 0, 0, 0, 0, 0,
		&administrators)
	if err != nil {
		return fmt.Errorf("could not query Administrator SID: %s", err)
	}
	defer windows.FreeSid(administrators)

	//
	// when getting the SID for the secret user, unlike above, we provide
	// the buffer. So this SID should *not* be passed to FreeSid() (the
	// way the other ones are. So much for API consistency
	//
	// also, *must* provide adequate buffer for the domain name, or the
	// function will fail (even though we aren't going to use it for anything)
	//
	var secretusersyscall *syscall.SID
	var cchRefDomain uint32
	var sidUse uint32
	var sidlen uint32
	var domainptr uint16

	// first call to get the sidbuf and domainbuf length
	err = syscall.LookupAccountName(nil, // local system lookup
		windows.StringToUTF16Ptr(username),
		secretusersyscall,
		&sidlen,
		&domainptr,
		&cchRefDomain,
		&sidUse)
	if err != error(syscall.ERROR_INSUFFICIENT_BUFFER) {
		// should never happen
		return fmt.Errorf("could not query %s SID (insufficient buffer): %s", username, err)
	}

	sidbuf := make([]uint8, sidlen+1)
	domainbuf := make([]uint16, cchRefDomain+1)
	secretusersyscall = (*syscall.SID)(unsafe.Pointer(&sidbuf[0]))

	// second call to actually fetch the SID for username
	err = syscall.LookupAccountName(nil, // local system lookup
		windows.StringToUTF16Ptr(username),
		secretusersyscall,
		&sidlen,
		&domainbuf[0],
		&cchRefDomain,
		&sidUse)
	if err != nil {
		// should never happen
		return fmt.Errorf("could not query %s SID: %s", username, err)
	}

	secretuser := (*windows.SID)(unsafe.Pointer(secretusersyscall))
	bSecretUserExplicitlyAllowed := false
	for i := uint32(0); i < aclSizeInfo.AceCount; i++ {
		var pAce *winutil.AccessAllowedAce
		if err := winutil.GetAce(fileDacl, i, &pAce); err != nil {
			return fmt.Errorf("Could not query a ACE on %s: %s", filename, err)
		}

		compareSid := (*windows.SID)(unsafe.Pointer(&pAce.SidStart))
		compareIsLocalSystem := windows.EqualSid(compareSid, localSystem)
		compareIsAdministrators := windows.EqualSid(compareSid, administrators)
		compareIsSecretUser := windows.EqualSid(compareSid, secretuser)

		if pAce.AceType == winutil.ACCESS_DENIED_ACE_TYPE {
			// if we're denying access to local system or administrators,
			// it's wrong. Otherwise, any explicit access denied is OK
			if compareIsLocalSystem || compareIsAdministrators || compareIsSecretUser {
				return fmt.Errorf("Invalid executable '%s': Can't deny access LOCAL_SYSTEM, Administrators or %s", filename, username)
			}
			// otherwise, it's fine; deny access to whomever
		}
		if pAce.AceType == winutil.ACCESS_ALLOWED_ACE_TYPE {
			if !(compareIsLocalSystem || compareIsAdministrators || compareIsSecretUser) {
				return fmt.Errorf("Invalid executable '%s': other users/groups than LOCAL_SYSTEM, Administrators or %s have rights on it", filename, username)
			}
			if compareIsSecretUser {
				bSecretUserExplicitlyAllowed = true
			}
		}
	}
	if !bSecretUserExplicitlyAllowed {
		// there was never an ACE explicitly allowing the secret user, so we can't use it
		return fmt.Errorf("'%s' user is not allowed to execute secretBackendCommand '%s'", username, filename)
	}
	return nil
}
