// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

#include <windows.h>

#ifdef __cplusplus
extern "C"
{
#endif

// InitOverlayA initializes the internal state of the overlay.
void
InitOverlayA(
    uintptr_t WindowHandle,
    char* TextContent
    );

// ShowOverlay handles the visibility of the overlay on WM_SHOWWINDOW.
void
ShowOverlay(
    uintptr_t WindowHandle,
    BOOL Show
    );

// RenderOverlay draws the overlay on WM_PAINT.
void
RenderOverlay(
    uintptr_t WindowHandle
    );

// CleanupOverlay releases all resources when the overlay is terminated.
void
CleanupOverlay(
    void
    );

// SetOverlayTextA copies the provided text to display in the overlay.
void
SetOverlayTextA(
    char* TextContent
    );

// CopyOverlayTextToClipboard copies the existing text in the overlay to the Clipboard.
int
CopyOverlayTextToClipboard(
    void
    );

// ScrollOverlayVertical updates the vertical position of the content in the overlay for scrolling.
// This responds to VK_UP, VK_DOWN, VK_NEXT, VK_PRIOR.
void
ScrollOverlayVertical(
    float Delta
    );

// ScrollOverlayVertical sets the vertical position of the content in the overlay to
// the top-most or bottom-most position. This responds to VK_HOME, VK_END.
void
ScrollOverlayToEnd(
    BOOL Front
    );

#ifdef __cplusplus
}
#endif
