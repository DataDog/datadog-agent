// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at StackState (https://www.datadoghq.com/).
// Copyright 2016-2019 StackState, Inc.
// +build windows

package main

import (
	"flag"
	"os"
	"os/exec"

	"github.com/StackVista/stackstate-agent/pkg/util/log"
	seelog "github.com/cihub/seelog"

	"github.com/StackVista/stackstate-agent/pkg/version"

	"github.com/lxn/walk"
	"golang.org/x/sys/windows"
)

type menuItem struct {
	label   string
	handler walk.EventHandler
	enabled bool
}

var (
	separator = "SEPARATOR"
	menuitems []menuItem
	ni        *walk.NotifyIcon
	launchgui bool
	eventname = windows.StringToUTF16Ptr("ststray-event")
)

func init() {
	enableLoggingToFile()
	av, _ := version.New(version.AgentVersion, version.Commit)
	verstring := av.GetNumberAndPre()

	menuitems = make([]menuItem, 0)
	menuitems = append(menuitems, menuItem{label: verstring, enabled: false})
	menuitems = append(menuitems, menuItem{label: separator})
	menuitems = append(menuitems, menuItem{label: "&Start", handler: onStart, enabled: true})
	menuitems = append(menuitems, menuItem{label: "S&top", handler: onStop, enabled: true})
	menuitems = append(menuitems, menuItem{label: "&Restart", handler: onRestart, enabled: true})
	menuitems = append(menuitems, menuItem{label: "&Configure", handler: onConfigure, enabled: canConfigure()})
	menuitems = append(menuitems, menuItem{label: "&Flare", handler: onFlare, enabled: true})
	menuitems = append(menuitems, menuItem{label: separator})
	menuitems = append(menuitems, menuItem{label: "E&xit", handler: onExit, enabled: true})
}

func onExit() {
	walk.App().Exit(0)
}

func main() {
	flag.BoolVar(&launchgui, "launch-gui", false, "Launch browser configuration and exit")
	flag.Parse()
	log.Debugf("launch-gui is %v", launchgui)
	if launchgui {
		//enableLoggingToConsole()
		defer log.Flush()
		log.Debug("Preparing to launch configuration interface...")
		onConfigure()
	}
	// check to see if the process is already running.  If so, just exit
	h, _ := windows.OpenEvent(0x1F0003, // EVENT_ALL_ACCESS
		false,
		eventname)

	if h != windows.Handle(0) {
		// was already there.  Process already running. We're done
		windows.CloseHandle(h)
		return
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

	icon, err := walk.NewIconFromResourceId(3)
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

		if err := ni.ShowCustom(
			"StackState Agent Manager",
			"Please right click to display available options."); err != nil {

			log.Warnf("Failed to show custom message %v", err)
		}
	})
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

	// Run the message loop.
	mw.Run()
}

// opens a browser window at the specified URL
func open(url string) error {
	return exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
}

func enableLoggingToFile() {
	seeConfig := `
	<seelog minlevel="debug">
	<outputs>
		<rollingfile type="size" filename="c:\\ProgramData\\StackState\\Logs\\ststray.log" maxsize="1000000" maxrolls="2" />
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
