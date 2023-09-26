// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.
//go:build windows

package systray

//#include "uac.h"
import "C"

import (
	"context"
	"fmt"
	"os/exec"
	"runtime"
	"sync"
	"time"
	"unsafe"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/flare"
	"github.com/DataDog/datadog-agent/comp/core/log"
	pkglog "github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/winutil"
	"github.com/DataDog/datadog-agent/pkg/version"

	"github.com/lxn/walk"
	"github.com/lxn/win"
	"go.uber.org/fx"
	"golang.org/x/sys/windows"
)

type dependencies struct {
	fx.In

	Lc         fx.Lifecycle
	Shutdowner fx.Shutdowner

	Log    log.Component
	Config config.Component
	Flare  flare.Component
	Params Params
}

type systray struct {
	// For triggering Shutdown
	shutdowner fx.Shutdowner

	log    log.Component
	config config.Component
	flare  flare.Component
	params Params

	// allocated in start, destroyed in stop
	singletonEventHandle windows.Handle

	// Window management
	notifyWindowToStop func()
	routineWaitGroup   sync.WaitGroup
}

type menuItem struct {
	label   string
	handler walk.EventHandler
	enabled bool
}

//revive:disable:var-naming Name is intended to match the Windows const name from RC file
const (
	RSRC_MAIN_ICON  = 1
	IDD_DIALOG1     = 101
	IDC_TICKET_EDIT = 1001
	IDC_EMAIL_EDIT  = 1002
)

//revive:enable:var-naming (const)

const (
	launchGraceTime       = 2
	eventname             = "ddtray-event"
	cmdTextStartService   = "StartService"
	cmdTextStopService    = "StopService"
	cmdTextRestartService = "RestartService"
	cmdTextConfig         = "Config"
	menuSeparator         = "SEPARATOR"
)

var (
	cmds = map[string]func(*systray){
		cmdTextStartService:   onStart,
		cmdTextStopService:    onStop,
		cmdTextRestartService: onRestart,
		cmdTextConfig:         onConfigure,
	}
)

// newSystray creates a new systray component, which will start and stop based on
// the fx Lifecycle
func newSystray(deps dependencies) (Component, error) {
	// init vars
	isAdmin, err := winutil.IsUserAnAdmin()
	if err != nil {
		return nil, fmt.Errorf("failed to call IsUserAnAdmin %v", err)
	}
	if !isAdmin {
		return nil, fmt.Errorf("not running as an admin, systray requires administrative privileges")
	}

	// fx init
	s := &systray{
		log:        deps.Log,
		config:     deps.Config,
		flare:      deps.Flare,
		params:     deps.Params,
		shutdowner: deps.Shutdowner,
	}

	// fx lifecycle hooks
	deps.Lc.Append(fx.Hook{OnStart: s.start, OnStop: s.stop})

	return s, nil
}

// start hook has a fx enforced timeout, so don't do long running things
func (s *systray) start(ctx context.Context) error {
	var err error

	s.log.Debugf("launch-gui is %v, launch-elev is %v, launch-cmd is %v", s.params.LaunchGuiFlag, s.params.LaunchElevatedFlag, s.params.LaunchCommand)

	if s.params.LaunchGuiFlag {
		s.log.Debug("Preparing to launch configuration interface...")
		go onConfigure(s)
	}

	s.singletonEventHandle, err = acquireProcessSingleton(eventname)
	if err != nil {
		s.log.Errorf("Failed to acquire singleton %v", err)
		return err
	}

	s.routineWaitGroup.Add(1)
	go windowRoutine(s)

	// If a command is specified in process command line, carry it out.
	if s.params.LaunchCommand != "" {
		go execCmd(s, s.params.LaunchCommand)
	}

	return nil
}

func (s *systray) stop(ctx context.Context) error {
	if s.notifyWindowToStop != nil {
		// Send stop message to window (stops windowRoutine goroutine)
		s.notifyWindowToStop()
	}

	// wait for goroutine to finish
	s.routineWaitGroup.Wait()

	// release our singleton
	if s.singletonEventHandle != windows.Handle(0) {
		windows.CloseHandle(s.singletonEventHandle)
	}

	return nil
}

// Run window setup and message loop in a single threadlocked goroutine
// https://github.com/lxn/walk/issues/601
// Use the notifyWindowToStop function to stop the message loop
// Use routineWaitGroup to wait until the routine exits
func windowRoutine(s *systray) {
	// Following https://github.com/lxn/win/commit/d9566253ae00d0a7dc7e4c9bda651dcfee029001
	// it's up to the caller to lock OS threads
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	defer stopSystray(s)
	defer s.routineWaitGroup.Done()

	// We need either a walk.MainWindow or a walk.Dialog for their message loop.
	mw, err := walk.NewMainWindow()
	if err != nil {
		s.log.Errorf("Failed to create main window: %v", err)
		return
	}
	defer mw.Dispose()

	ni, err := createNotifyIcon(s, mw)
	if err != nil {
		s.log.Errorf("Failed to create notification tray icon: %v", err)
		return
	}
	defer ni.Dispose()

	// Provide a function that will trigger this thread to run PostQuitMessage()
	// which will cause the message loop to return
	s.notifyWindowToStop = func() {
		mw.Synchronize(func() {
			win.PostQuitMessage(0)
		})
	}

	// Run the message loop
	// use the notifyWindowToStop function to stop the message loop
	mw.Run()
}

func acquireProcessSingleton(eventname string) (windows.Handle, error) {
	var utf16EventName = windows.StringToUTF16Ptr(eventname)

	// Check to see if the process is already running
	h, _ := windows.OpenEvent(windows.EVENT_ALL_ACCESS,
		false,
		utf16EventName)

	if h != windows.Handle(0) {
		// Process already running.
		windows.CloseHandle(h)

		// Wait a short period and recheck in case the other process will quit.
		time.Sleep(time.Duration(launchGraceTime) * time.Second)

		// Try again
		h, _ := windows.OpenEvent(windows.EVENT_ALL_ACCESS,
			false,
			utf16EventName)

		if h != windows.Handle(0) {
			windows.CloseHandle(h)
			return windows.Handle(0), fmt.Errorf("systray is already running")
		}
	}

	// otherwise, create the handle so that nobody else will
	h, err := windows.CreateEvent(nil, 0, 0, utf16EventName)
	if err != nil {
		// can fail with ERROR_ALREADY_EXISTS if we lost a race
		if h != windows.Handle(0) {
			windows.CloseHandle(h)
		}
		return windows.Handle(0), err
	}

	return h, nil
}

func loadIconFromResource(log log.Component, iconID int) (*walk.Icon, error) {
	icon, err := walk.NewIconFromResourceId(iconID)
	if err == nil {
		return icon, nil
	}
	log.Warnf("Failed to load icon: %v", err)

	// NOTE: Windows 7/2008r2 issue only
	//       walk.NewIconFromResource eventually calls comctl32.LoadIconWithScaleDown, which due to an
	//       issue in lxn/win cannot be called on Windows 7/2008r2. This issue presents oddly because
	//       lxn/win will return 0 and no error when it fails to find/load the function, so the function call
	//       appears as if it is failing without cause. The function is missing because lxn/win uses
	//       windows.NewLazySystemDLL to load comctl32 which forces the system32 DLL to be loaded instead of
	//       the side-by-side (SxS) DLL specified in the manifest.
	//
	//       We can manually load and call LoadIconWithScaleDown, but when we do
	//           if we pass 96x96 (lxn/walk default), the icon comes out bad from scaling
	//           if we pass 16x16 (windows tray icon size), the notifyIcon.ShowCustomMessge icon is too small.
	//       Windows 10/2019 seem to not have either of these issues.
	//
	//       Previous versions of lxn/walk called LoadImage instead and it worked okay, so fallback to that.
	hIcon := win.LoadImage(
		win.GetModuleHandle(nil),
		win.MAKEINTRESOURCE(uintptr(iconID)),
		win.IMAGE_ICON,
		0, // width
		0, // height
		win.LR_DEFAULTSIZE,
	)
	if hIcon == 0 {
		gle := win.GetLastError()
		return nil, fmt.Errorf("Failed to load fallback icon: %x (%d)", gle, gle)
	}

	icon, err = walk.NewIconFromHICON(win.HICON(hIcon))
	return icon, err
}

// this function must be called from and the NotifyIcon used from a single thread locked goroutine
// https://github.com/lxn/walk/issues/601
func createNotifyIcon(s *systray, mw *walk.MainWindow) (ni *walk.NotifyIcon, err error) {
	// Create the notify icon (must be cleaned up)
	ni, err = walk.NewNotifyIcon(mw)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err != nil && ni != nil {
			ni.Dispose()
			ni = nil
		}
	}()

	// Set the icon and a tool tip text.
	// 1 is the ID of the MAIN_ICON in systray.rc
	icon, err := loadIconFromResource(s.log, RSRC_MAIN_ICON)
	if err != nil {
		s.log.Warnf("%v, last chance fallback to standard question mark icon.", err)
		// Fallback to a question mark icon
		icon = walk.IconQuestion()
	}
	if err := ni.SetIcon(icon); err != nil {
		return nil, fmt.Errorf("Failed to set icon: %v", err)
	}

	// Set mouseover tooltip
	if err := ni.SetToolTip("Click for info or use the context menu to exit."); err != nil {
		s.log.Warnf("Failed to set tooltip text: %v", err)
	}

	// When the left mouse button is pressed, bring up our balloon.
	ni.MouseDown().Attach(func(x, y int, button walk.MouseButton) {
		if button != walk.LeftButton {
			return
		}
		showCustomMessage(ni, "Please right click to display available options.")
	})

	menuitems := createMenuItems(s, ni)

	for _, item := range menuitems {
		var action *walk.Action
		if item.label == menuSeparator {
			action = walk.NewSeparatorAction()
		} else {
			action = walk.NewAction()
			if err := action.SetText(item.label); err != nil {
				s.log.Warnf("Failed to set text for item %s: %v", item.label, err)
				continue
			}
			err = action.SetEnabled(item.enabled)
			if err != nil {
				s.log.Warnf("Failed to set enabled for item %s: %v", item.label, err)
				continue
			}
			if item.handler != nil {
				_ = action.Triggered().Attach(item.handler)
			}
		}
		err = ni.ContextMenu().Actions().Add(action)
		if err != nil {
			s.log.Warnf("Failed to add action for item %s to context menu %v", item.label, err)
			continue
		}
	}

	// The notify icon is hidden initially, so we have to make it visible.
	if err := ni.SetVisible(true); err != nil {
		s.log.Warnf("Failed to set window visibility %v", err)
	}

	return ni, nil
}

func showCustomMessage(notifyIcon *walk.NotifyIcon, message string) {
	if err := notifyIcon.ShowCustom("Datadog Agent Manager", message, nil); err != nil {
		pkglog.Warnf("Failed to show custom message %v", err)
	}
}

func stopSystray(s *systray) {
	// TODO: This will shutdown the entire fx app, how do we stop just this component?
	//       Stopping just this componenent is not strictly needed at the moment because
	//       it is only used in the standalone ddtray.exe executable.
	triggerShutdown(s)
}

func triggerShutdown(s *systray) {
	if s != nil {
		// Tell fx to begin shutdown process
		_ = s.shutdowner.Shutdown()
	}
}

func onExit(s *systray) {
	triggerShutdown(s)
}

func createMenuItems(s *systray, notifyIcon *walk.NotifyIcon) []menuItem {
	av, _ := version.Agent()
	verstring := av.GetNumberAndPre()

	menuHandler := func(cmd string) func() {
		return func() {
			execCmd(s, cmd)
		}
	}

	menuitems := make([]menuItem, 0)
	menuitems = append(menuitems, menuItem{label: verstring, enabled: false})
	menuitems = append(menuitems, menuItem{label: menuSeparator})
	menuitems = append(menuitems, menuItem{label: "&Start", handler: menuHandler(cmdTextStartService), enabled: true})
	menuitems = append(menuitems, menuItem{label: "S&top", handler: menuHandler(cmdTextStopService), enabled: true})
	menuitems = append(menuitems, menuItem{label: "&Restart", handler: menuHandler(cmdTextRestartService), enabled: true})
	menuitems = append(menuitems, menuItem{label: "&Configure", handler: menuHandler(cmdTextConfig), enabled: true})
	menuitems = append(menuitems, menuItem{label: "&Flare", handler: func() { onFlare(s) }, enabled: true})
	menuitems = append(menuitems, menuItem{label: menuSeparator})
	menuitems = append(menuitems, menuItem{label: "E&xit", handler: func() { onExit(s) }, enabled: true})

	return menuitems
}

// opens a browser window at the specified URL
func open(url string) error {
	cmdptr := windows.StringToUTF16Ptr("rundll32.exe url.dll,FileProtocolHandler " + url)
	if C.LaunchUnelevated(C.LPCWSTR(unsafe.Pointer(cmdptr))) == 0 {
		// Failed to run process non-elevated, retry with normal launch.
		pkglog.Warnf("Failed to launch configuration page as non-elevated, will launch as current process.")
		return exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
	}

	// Succeeded, return no error.
	return nil
}

// execCmdOrElevate carries out a command
func execCmd(s *systray, cmd string) {
	if cmds[cmd] != nil {
		cmds[cmd](s)
	}
}
