// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.
//go:build windows

package systrayimpl

import (
	"bytes"
	"fmt"
	"regexp"
	"strconv"
	"sync"
	"unsafe"

	"github.com/lxn/win"
	"golang.org/x/sys/windows"

	"github.com/DataDog/datadog-agent/comp/core/flare/helpers"
	"github.com/DataDog/datadog-agent/pkg/api/util"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	pkglog "github.com/DataDog/datadog-agent/pkg/util/log"
)

var (
	validemail = regexp.MustCompile(`^[\w-\.]+@([\w-]+\.)+[\w-]{2,4}$`)
	moduser32  = windows.NewLazyDLL("user32.dll")

	procGetDlgItem       = moduser32.NewProc("GetDlgItem")
	procSetWindowPos     = moduser32.NewProc("SetWindowPos")
	procGetWindowRect    = moduser32.NewProc("GetWindowRect")
	procGetDesktopWindow = moduser32.NewProc("GetDesktopWindow")
	info                 flareInfo
	inProgress           = sync.Mutex{}
)

type flareInfo struct {
	caseid string
	email  string
}

func getDlgItem(hwnd win.HWND, id int32) (win.HWND, error) {
	ret, _, err := procGetDlgItem.Call(uintptr(hwnd), uintptr(id))
	if ret == 0 {
		return win.HWND(0), err
	}
	return win.HWND(ret), nil
}
func calcPos(outer win.RECT, inner win.RECT) (x, y, w, h int32) {
	outerWidth := outer.Right - outer.Left
	innerWidth := inner.Right - inner.Left

	outerHeight := outer.Bottom - outer.Top
	innerHeight := inner.Bottom - inner.Top
	x = (outerWidth / 2) - (innerWidth / 2)
	y = (outerHeight / 2) - (innerHeight / 2)

	w = innerWidth
	h = innerHeight
	return
}
func dialogProc(hwnd win.HWND, msg uint32, wParam, _ uintptr) (result uintptr) {
	switch msg {
	case win.WM_INITDIALOG:
		var wndrect win.RECT
		var dlgrect win.RECT
		// get the screen client area
		dt, _, _ := procGetDesktopWindow.Call()
		if dt != uintptr(0) {
			r, _, err := procGetWindowRect.Call(dt, uintptr(unsafe.Pointer(&wndrect)))
			if r != 0 {
				r, _, _ = procGetWindowRect.Call(dt, uintptr(unsafe.Pointer(&wndrect)))
				if r != 0 {
					x, y, _, _ := calcPos(wndrect, dlgrect)
					r, _, err = procSetWindowPos.Call(uintptr(hwnd), 0, uintptr(x), uintptr(y), uintptr(0), uintptr(0), uintptr(0x0041))
					if r != 0 {
						pkglog.Debugf("failed to set window pos %v", err)
					}
				}
			} else {
				pkglog.Debugf("failed to get pos %v", err)
			}
		}
		// set the "OK" to disabled until there's something approximating an email
		// address in the edit field
		edithandle, err := getDlgItem(hwnd, win.IDOK)
		if err == nil {
			win.EnableWindow(edithandle, false)
		}
		return uintptr(1)

	case win.WM_COMMAND:
		switch win.LOWORD(uint32(wParam)) {
		case IDC_EMAIL_EDIT:
			switch win.HIWORD(uint32(wParam)) {
			case win.EN_UPDATE:
				// get the text, see if it looks kinda like an email address
				buf := make([]uint16, 256)
				win.SendDlgItemMessage(hwnd, IDC_EMAIL_EDIT, win.WM_GETTEXT, 255, uintptr(unsafe.Pointer(&buf[0])))
				emailstr := windows.UTF16ToString(buf)
				edithandle, err := getDlgItem(hwnd, win.IDOK)
				if err == nil {
					if validemail.MatchString(emailstr) {
						win.EnableWindow(edithandle, true)
					} else {
						win.EnableWindow(edithandle, false)
					}
				}
			}
		case win.IDOK:
			buf := make([]uint16, 256)

			win.SendDlgItemMessage(hwnd, IDC_TICKET_EDIT, win.WM_GETTEXT, 255, uintptr(unsafe.Pointer(&buf[0])))
			info.caseid = windows.UTF16ToString(buf)
			pkglog.Debugf("ticket number %s", info.caseid)

			win.SendDlgItemMessage(hwnd, IDC_EMAIL_EDIT, win.WM_GETTEXT, 255, uintptr(unsafe.Pointer(&buf[0])))
			info.email = windows.UTF16ToString(buf)
			pkglog.Debugf("email %s", info.email)

			win.EndDialog(hwnd, win.IDOK)
			return uintptr(1)
		case win.IDCANCEL:
			win.EndDialog(hwnd, win.IDCANCEL)
			return uintptr(1)
		}
	}
	return uintptr(0)
}
func onFlare(s *systrayImpl) {
	// Create a dialog box to prompt for ticket number and email, then create and submit the flare
	// library will allow multiple calls (multi-threaded window proc?)
	// however, we're using a single instance of the info structure to
	// pass data around.  Don't allow multiple dialogs to be displayed

	if !inProgress.TryLock() {
		s.log.Warn("Dialog already in progress, skipping")
		return
	}
	defer inProgress.Unlock()

	myInst := win.GetModuleHandle(nil)
	if myInst == win.HINSTANCE(0) {
		s.log.Errorf("Failed to get my own module handle")
		return
	}
	ret := win.DialogBoxParam(myInst, win.MAKEINTRESOURCE(uintptr(IDD_DIALOG1)), win.HWND(0), windows.NewCallback(dialogProc), uintptr(0))
	if ret == 1 {
		// kick off the flare
		if _, err := strconv.Atoi(info.caseid); err != nil {
			// got a non number, just create a new case
			info.caseid = "0"
		}
		r, e := requestFlare(s, info.caseid, info.email)
		caption, _ := windows.UTF16PtrFromString("Datadog Flare")
		var text *uint16
		if e == nil {
			text, _ = windows.UTF16PtrFromString(r)
		} else {
			text, _ = windows.UTF16PtrFromString(fmt.Sprintf("Error creating flare %v", e))
		}
		win.MessageBox(win.HWND(0), text, caption, win.MB_OK)
	}
	s.log.Debugf("DialogBoxParam returns %d", ret)

}

func requestFlare(s *systrayImpl, caseID, customerEmail string) (response string, e error) {
	// For first try, ask the agent to build the flare
	s.log.Debug("Asking the agent to build the flare archive.")

	c := util.GetClient(false) // FIX: get certificates right then make this true
	ipcAddress, err := pkgconfigsetup.GetIPCAddress(pkgconfigsetup.Datadog())
	if err != nil {
		return "", err
	}
	urlstr := fmt.Sprintf("https://%v:%v/agent/flare", ipcAddress, pkgconfigsetup.Datadog().GetInt("cmd_port"))

	// Set session token
	e = util.SetAuthToken(pkgconfigsetup.Datadog())
	if e != nil {
		return
	}

	r, e := util.DoPost(c, urlstr, "application/json", bytes.NewBuffer([]byte{}))
	var filePath string
	if e != nil {
		// The agent could not make the flare, try create one from this context
		if r != nil && string(r) != "" {
			s.log.Warnf("The agent ran into an error while making the flare: %s\n", r)
			e = fmt.Errorf("Error getting flare from running agent: %s", r)
		} else {
			s.log.Debug("The agent was unable to make the flare.")
			e = fmt.Errorf("Error getting flare from running agent: %w", e)
		}
		s.log.Debug("Initiating flare locally.")

		filePath, e = s.flare.Create(nil, 0, e)
		if e != nil {
			s.log.Errorf("The flare zipfile failed to be created: %s\n", e)
			return
		}
	} else {
		filePath = string(r)
	}

	s.log.Warnf("%s is going to be uploaded to Datadog\n", filePath)

	response, e = s.flare.Send(filePath, caseID, customerEmail, helpers.NewLocalFlareSource())
	s.log.Debug(response)
	if e != nil {
		return
	}
	return response, nil
}
