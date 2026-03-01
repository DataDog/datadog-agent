// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build windows

package systrayimpl

// #cgo CFLAGS:  -DUNICODE -D_UNICODE -DWINVER=0x0601 -D_WIN32_WINNT=0x0601
// #cgo LDFLAGS: -ld2d1 -ldwrite -lole32 -luuid -lgdi32
// #include <windows.h>
// #include <d2d1.h>
// #include <dwrite.h>
// #include "overlay.h"
import "C"

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"syscall"
	"unsafe"

	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	"github.com/lxn/win"
)

type overlayWindow struct {
	windowHandle win.HWND
	log          log.Component
}
type overlayWindowOpts struct {
	// Pass a real parent HWND for a true child; 0 => top-level
	Parent win.HWND
	Log    log.Component
	Title  string

	Left   int
	Top    int
	Width  int
	Height int

	TextContent string
}

var (
	overlay *overlayWindow
)

// onOverlay is a callback from systray to issue a background command and
// present the output in an overlay window.
func onOverlay(s *systrayImpl) {
	if overlay == nil {
		// Create the overlay for the first time.
		var err error

		// TODO: Potentially support other commands.
		status, err := queryAgentStatus()
		if err != nil {
			status = fmt.Sprintf("Failed to query agent status: %v", err)
			s.log.Error(status)
		}

		overlayOpts := overlayWindowOpts{
			Title:       "Datadog Overlay",
			Log:         s.log,
			TextContent: status,
		}

		calculateOverlayArea(&overlayOpts, s.log)
		overlay, err = launchOverlayWindow(&overlayOpts)
		if err != nil {
			overlay = nil
			s.log.Errorf("Failed to launch overlay: %v", err)
		}

	} else {
		// The overlay exists, toggle its visibility.
		overlay.onShow(!win.IsWindowVisible(overlay.windowHandle))
	}
}

// launchOverlayWindow creates the overlay window on a dedicated UI thread
// and returns immediately. The window message loop in the dedicated thread will
// run forever until the window is closed (WM_QUIT).
func launchOverlayWindow(opts *overlayWindowOpts) (*overlayWindow, error) {
	log := opts.Log
	ow := &overlayWindow{
		log: log,
	}

	className, err := syscall.UTF16PtrFromString("DatadogOverlayClass")
	if err != nil {
		return nil, syscall.GetLastError()
	}

	errc := make(chan error, 1)

	go func() {
		// Pin all Win32 UI work to this single OS thread.
		// This will detach the overlay window from system tray.
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()

		log.Infof("Creating overlay")

		title, err := syscall.UTF16PtrFromString(opts.Title)
		if err != nil {
			errc <- syscall.GetLastError()
			return
		}

		// Register the overlay window class and tie the message handler to it.
		var wc win.WNDCLASSEX
		wc.CbSize = uint32(unsafe.Sizeof(wc))
		wc.Style = win.CS_HREDRAW | win.CS_VREDRAW
		wc.LpfnWndProc = syscall.NewCallback(ow.wndProc)
		wc.HInstance = win.GetModuleHandle(nil)
		wc.HCursor = win.LoadCursor(0, win.MAKEINTRESOURCE(win.IDC_ARROW))
		wc.HbrBackground = win.COLOR_WINDOW + 1
		wc.LpszClassName = className

		var atom win.ATOM
		if atom = win.RegisterClassEx(&wc); atom == 0 {
			// Failing to register the window class means the message handler
			// will not be active.
			errCode := syscall.GetLastError()
			log.Errorf("Failed to register window class: %v", errCode)
			errc <- errCode
			return
		}

		defer func() {
			// UnregisterClass may fail with code 824637302304, leaving the message handler stuck,
			// and causing new overlay windows to not have message handling.
			// As long as we reuse the same overlay window, avoid recreating it, and
			// bail out completely on termination, this should be fine.
			if atom != 0 && !win.UnregisterClass(className) {
				log.Warnf("Failed to unregister window class: %v", syscall.GetLastError())
			}
		}()

		hwnd := win.CreateWindowEx(
			win.WS_EX_LAYERED|win.WS_EX_TOPMOST|win.WS_EX_TOOLWINDOW,
			className,
			title,
			win.WS_POPUP,
			int32(opts.Left),
			int32(opts.Top),
			int32(opts.Width),
			int32(opts.Height),
			opts.Parent,
			0,
			wc.HInstance,
			nil,
		)
		if hwnd == 0 {
			errCode := syscall.GetLastError()
			log.Errorf("Failed to create overlay window: %v", errCode)
			errc <- errCode
			return
		}

		// This must be setup before the message handling starts.
		textContent := C.CString(opts.TextContent)
		C.InitOverlayA((C.uintptr_t)(hwnd), textContent)
		C.free(unsafe.Pointer(textContent))

		ow.windowHandle = hwnd
		log.Infof("Overlay created")
		win.ShowWindow(hwnd, win.SW_SHOWNOACTIVATE)

		// The message loop will run forever, let the caller proceed.
		errc <- nil

		var msg win.MSG
		for {
			ret := win.GetMessage(&msg, 0, 0, 0)
			if (ret == 0) || (ret == -1) {
				// Stop message loop.
				break
			}

			win.TranslateMessage(&msg)
			win.DispatchMessage(&msg)
		}

		C.CleanupOverlay()

		log.Infof("Overlay closed")
	}()

	// Wait for the window creation to complete.
	err = <-errc

	if err != nil {
		log.Errorf("failed to create overlay: %v)", errc)
		return nil, err
	}

	return ow, nil
}

//
// Window handling functions
//

// wndProc handles Windows messages, specifically showing/hiding the overlay, and key inputs.
func (w *overlayWindow) wndProc(hwnd win.HWND, msg uint32, wParam, lParam uintptr) uintptr {
	switch msg {
	case win.WM_PAINT:
		C.RenderOverlay((C.uintptr_t)(hwnd))
		return 0
	case win.WM_LBUTTONDOWN:
		return 0
	case win.WM_KEYDOWN:
		if w.handleKeyDown(hwnd, wParam) {
			return 0
		}
	case win.WM_KEYUP:
		if w.handleKeyUp(hwnd, wParam) {
			return 0
		}
	case win.WM_SHOWWINDOW:
		if wParam != 0 {
			// The overlay will be shown.
			C.ShowOverlay((C.uintptr_t)(hwnd), win.TRUE)
		} else {
			// The overlay will be hidden. Release rendering resources.
			C.ShowOverlay((C.uintptr_t)(hwnd), win.FALSE)
		}
		return 0
	case win.WM_CREATE:
		// Always first message.
		return 0
	case win.WM_CLOSE:
		win.DestroyWindow(hwnd)
		return 0
	case win.WM_DESTROY:
		// This will stop the message loop.
		win.PostQuitMessage(0)
		return 0
	}

	return win.DefWindowProc(hwnd, msg, wParam, lParam)
}

// handleKeyDown handles when the user presses or holds a key down.
func (w *overlayWindow) handleKeyDown(_ win.HWND, wParam uintptr) bool {
	switch wParam {
	case win.VK_DOWN:
		C.ScrollOverlayVertical(C.float(20.0))
		return true
	case win.VK_UP:
		C.ScrollOverlayVertical(C.float(-20.0))
		return true
	case win.VK_NEXT:
		C.ScrollOverlayVertical(C.float(100.0))
		return true
	case win.VK_PRIOR:
		C.ScrollOverlayVertical(C.float(-100.0))
		return true
	default:
		return false
	}
}

// handleKeyUp handles when the user releases a key press.
func (w *overlayWindow) handleKeyUp(_ win.HWND, wParam uintptr) bool {
	switch wParam {
	case win.VK_DOWN:
		fallthrough
	case win.VK_UP:
		fallthrough
	case win.VK_NEXT:
		fallthrough
	case win.VK_PRIOR:
		// Managed by handleKeyDown.
		return true
	case win.VK_HOME:
		C.ScrollOverlayToEnd(win.TRUE)
		return true
	case win.VK_END:
		C.ScrollOverlayToEnd(win.FALSE)
		return true
	default:
		if win.GetKeyState(win.VK_CONTROL) < 0 {
			if wParam == 'C' {
				// Copy to clipboard.
				errCode := C.CopyOverlayTextToClipboard()
				if int(errCode) != 0 {
					w.log.Errorf("failed to copy to clipboard: %d", errCode)
				}
			}

			return true
		} else if !(win.GetKeyState(win.VK_SHIFT) < 0) {
			// Hide the overlay with any other key press.
			w.onShow(false)
			return true
		}
	}

	return false
}

// onShow initiates showing or hiding the overlay.
// When called to show, it first issues a background command to query the Agent status.
func (w *overlayWindow) onShow(show bool) {
	if w.windowHandle == 0 {
		return
	}

	if show {
		status, err := queryAgentStatus()
		if err != nil {
			status := fmt.Sprintf("Failed to query agent status: %v", err)
			w.log.Error(status)
		}

		win.EnableWindow(w.windowHandle, true)
		C.SetOverlayTextA(C.CString(status))
		win.ShowWindow(w.windowHandle, win.SW_SHOWNOACTIVATE)
		win.InvalidateRect(w.windowHandle, nil, false)
	} else {
		win.ShowWindow(w.windowHandle, win.SW_HIDE)
		win.EnableWindow(w.windowHandle, false)
	}
}

//
// Support functions
//

// queryAgentStatus issues a background system command to query the Agent status.
func queryAgentStatus() (string, error) {
	var stdoutBuf, stderrBuf bytes.Buffer

	programFilesPath := os.Getenv("ProgramFiles")
	agentPath := filepath.Join(
		programFilesPath,
		"Datadog",
		"Datadog Agent",
		"bin",
		"agent.exe")
	cmd := exec.Command("cmd.exe", "/C", agentPath, "status", "2>&1")
	cmd.Stdout = &stdoutBuf
	cmd.Stderr = &stderrBuf
	err := cmd.Run()

	if err != nil {
		return "", err
	}

	return stdoutBuf.String(), nil
}

// calculateOverlayArea determines the position and size of the overlay.
func calculateOverlayArea(opts *overlayWindowOpts, log log.Component) {
	var rect win.RECT

	user32 := syscall.NewLazyDLL("user32.dll")
	procGetWindowRect := user32.NewProc("GetWindowRect")

	// Defaults
	padding := 20
	overlayWidth := 640
	overlayHeight := 200
	opts.Left = 0
	opts.Top = 0
	opts.Width = overlayWidth
	opts.Height = overlayHeight

	// Get the screen client area
	desktopHandle, _, _ := procGetDesktopWindow.Call()
	if desktopHandle == uintptr(0) {
		log.Errorf("failed to get desktop handle")
		return
	}

	r, _, _ := procGetWindowRect.Call(desktopHandle, uintptr(unsafe.Pointer(&rect)))
	if r == 0 {
		log.Errorf("failed to get desktop window rect")
		return
	}

	desktopWidth := int(rect.Right - rect.Left)
	desktopHeight := int(rect.Bottom - rect.Top)

	if desktopWidth < overlayWidth {
		overlayWidth = desktopWidth
	}

	if desktopHeight < (overlayHeight + padding) {
		overlayHeight = desktopHeight
		padding = 0
	}

	opts.Left = (desktopWidth / 2) - (overlayWidth / 2)
	opts.Top = desktopHeight - overlayHeight - padding
	opts.Width = overlayWidth
	opts.Height = overlayHeight
}

// closeOverlayWindow requests the overlay to exit.
func closeOverlayWindow() {
	if overlay != nil {
		win.PostMessage(overlay.windowHandle, win.WM_CLOSE, 0, 0)
	}
}

// goReportErrorCallback is a callback for the native (C++) code to report errors for logging.
//
//export goReportErrorCallback
func goReportErrorCallback(errorCode C.uint, message *C.char) {
	if overlay != nil {
		// Do not free or modify the original message.
		goMessage := C.GoString(message)
		overlay.log.Errorf("Error 0x%X. %s", errorCode, goMessage)
	}
}
