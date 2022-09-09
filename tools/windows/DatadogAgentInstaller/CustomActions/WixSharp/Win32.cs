using System;
using System.Drawing;
using System.Runtime.InteropServices;
using System.Text;
using System.Windows.Forms;

namespace Datadog.CustomActions.WixSharp
{
#pragma warning disable 1591

    /// <summary>
    /// Set of Win32 API wrappers
    /// </summary>
    public static class Win32
    {
        public const int SW_HIDE = 0;
        public const int SW_SHOW = 1;
        public const int SW_RESTORE = 9;

        [DllImport("user32", EntryPoint = "SendMessage")]
        public extern static int SendMessage(IntPtr hwnd, uint msg, uint wParam, uint lParam);

        [DllImport("user32.dll")]
        public static extern bool ShowWindow(IntPtr hWnd, int nCmdShow);

        public static string GetWindowText(IntPtr wnd)
        {
            int length = GetWindowTextLength(wnd);
            StringBuilder sb = new StringBuilder(length + 1);
            GetWindowText(wnd, sb, sb.Capacity);
            return sb.ToString();
        }

        public static void Hide(this IntPtr wnd)
        {
            ShowWindow(wnd, SW_HIDE);
        }

        public static void Show(this IntPtr wnd)
        {
            ShowWindow(wnd, SW_SHOW);
        }

        public static void MoveToMiddleOf(this IntPtr wnd, Form refForm)
        {
            if (wnd != IntPtr.Zero)
                wnd.MoveToMiddleOf(refForm.Bounds);
        }

        public static void MoveToMiddleOf(this Form form, IntPtr refWnd)
        {
            if (refWnd != IntPtr.Zero)
                form.Handle.MoveToMiddleOf(refWnd.GetRectangle());
        }

        public static void MoveToMiddleOf(this IntPtr wnd, Rectangle refRect)
        {
            var rect = wnd.GetRectangle();
            var center = refRect;
            center.Offset(center.Width / 2, center.Height / 2);
            center.Offset(-rect.Width / 2, -rect.Height / 2);

            MoveWindow(wnd, center.Left, center.Top, rect.Width, rect.Height, true);
        }

        [DllImport("user32.dll", SetLastError = true)]
        public static extern IntPtr FindWindow(string className, string windowName);

        public static Rectangle GetRectangle(this IntPtr hWnd)
        {
            var rect = new RECT();
            GetWindowRect(hWnd, out rect);
            return new Rectangle { X = rect.Left, Y = rect.Top, Width = rect.Right - rect.Left + 1, Height = rect.Bottom - rect.Top + 1 };
        }

        public static bool ShowWindow(IntPtr hWnd, bool show)
        {
            return ShowWindow(hWnd, show ? SW_SHOW : SW_HIDE);
        }

        [DllImport("user32.dll", CharSet = CharSet.Auto)]
        public static extern int GetWindowText(IntPtr hWnd, StringBuilder lpString, int nMaxCount);

        [DllImport("user32.dll", CharSet = CharSet.Auto)]
        public static extern int GetWindowTextLength(IntPtr hWnd);

        [DllImport("user32.dll", SetLastError = true)]
        public static extern long SetParent(IntPtr hWndChild, IntPtr hWndNewParent);

        [DllImport("user32.dll")]
        public static extern IntPtr GetForegroundWindow();

        [DllImport("user32.dll", SetLastError = true)]
        public static extern IntPtr SetActiveWindow(IntPtr hWnd);

        [DllImport("User32.dll")]
        public static extern int SetForegroundWindow(IntPtr hwnd);

        [DllImport("user32.dll", EntryPoint = "GetWindowLongA", SetLastError = true)]
        public static extern long GetWindowLong(IntPtr hwnd, int nIndex);

        [DllImport("user32.dll", SetLastError = true)]
        public static extern bool MoveWindow(IntPtr hwnd, int x, int y, int cx, int cy, bool repaint);

        [DllImport("user32.dll")]
        public static extern bool GetWindowRect(IntPtr hWnd, out RECT lpRect);

        [StructLayout(LayoutKind.Sequential)]
        public struct RECT
        {
            public int Left;
            public int Top;
            public int Right;
            public int Bottom;
        }
    }

#pragma warning restore 1591
}
