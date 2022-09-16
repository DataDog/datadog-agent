// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.
//go:build windows
// +build windows

package main

//#include <windows.h>
//
//BOOL LaunchUnelevated(LPCWSTR CommandLine)
//{
//    BOOL result = FALSE;
//    HWND hwnd = GetShellWindow();
//
//    if (hwnd != NULL)
//    {
//        DWORD pid;
//        if (GetWindowThreadProcessId(hwnd, &pid) != 0)
//        {
//            HANDLE process = OpenProcess(PROCESS_CREATE_PROCESS, FALSE, pid);
//
//            if (process != NULL)
//            {
//                SIZE_T size;
//                if ((!InitializeProcThreadAttributeList(NULL, 1, 0, &size)) && (GetLastError() == ERROR_INSUFFICIENT_BUFFER))
//                {
//                    LPPROC_THREAD_ATTRIBUTE_LIST p = (LPPROC_THREAD_ATTRIBUTE_LIST)malloc(size);
//                    if (p != NULL)
//                    {
//                        if (InitializeProcThreadAttributeList(p, 1, 0, &size))
//                        {
//                            if (UpdateProcThreadAttribute(p, 0,
//                                                          PROC_THREAD_ATTRIBUTE_PARENT_PROCESS,
//                                                          &process, sizeof(process),
//                                                          NULL, NULL))
//                            {
//                                STARTUPINFOEXW siex = {0};
//                                siex.lpAttributeList = p;
//                                siex.StartupInfo.cb = sizeof(siex);
//                                PROCESS_INFORMATION pi = {0};
//
//                                size_t cmdlen = wcslen(CommandLine);
//                                size_t rawcmdlen = (cmdlen + 1) * sizeof(WCHAR);
//                                PWSTR cmdstr = (PWSTR)malloc(rawcmdlen);
//                                if (cmdstr != NULL)
//                                {
//                                    memcpy(cmdstr, CommandLine, rawcmdlen);
//                                    if (CreateProcessW(NULL, cmdstr, NULL, NULL, FALSE,
//                                                       CREATE_NEW_CONSOLE | EXTENDED_STARTUPINFO_PRESENT,
//                                                       NULL, NULL, &siex.StartupInfo, &pi))
//                                    {
//                                        result = TRUE;
//                                        CloseHandle(pi.hProcess);
//                                        CloseHandle(pi.hThread);
//                                    }
//                                    free(cmdstr);
//                                }
//                            }
//                        }
//                        free(p);
//                    }
//                }
//                CloseHandle(process);
//            }
//        }
//    }
//    return result;
//}
import "C"

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"
	"unsafe"

	seelog "github.com/cihub/seelog"

	"github.com/DataDog/datadog-agent/pkg/util/log"

	"github.com/DataDog/datadog-agent/pkg/version"

	"github.com/lxn/walk"
	"golang.org/x/sys/windows"
)

type menuItem struct {
	label   string
	handler walk.EventHandler
	enabled bool
}

const (
	cmdTextStartService   = "StartService"
	cmdTextStopService    = "StopService"
	cmdTextRestartService = "RestartService"
	cmdTextConfig         = "Config"
)

var (
	separator       = "SEPARATOR"
	launchGraceTime = 2
	ni              *walk.NotifyIcon
	launchgui       bool
	launchelev      bool
	launchcmd       string
	eventname       = windows.StringToUTF16Ptr("ddtray-event")
	isUserAdmin     bool
	cmds            = map[string]func(){
		cmdTextStartService:   onStart,
		cmdTextStopService:    onStop,
		cmdTextRestartService: onRestart,
		cmdTextConfig:         onConfigure,
	}
)

func init() {
	enableLoggingToFile()

	isAdmin, err := isUserAnAdmin()
	isUserAdmin = isAdmin

	if err != nil {
		log.Warnf("Failed to call isUserAnAdmin %v", err)
		// If we cannot determine if the user is admin or not let the user allow to click on the buttons.
		isUserAdmin = true
	}
}

func createMenuItems(notifyIcon *walk.NotifyIcon) []menuItem {
	av, _ := version.Agent()
	verstring := av.GetNumberAndPre()

	menuHandler := func(cmd string) func() {
		return func() {
			execCmdOrElevate(cmd)
		}
	}

	menuitems := make([]menuItem, 0)
	menuitems = append(menuitems, menuItem{label: verstring, enabled: false})
	menuitems = append(menuitems, menuItem{label: separator})
	menuitems = append(menuitems, menuItem{label: "&Start", handler: menuHandler(cmdTextStartService), enabled: true})
	menuitems = append(menuitems, menuItem{label: "S&top", handler: menuHandler(cmdTextStopService), enabled: true})
	menuitems = append(menuitems, menuItem{label: "&Restart", handler: menuHandler(cmdTextRestartService), enabled: true})
	menuitems = append(menuitems, menuItem{label: "&Configure", handler: menuHandler(cmdTextConfig), enabled: true})
	menuitems = append(menuitems, menuItem{label: "&Flare", handler: onFlare, enabled: true})
	menuitems = append(menuitems, menuItem{label: separator})
	menuitems = append(menuitems, menuItem{label: "E&xit", handler: onExit, enabled: true})

	return menuitems
}

func isUserAnAdmin() (bool, error) {
	shell32 := windows.NewLazySystemDLL("Shell32.dll")
	defer windows.FreeLibrary(windows.Handle(shell32.Handle()))

	isUserAnAdminProc := shell32.NewProc("IsUserAnAdmin")
	ret, _, winError := isUserAnAdminProc.Call()

	if winError != windows.NTE_OP_OK {
		return false, fmt.Errorf("IsUserAnAdmin returns error code %d", winError)
	}
	if ret == 0 {
		return false, nil
	}
	return true, nil
}

func showCustomMessage(notifyIcon *walk.NotifyIcon, message string) {
	if err := notifyIcon.ShowCustom("Datadog Agent Manager", message); err != nil {
		log.Warnf("Failed to show custom message %v", err)
	}
}

func onExit() {
	walk.App().Exit(0)
}

func main() {
	// Following https://github.com/lxn/win/commit/d9566253ae00d0a7dc7e4c9bda651dcfee029001
	// it's up to the caller to lock OS threads
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	flag.BoolVar(&launchgui, "launch-gui", false, "Launch browser configuration and exit")

	// launch-elev=true only means the process should have been elevated so that it will not elevate again. If the
	// parameter is specified but the process is not elevated, some operation will fail due to access denied.
	flag.BoolVar(&launchelev, "launch-elev", false, "Launch program as elevated, internal use only")

	// If this parameter is specified, the process will try to carry out the command before the message loop.
	flag.StringVar(&launchcmd, "launch-cmd", "", "Carry out a specific command after launch")
	flag.Parse()

	log.Debugf("launch-gui is %v, launch-elev is %v, launch-cmd is %v", launchgui, launchelev, launchcmd)

	if launchgui {
		//enableLoggingToConsole()
		defer log.Flush()
		log.Debug("Preparing to launch configuration interface...")
		onConfigure()
	}

	// Check to see if the process is already running
	h, _ := windows.OpenEvent(0x1F0003, // EVENT_ALL_ACCESS
		false,
		eventname)

	if h != windows.Handle(0) {
		// Process already running.
		windows.CloseHandle(h)

		// Wait a short period and recheck in case the other process will quit.
		time.Sleep(time.Duration(launchGraceTime) * time.Second)

		// Try again
		h, _ := windows.OpenEvent(0x1F0003, // EVENT_ALL_ACCESS
			false,
			eventname)

		if h != windows.Handle(0) {
			windows.CloseHandle(h)
			return
		}
	}

	// otherwise, create the handle so that nobody else will
	h, _ = windows.CreateEvent(nil, 0, 0, eventname)
	// should never fail; test just to make sure we don't close unopened handle
	if h != windows.Handle(0) {
		defer windows.CloseHandle(h)
	}
	// We need either a walk.MainWindow or a walk.Dialog for their message loop.
	// We will not make it visible in this example, though.
	mw, err := walk.NewMainWindow()
	if err != nil {
		log.Errorf("Failed to create main window %v", err)
		os.Exit(1)
	}

	// 1 is the ID of the MAIN_ICON in systray.rc
	icon, err := walk.NewIconFromResourceId(1)
	if err != nil {
		log.Warnf("Failed to load icon %v", err)
	}
	// Create the notify icon and make sure we clean it up on exit.
	ni, err = walk.NewNotifyIcon()
	if err != nil {
		log.Errorf("Failed to create newNotifyIcon %v", err)
		os.Exit(2)
	}
	defer ni.Dispose()

	// Set the icon and a tool tip text.
	if err := ni.SetIcon(icon); err != nil {
		log.Warnf("Failed to set icon %v", err)
	}
	if err := ni.SetToolTip("Click for info or use the context menu to exit."); err != nil {
		log.Warnf("Failed to set tooltip text %v", err)
	}

	// When the left mouse button is pressed, bring up our balloon.
	ni.MouseDown().Attach(func(x, y int, button walk.MouseButton) {
		if button != walk.LeftButton {
			return
		}
		showCustomMessage(ni, "Please right click to display available options.")
	})

	menuitems := createMenuItems(ni)

	for _, item := range menuitems {
		var action *walk.Action
		if item.label == separator {
			action = walk.NewSeparatorAction()
		} else {
			action = walk.NewAction()
			if err := action.SetText(item.label); err != nil {
				log.Warnf("Failed to set text for item %s %v", item.label, err)
				continue
			}
			action.SetEnabled(item.enabled)
			if item.handler != nil {
				action.Triggered().Attach(item.handler)
			}
		}
		ni.ContextMenu().Actions().Add(action)
	}

	// The notify icon is hidden initially, so we have to make it visible.
	if err := ni.SetVisible(true); err != nil {
		log.Warnf("Failed to set window visibility %v", err)
	}

	// If a command is specified in process command line, carry it out.
	if launchcmd != "" {
		execCmdOrElevate(launchcmd)
	}

	// Run the message loop.
	mw.Run()
}

// opens a browser window at the specified URL
func open(url string) error {
	cmdptr := windows.StringToUTF16Ptr("rundll32.exe url.dll,FileProtocolHandler " + url)
	if C.LaunchUnelevated(C.LPCWSTR(unsafe.Pointer(cmdptr))) == 0 {
		// Failed to run process non-elevated, retry with normal launch.
		log.Warnf("Failed to launch configuration page as non-elevated, will launch as current process.")
		return exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
	}

	// Succeeded, return no error.
	return nil
}

func enableLoggingToFile() {
	seeConfig := `
	<seelog minlevel="debug">
	<outputs>
		<rollingfile type="size" filename="c:\\ProgramData\\DataDog\\Logs\\ddtray.log" maxsize="1000000" maxrolls="2" />
	</outputs>
	</seelog>`
	logger, _ := seelog.LoggerFromConfigAsBytes([]byte(seeConfig))
	log.ReplaceLogger(logger)
}

func enableLoggingToConsole() {
	seeConfig := `
	<seelog minlevel="debug">
	<outputs>
		<console />
	</outputs>
	</seelog>`
	logger, _ := seelog.LoggerFromConfigAsBytes([]byte(seeConfig))
	log.ReplaceLogger(logger)
}

// execCmdOrElevate carries out a command. If current process is not elevated and is not supposed to be elevated, it will launch
// itself as elevated and quit from the current instance.
func execCmdOrElevate(cmd string) {
	if !launchelev && !isUserAdmin {
		// If not launched as elevated and user is not admin, relaunch self. Use AND here to prevent from dead loop.
		relaunchElevated(cmd)

		// If elevation failed, just quit to the caller.
		return
	}

	if cmds[cmd] != nil {
		cmds[cmd]()
	}
}

// relaunchElevated launch another instance of the current process asking it to carry out a command as admin.
// If the function succeeds, it will quit the process, otherwise the function will return to the caller.
func relaunchElevated(cmd string) {
	verb := "runas"
	exe, _ := os.Executable()
	cwd, _ := os.Getwd()

	// Reconstruct arguments, drop launch-gui and tell the new process it should have been elevated.
	xargs := []string{"-launch-elev=true", "-launch-cmd=" + cmd}
	args := strings.Join(xargs, " ")

	verbPtr, _ := windows.UTF16PtrFromString(verb)
	exePtr, _ := windows.UTF16PtrFromString(exe)
	cwdPtr, _ := windows.UTF16PtrFromString(cwd)
	argPtr, _ := windows.UTF16PtrFromString(args)

	var showCmd int32 = 1 //SW_NORMAL

	err := windows.ShellExecute(0, verbPtr, exePtr, argPtr, cwdPtr, showCmd)
	if err != nil {
		log.Warnf("Failed to launch self as elevated %v", err)
	} else {
		onExit()
	}
}
