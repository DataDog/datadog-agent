// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

#include <d2d1.h>
#include <dwrite.h>
#include <stdint.h>
#include "overlay.h"

// Exported Go functions.
#include "_cgo_export.h"

#pragma comment(lib, "d2d1.lib")
#pragma comment(lib, "dwrite.lib")

#define DATADOG_OVERLAY_FADE_IN 2
#define DATADOG_OVERLAY_FADE_OUT 3
#define DATADOG_OVERLAY_MAX_ALPHA 190

//
// Types
//

// _RENDER_CONTEXT encapsulates the resources needed for rending the overlay.
// The resources are frequently setup and released on demand.
typedef struct _RENDER_CONTEXT
{
    ID2D1Factory* DFactory;
    ID2D1HwndRenderTarget* RenderTarget;
    ID2D1SolidColorBrush* Brush;
    ID2D1SolidColorBrush* BorderBrush;
    ID2D1SolidColorBrush* BarBrush;
    IDWriteFactory* DWriteFactory;
    IDWriteTextFormat* MainTextFormat;
    IDWriteTextFormat* BarTextFormat;
    ID2D1Bitmap* Icon;
    float TextLineHeight;
    float TextBaseHeightOffset;

    float ScrollOffset;
    float MaxScrollOffset;
} RENDER_CONTEXT;

// _OVERLAY_CONTEXT encapsulates the state and resources of the overlay window.
typedef struct _OVERLAY_CONTEXT
{
    HWND WindowHandle;
    RENDER_CONTEXT RenderCtx;
    int FadeAlpha;
    wchar_t* TextContent;
    UINT32 TextContentLength;
} OVERLAY_CONTEXT;

//
// Globals
//

static OVERLAY_CONTEXT g_Overlay = {};

//
// Forward declarations
//

static
void
ReportError(
    HRESULT ErrorCode,
    const char* Message
    );

static
void
SetupRenderContext(
    HWND WindowHandle,
    RENDER_CONTEXT* Ctx
    );

static
void
CleanupRenderContext(
    RENDER_CONTEXT* Ctx
    );

static
void
Render(
    OVERLAY_CONTEXT* Overlay
    );

static
float
ComputeMaxTextHeight(
    const wchar_t* Text,
    float TextLineHeight
    );

static
void
ReleaseOverlayResources(
    void
    );

//
// Implementations
//

// SetupRenderContext sets up the DirectX related resources for rendering the overlay.
// These are not persisted throughout the systray lifetime, but are released when
// the overlay is no longer visible to minimize the idle memory footprint.
static
void
SetupRenderContext(
    HWND WindowHandle,
    RENDER_CONTEXT* Ctx
    )
{
    const wchar_t* fontFamily = L"Segoe UI";
    const wchar_t* localeName = L"en-us";
    const float fontSize = 12.0f;
    RECT rc;
    D2D1_SIZE_U size;
    D2D1_RENDER_TARGET_PROPERTIES rtProps = {};
    D2D1_HWND_RENDER_TARGET_PROPERTIES hwndProps = {};
    HRESULT hr = E_FAIL;

    ZeroMemory(Ctx, sizeof(*Ctx));

    hr = D2D1CreateFactory(D2D1_FACTORY_TYPE_SINGLE_THREADED, &Ctx->DFactory);
    if (FAILED(hr)) {
        ReportError(hr, "Failed create D2D1.");
        goto Exit;
    }

    if (GetClientRect(WindowHandle, &rc) == FALSE) {
        ReportError(E_FAIL, "Failed get client rect");
        goto Exit;
    }

    size = D2D1::SizeU(rc.right - rc.left, rc.bottom - rc.top);

    rtProps.type = D2D1_RENDER_TARGET_TYPE_DEFAULT;
    rtProps.pixelFormat.format = DXGI_FORMAT_UNKNOWN;
    rtProps.pixelFormat.alphaMode = D2D1_ALPHA_MODE_PREMULTIPLIED;
    rtProps.dpiX = 0.0f;
    rtProps.dpiY = 0.0f;
    rtProps.usage = D2D1_RENDER_TARGET_USAGE_NONE;
    rtProps.minLevel = D2D1_FEATURE_LEVEL_DEFAULT;

    hwndProps.hwnd = WindowHandle;
    hwndProps.pixelSize = size;
    hwndProps.presentOptions = D2D1_PRESENT_OPTIONS_NONE;

    // Create the render target.
    // This consumes the most memory, size 748x460 => 20472 MB.
    hr = Ctx->DFactory->CreateHwndRenderTarget(rtProps,
                                               hwndProps,
                                               &Ctx->RenderTarget);
    if (FAILED(hr)) {
        ReportError(hr, "Failed to create render target.");
        goto Exit;
    }

    // Create solid brushes.
    hr = Ctx->RenderTarget->CreateSolidColorBrush(D2D1::ColorF(1.0f, 1.0f, 1.0f, 1.0f),
                                                  &Ctx->Brush);
    if (FAILED(hr)) {
        ReportError(hr, "Failed to create brush.");
        goto Exit;
    }

    hr = Ctx->RenderTarget->CreateSolidColorBrush(D2D1::ColorF(0.3f, 0.0f, 0.3f, 0.5f),
                                                  &Ctx->BorderBrush);
    if (FAILED(hr)) {
        ReportError(hr, "Failed to create brush.");
        goto Exit;
    }

    hr = Ctx->RenderTarget->CreateSolidColorBrush(D2D1::ColorF(0.5f, 0.5f, 0.5f, 1.0f),
                                                  &Ctx->BarBrush);
    if (FAILED(hr)) {
        ReportError(hr, "Failed to create brush.");
        goto Exit;
    }

    // Create DirectWrite.
    hr = DWriteCreateFactory(DWRITE_FACTORY_TYPE_SHARED,
                            __uuidof(IDWriteFactory),
                             reinterpret_cast<IUnknown**>(&Ctx->DWriteFactory));
    if (FAILED(hr)) {
        ReportError(hr, "Failed to create DWrite.");
        goto Exit;
    }

    hr = Ctx->DWriteFactory->CreateTextFormat(fontFamily,
                                              nullptr,
                                              DWRITE_FONT_WEIGHT_BOLD,
                                              DWRITE_FONT_STYLE_NORMAL,
                                              DWRITE_FONT_STRETCH_NORMAL,
                                              fontSize,
                                              localeName,
                                              &Ctx->MainTextFormat);
    if (FAILED(hr)) {
        ReportError(hr, "Failed to create TextFormat.");
        goto Exit;
    }

    // This aligns the text to be left (or right in RTL) of the layout rect.
    hr = Ctx->MainTextFormat->SetTextAlignment(DWRITE_TEXT_ALIGNMENT_LEADING);
    if (FAILED(hr)) {
        ReportError(hr, "Failed to set text alignment");
        goto Exit;
    }

    // This aligns the text to be top of the layout rect.
    hr = Ctx->MainTextFormat->SetParagraphAlignment(DWRITE_PARAGRAPH_ALIGNMENT_NEAR);
    if (FAILED(hr)) {
        ReportError(hr, "Failed to set paragraph alignment");
        goto Exit;
    }

    hr = Ctx->MainTextFormat->SetWordWrapping(DWRITE_WORD_WRAPPING_WRAP);
    if (FAILED(hr)) {
        ReportError(hr, "Failed to set word wrapping.");
        goto Exit;
    }

    Ctx->TextLineHeight = 15.0f;
    Ctx->TextBaseHeightOffset = 20.0f;
    hr = Ctx->MainTextFormat->SetLineSpacing(DWRITE_LINE_SPACING_METHOD_UNIFORM,
                                             Ctx->TextLineHeight,
                                             Ctx->TextBaseHeightOffset);
    if (FAILED(hr)) {
        ReportError(hr, "Failed to set line spacing.");
        goto Exit;
    }

    hr = Ctx->DWriteFactory->CreateTextFormat(fontFamily,
                                              nullptr,
                                              DWRITE_FONT_WEIGHT_BOLD,
                                              DWRITE_FONT_STYLE_NORMAL,
                                              DWRITE_FONT_STRETCH_NORMAL,
                                              fontSize,
                                              localeName,
                                              &Ctx->BarTextFormat);
    if (FAILED(hr)) {
        ReportError(hr, "Failed to create TextFormat.");
        goto Exit;
    }

    hr = Ctx->BarTextFormat->SetTextAlignment(DWRITE_TEXT_ALIGNMENT_CENTER);
    if (FAILED(hr)) {
        ReportError(hr, "Failed to set text alignment");
        goto Exit;
    }

    hr = Ctx->BarTextFormat->SetParagraphAlignment(DWRITE_PARAGRAPH_ALIGNMENT_CENTER);
    if (FAILED(hr)) {
        ReportError(hr, "Failed to set paragraph alignment");
        goto Exit;
    }

Exit:

    if (FAILED(hr)) {
        CleanupRenderContext(Ctx);
    }

    return;
}

// CleanupRenderContext releases the resources created in SetupRenderContext to
// minimize the memory footprint when idle.
static
void
CleanupRenderContext(
    RENDER_CONTEXT* Ctx
    )
{
    // Order of release matters.
    // DirectX seems to suffer internal memory leak when the release order is not
    // hierarchical.

    if (Ctx->BarTextFormat != nullptr) {
        Ctx->BarTextFormat->Release();
        Ctx->BarTextFormat = nullptr;
    }

    if (Ctx->MainTextFormat != nullptr) {
        Ctx->MainTextFormat->Release();
        Ctx->MainTextFormat = nullptr;
    }

    if (Ctx->DWriteFactory != nullptr) {
        Ctx->DWriteFactory->Release();
        Ctx->DWriteFactory = nullptr;
    }

    if (Ctx->Icon != nullptr) {
        Ctx->Icon->Release();
        Ctx->Icon = nullptr;
     }

    if (Ctx->BarBrush != nullptr) {
        Ctx->BarBrush->Release();
        Ctx->BarBrush = nullptr;
    }

    if (Ctx->BorderBrush != nullptr) {
        Ctx->BorderBrush->Release();
        Ctx->BorderBrush = nullptr;
    }

    if (Ctx->Brush != nullptr) {
        Ctx->Brush->Release();
        Ctx->Brush = nullptr;
    }

    if (Ctx->RenderTarget != nullptr) {
        Ctx->RenderTarget->Release();
        Ctx->RenderTarget = nullptr;
    }

    if (Ctx->DFactory != nullptr) {
        Ctx->DFactory->Release();
        Ctx->DFactory = nullptr;
    }
}

// ReleaseOverlayResources is a wrapper to release the resources created in SetupRenderContext
// and any other complimentary resources to minimize the memory footprint when idle.
// This should be called after the overlay window is hidden.
void
ReleaseOverlayResources(
    OVERLAY_CONTEXT* Overlay
    )
{
    CleanupRenderContext(&Overlay->RenderCtx);

    if (Overlay->TextContent != nullptr) {
        free(Overlay->TextContent);
        Overlay->TextContent = nullptr;
    }

    Overlay->TextContentLength = 0;
}

// Render is responsible for drawing the overlay.
static
void
Render(
    OVERLAY_CONTEXT* Overlay
    )
{
    const float borderPadding = 20.0f;
    const float borderThickness = 2.0f;
    const float borderHalfThickness = borderThickness / 2.0f;
    const float barHeight = 16.0f;
    const wchar_t* tip = L"Save to clipboard with Ctrl+C";

    float width;
    float height;
    HRESULT hr = E_FAIL;
    RECT rc;
    D2D1_RECT_F layoutRect;
    D2D1_RECT_F borderRect;
    D2D1_RECT_F barRect;
    D2D1::Matrix3x2F transform;
    ID2D1HwndRenderTarget* renderTarget = nullptr;
    RENDER_CONTEXT* ctx = &Overlay->RenderCtx;

    // Since render is called very frequently, do not pop message boxes for errors.

    if (ctx->RenderTarget == nullptr) {
        // Create the render context only when ready to draw since
        // the render target consumes a lot of memory.
        CleanupRenderContext(ctx);
        SetupRenderContext(Overlay->WindowHandle, ctx);

        if (ctx->RenderTarget == nullptr) {
            // Still failed, bail out.
            goto Exit;
        }

        ctx->MaxScrollOffset = ComputeMaxTextHeight(Overlay->TextContent,
                                                    ctx->TextLineHeight);
    }

    if (GetClientRect(Overlay->WindowHandle, &rc) == FALSE) {
        // Silently drop. Do not spam errors.
        goto Exit;
    }

    width = (float)(rc.right - rc.left);
    height = (float)(rc.bottom - rc.top);
    renderTarget = ctx->RenderTarget;

    renderTarget->BeginDraw();

    // Transparentcy is cntrolled by SetLayeredWindowAttributes.

    renderTarget->Clear(D2D1::ColorF(0.1f, 0.0f, 0.1f, 1.0f));

    layoutRect = D2D1::RectF(borderPadding,
                             borderPadding,
                             width - borderPadding,
                             height - borderPadding);

    borderRect = D2D1::RectF(borderHalfThickness,
                             borderHalfThickness,
                             width - borderHalfThickness,
                             height - borderHalfThickness);

    barRect = D2D1::RectF(borderThickness,
                          borderThickness,
                          width - borderHalfThickness,
                          barHeight);

    // Clip visible region.
    renderTarget->PushAxisAlignedClip(layoutRect, D2D1_ANTIALIAS_MODE_ALIASED);

    // Vertical scroll offset.
    // This is moving the rect upwards, which means an effective negative translation.
    // Flip the scroll value to negative.
    transform = D2D1::Matrix3x2F::Translation(0, -ctx->ScrollOffset);
    renderTarget->SetTransform(transform);

    if (Overlay->TextContent != nullptr) {
        renderTarget->DrawText(Overlay->TextContent,
                               Overlay->TextContentLength,
                               ctx->MainTextFormat,
                               layoutRect,
                               ctx->Brush);
    }

    renderTarget->SetTransform(D2D1::Matrix3x2F::Identity());
    renderTarget->PopAxisAlignedClip();

    renderTarget->FillRectangle(barRect, ctx->BarBrush);
    renderTarget->DrawRectangle(borderRect, ctx->BorderBrush, borderThickness);

    renderTarget->DrawText(tip,
                           (UINT32)wcslen(tip),
                           ctx->BarTextFormat,
                           barRect,
                           ctx->Brush);

    if (ctx->Icon != nullptr) {
        D2D1_RECT_F iconRect;

        // Should match size in LoadDatadogIcon.
        iconRect = D2D1::RectF(borderThickness,
                               borderThickness,
                               barHeight + borderThickness,
                               barHeight + borderThickness);
        renderTarget->DrawBitmap(ctx->Icon, iconRect, 1.0f);
    }

    hr = renderTarget->EndDraw();
    if (hr == D2DERR_RECREATE_TARGET) {
        // Recreate graphic resources.
        CleanupRenderContext(ctx);
    }

Exit:

    return;
}

// ReportError is a wrapper to redirect error codes and messages.
void
ReportError(
    HRESULT ErrorCode,
    const char* Message
    )
{
    goReportErrorCallback(ErrorCode, (char*)Message);
}

// ComputeMaxTextHeight estimates the height in pixels of the overlay text to
// assign a maximum scroll value.
float
ComputeMaxTextHeight(
    const wchar_t* Text,
    float TextLineHeight
    )
{
    int lineCount = 1;
    float maxHeight;
    wchar_t c;

    if (Text == nullptr) {
        goto Exit;
    }

    while (*Text != L'\0') {
        if (*Text == L'\n') {
            ++lineCount;
        }

        ++Text;
    }

Exit:

    maxHeight = TextLineHeight * ((float)(lineCount));

    return maxHeight;
}

//
// Public functions
//

extern "C"
{

// InitOverlayA initializes the internal state of the overlay.
void
InitOverlayA(
    uintptr_t WindowHandle,
    char* TextContent
    )
{
    ZeroMemory(&g_Overlay, sizeof(g_Overlay));
    g_Overlay.WindowHandle = (HWND)WindowHandle;
    SetOverlayTextA(TextContent);
    return;
}

// ShowOverlay handles the visibility of the overlay on WM_SHOWWINDOW.
void
ShowOverlay(
    uintptr_t WindowHandle,
    BOOL Show
    )
{
    if (Show != FALSE) {
        SetLayeredWindowAttributes((HWND)WindowHandle,
                                   0,
                                   DATADOG_OVERLAY_MAX_ALPHA,
                                   LWA_ALPHA);

        // The parent caller should have setup TextContent.
        // The render context will be (re)created on Render.
    } else {
        SetLayeredWindowAttributes((HWND)WindowHandle,
                                   0,
                                   0,
                                   LWA_ALPHA);

        // Release resources on hide.
        ReleaseOverlayResources(&g_Overlay);
    }
}

// RenderOverlay draws the overlay on WM_PAINT.
void
RenderOverlay(
    uintptr_t WindowHandle
    )
{
    PAINTSTRUCT ps;

    BeginPaint((HWND)WindowHandle, &ps);
    Render(&g_Overlay);
    EndPaint((HWND)WindowHandle, &ps);
}

// CleanupOverlay releases all resources when the overlay is terminated.
void
CleanupOverlay(
    void
    )
{
    ReleaseOverlayResources(&g_Overlay);
    ZeroMemory(&g_Overlay, sizeof(g_Overlay));
}

// CopyOverlayTextToClipboard copies the existing text in the overlay to the Clipboard.
int
CopyOverlayTextToClipboard(
    void
    )
{
    const wchar_t* text;
    wchar_t* buffer = nullptr;
    UINT32 textLength;
    size_t size;
    HGLOBAL memHandle = NULL;
    bool opened = false;
    int err = ERROR_SUCCESS;

    text = g_Overlay.TextContent;
    textLength = g_Overlay.TextContentLength;

    if ((text == nullptr) || (textLength == 0)) {
        goto Exit;
    }

    if (OpenClipboard(nullptr) == FALSE) {
        err = GetLastError();
        goto Exit;
    }

    opened = true;

    size = (textLength + 1) * sizeof(wchar_t);
    memHandle = GlobalAlloc(GMEM_MOVEABLE, size);
    if (memHandle == NULL) {
        err = ERROR_NOT_ENOUGH_MEMORY;
        goto Exit;
    }

    buffer = (wchar_t*)(GlobalLock(memHandle));
    CopyMemory(buffer,text, textLength);
    GlobalUnlock(memHandle);

    EmptyClipboard();

    // Do not free glob after SetClipboardData.
    if (SetClipboardData(CF_UNICODETEXT, memHandle) == NULL) {
        err = GetLastError();
        GlobalFree(memHandle);
        goto Exit;
    }

Exit:

    if (opened) {
        CloseClipboard();
    }

    return err;
}

// SetOverlayTextA sets the text to display in the overlay.
// Do not modify the input text.
void
SetOverlayTextA(
    char* TextContent
    )
{
    wchar_t* content = nullptr;
    int charCount = 0;
    int wideCharCount = 0;

    if (TextContent == nullptr) {
        goto Exit;
    }

    // Sanity check for null termination.
    charCount = strnlen_s(TextContent, 0xFFFE);
    if (charCount == (0xFFFE)) {
        goto Exit;
    }

    wideCharCount = MultiByteToWideChar(CP_UTF8,
                                        0,
                                        TextContent,
                                        -1,
                                        nullptr,
                                        0);
    if (wideCharCount == 0) {
        goto Exit;
    }

    content = (wchar_t*)malloc(sizeof(wchar_t) * wideCharCount);
    if (content == nullptr) {
        ReportError(E_OUTOFMEMORY, "Failed to allocate memory");
        goto Exit;
    }

    if (MultiByteToWideChar(CP_UTF8,
                            0,
                            TextContent,
                            -1,
                            content,
                            wideCharCount) == 0) {
        ReportError(E_FAIL, "Failed to convert to wide string");
        goto Exit;
    }

    if (g_Overlay.TextContent != nullptr) {
        free(g_Overlay.TextContent);
    }

    g_Overlay.TextContentLength = wideCharCount;
    g_Overlay.TextContent = content;
    content = nullptr;

Exit:

    if (content != nullptr) {
        free(content);
    }

    return;
}

// ScrollOverlayVertical updates the vertical position of the content in the overlay for scrolling.
// This responds to VK_UP, VK_DOWN, VK_NEXT, VK_PRIOR.
void
ScrollOverlayVertical(
    float Delta
    )
{
    RENDER_CONTEXT& ctx = g_Overlay.RenderCtx;
    float newOffset = ctx.ScrollOffset + Delta;

    if (newOffset < 0) {
        newOffset = 0;
    } else if(newOffset > ctx.MaxScrollOffset) {
        newOffset = ctx.MaxScrollOffset;
    }

    if (newOffset != ctx.ScrollOffset) {
        ctx.ScrollOffset = newOffset;
        InvalidateRect(g_Overlay.WindowHandle, NULL, FALSE);
    }
}

// ScrollOverlayVertical sets the vertical position of the content in the overlay to
// the top-most or bottom-most position. This responds to VK_HOME, VK_END.
void
ScrollOverlayToEnd(
    BOOL Front
    )
{
    float newOffset;

    if (Front != FALSE) {
        newOffset = 0.0f;
    } else {
        newOffset = g_Overlay.RenderCtx.MaxScrollOffset;
    }

    if (newOffset != g_Overlay.RenderCtx.ScrollOffset) {
        g_Overlay.RenderCtx.ScrollOffset = newOffset;
        InvalidateRect(g_Overlay.WindowHandle, NULL, FALSE);
    }
}

}
