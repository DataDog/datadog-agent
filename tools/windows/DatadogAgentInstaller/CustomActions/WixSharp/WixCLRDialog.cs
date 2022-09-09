using System;
using System.Collections.Generic;
using System.ComponentModel;
using System.Diagnostics;
using System.Drawing;
using System.IO;
using System.Linq;
using System.Runtime.InteropServices;
using System.Text;
using System.Windows.Forms;
using Microsoft.Deployment.WindowsInstaller;

namespace Datadog.CustomActions.WixSharp
{
    /// <summary>
    /// Defines System.Windows.Forms.<see cref="T:System.Windows.Forms.Form" />, which is to be used as the  for custom MSI dialog.
    /// <para>
    /// As opposite to the WixSharp.<see cref="T:WixSharp.WixForm" /> based custom dialogs <c>WixCLRDialog</c> is instantiated not at 
    /// compile but at run time. Thus it is possible to implement comprehensive deployment algorithms in any of the available Form event handlers.
    /// </para>
    /// <para>
    /// The usual usability scenario is the injection of the managed Custom Action (for displaying the <c>WixCLRDialog</c>) 
    /// into the sequence of the standard dialogs (WixSharp.<see cref="T:WixSharp.CustomUI"/>). 
    /// </para>
    /// <para>
    /// While it is possible to construct <see cref="T:WixSharp.CustomUI"/> instance manually it is preferred to use 
    /// Factory methods of  <see cref="T:WixSharp.CustomUIBuilder"/> (e.g. InjectPostLicenseClrDialog) for this.
    /// </para>
    /// <code>
    /// static public void Main()
    /// {
    ///     ManagedAction showDialog;
    /// 
    ///     var project = new Project("CustomDialogTest",
    ///                                 showDialog = new ShowClrDialogAction("ShowProductActivationDialog"));
    /// 
    ///     project.UI = WUI.WixUI_Common;
    ///     project.CustomUI = CustomUIBuilder.InjectPostLicenseClrDialog(showDialog.Id, " LicenseAccepted = \"1\"");
    ///     
    ///     Compiler.BuildMsi(project);
    /// }
    /// ...
    /// public class CustomActions
    /// {
    ///     [CustomAction]
    ///     public static ActionResult ShowProductActivationDialog(Session session)
    ///     {
    ///         return WixCLRDialog.ShowAsMsiDialog(new CustomDialog(session));
    ///     }
    /// }
    /// ...
    /// public partial class CustomDialog : WixCLRDialog
    /// {
    ///     private GroupBox groupBox1;
    ///     private Button cancelBtn;
    ///     ... 
    /// </code>
    /// <para>
    /// The all communications with the installation in progress are to be done by modifying the MSI properties or executing MSI actions 
    /// via <c>Session</c> object.</para>
    /// <para>
    /// When closing the dialog make sure you set the DeialogResul properly. <c>WixCLRDialog</c> offers three predefined routines for setting the 
    /// DialogResult:
    /// <para>- MSINext</para>
    /// <para>- MSIBack</para>
    /// <para>- MSICancel</para>
    /// By invoking these routines from the corresponding event handlers you can control your MSI UI sequence:
    /// <code>
    /// void cancelBtn_Click(object sender, EventArgs e)
    /// {
    ///     MSICancel();
    /// }
    /// 
    /// void nextBtn_Click(object sender, EventArgs e)
    /// {
    ///     MSINext();
    /// }
    /// 
    /// void backBtn_Click(object sender, EventArgs e)
    /// {
    ///     MSIBack();
    /// }
    /// </code>
    /// </para>
    /// </summary>
    public partial class WixCLRDialog : Form
    {
        /// <summary>
        /// The MSI session
        /// </summary>
        public Session session;

        /// <summary>
        /// The WIN32 handle to the host window (parent MSI dialog).
        /// </summary>
        protected IntPtr hostWindow;

        /// <summary>
        /// Initializes a new instance of the <see cref="WixCLRDialog"/> class.
        /// <remarks>
        /// This constructor is to be used by the Visual Studio Form designer only. 
        /// You should always use <c>WixCLRDialog(Session session)</c> constructor instead.
        /// </remarks>
        /// </summary>
        public WixCLRDialog()
        {
            InitializeComponent();
        }

        /// <summary>
        /// Initializes a new instance of the <see cref="WixCLRDialog"/> class.
        /// </summary>
        /// <param name="session">The session.</param>
        public WixCLRDialog(Session session)
        {
            this.session = session;
            InitializeComponent();
        }

        void InitializeComponent()
        {
            try
            {
                Application.EnableVisualStyles();
                Application.SetCompatibleTextRenderingDefault(false);
            }
            catch { }

            if (LicenseManager.UsageMode != LicenseUsageMode.Designtime)
            {
                try
                {
                    if (this.DesignMode)
                        return;

                    this.VisibleChanged += form_VisibleChanged;
                    this.FormClosed += WixPanel_FormClosed;
                    this.Load += WixDialog_Load;
                    Init();
                }
                catch { }
            }
        }

        void WixDialog_Load(object sender, EventArgs e)
        {
            this.Text = Win32.GetWindowText(this.hostWindow);
        }

        /// <summary>
        /// Inits this instance.
        /// </summary>
        protected void Init()
        {
            //Debug.Assert(false);

            this.hostWindow = GetMsiForegroundWindow();

            this.Opacity = 0.0005;
            this.Text = Win32.GetWindowText(this.hostWindow);

#if DEBUG

            //System.Diagnostics.Debugger.Launch();
#endif
            foreach (Process p in Process.GetProcessesByName("msiexec"))
                try
                {
                    this.Icon = Icon.ExtractAssociatedIcon(p.MainModule.FileName); //service process throws on accessing MainModule
                    break;
                }
                catch { }
        }

        /// <summary>
        /// Gets the msi foreground window.
        /// </summary>
        /// <returns></returns>
        protected virtual IntPtr GetMsiForegroundWindow()
        {
            Process proc = null;

            var bundleUI = Environment.GetEnvironmentVariable("WIXSHARP_SILENT_BA_PROC_ID");
            if (bundleUI != null)
            {
                int id = 0;
                if (int.TryParse(bundleUI, out id))
                    proc = Process.GetProcessById(id);
            }

            var bundlePath = session["WIXBUNDLEORIGINALSOURCE"];
            if (!string.IsNullOrEmpty(bundlePath))
            {
                try
                {
                    proc = Process.GetProcessesByName(Path.GetFileNameWithoutExtension(bundlePath)).Where(p => p.MainWindowHandle != IntPtr.Zero).FirstOrDefault();
                }
                catch { }
            }

            if (proc == null)
            {
                proc = Process.GetProcessesByName("msiexec").Where(p => p.MainWindowHandle != IntPtr.Zero).FirstOrDefault();
            }

            if (proc != null)
            {
                //IntPtr handle = proc.MainWindowHandle; //old algorithm

                IntPtr handle = MsiWindowFinder.GetMsiDialog(proc.Id);

                Win32.ShowWindow(handle, Win32.SW_RESTORE);
                Win32.SetForegroundWindow(handle);

                return handle;
            }
            else return IntPtr.Zero;
        }


        /// <summary>
        /// There is some strange resizing artifact (at least on Win7) when MoveWindow does not resize the window accurately.
        /// Thus special adjustment ('delta') is needed to fix the problem.
        /// <para>
        /// The delta value is used in the ReplaceHost method.</para>
        /// </summary>
        protected int delta = 4;


        /// <summary>
        /// 'Replaces' the current step dialog with the "itself".
        /// <para>It uses WIN32 API to hide the parent native MSI dialog and place managed form dialog (itself)
        /// at the same desktop location and with the same size as the parent.</para>
        /// </summary>
        protected void ReplaceHost()
        {
            try
            {
                Win32.RECT r;
                Win32.GetWindowRect(hostWindow, out r);

                Win32.MoveWindow(this.Handle, r.Left - delta, r.Top - delta, r.Right - r.Left + delta * 2, r.Bottom - r.Top + delta * 2, true);

                this.Opacity = 1;
                Application.DoEvents();

                this.MaximumSize =
                    this.MinimumSize = new Size(this.Width, this.Height); //prevent resizing

                hostWindow.Hide();
            }
            catch (Exception ex)
            {
                MessageBox.Show(ex.ToString());
            }
        }

        /// <summary>
        /// Restores parent native MSI dialog after the previous <c>ReplaceHost</c> call.
        /// </summary>
        protected void RestoreHost()
        {
            Win32.RECT r;
            Win32.GetWindowRect(this.Handle, out r);

            Win32.RECT rHost;
            Win32.GetWindowRect(hostWindow, out rHost);

            Win32.MoveWindow(hostWindow, r.Left + delta, r.Top + delta, rHost.Right - rHost.Left, rHost.Bottom - rHost.Top, true);
            hostWindow.Show();
            this.Opacity = 0.01;

            Application.DoEvents();
        }

        private void WixPanel_FormClosed(object sender, FormClosedEventArgs e)
        {
            RestoreHost();
        }

        bool initialized = false;

        private void form_VisibleChanged(object sender, EventArgs e)
        {
            if (Visible)
            {
                if (!initialized)
                {
                    initialized = true;
                    ReplaceHost();
                    this.Visible = true;
                    Application.DoEvents();
                }
            }
        }

        /// <summary>
        /// Closes the dialog and sets the <c>this.DialogResult</c> to the 'DialogResult.Cancel' value ensuring the 
        /// setup is canceled.
        /// </summary>
        public void MSICancel()
        {
            this.DialogResult = DialogResult.Cancel;
            Close();
        }

        /// <summary>
        /// Closes the dialog and sets the <c>this.DialogResult</c> to the 'DialogResult.Retry' value ensuring the 
        /// setup is resumed with the previous UI sequence dialog is displayed.
        /// </summary>
        public void MSIBack()
        {
            this.DialogResult = DialogResult.Retry;
            Close();
        }

        /// <summary>
        /// Closes the dialog and sets the <c>this.DialogResult</c> to the 'DialogResult.OK' value ensuring the 
        /// setup is resumed and the UI sequence advanced to the next step.
        /// </summary>
        public void MSINext()
        {
            this.DialogResult = DialogResult.OK;
            Close();
        }

        /// <summary>
        /// Shows as specified managed dialog.
        /// <para>It uses WIN32 API to hide the parent native MSI dialog and place managed form dialog
        /// at the same desktop location and with the same size as the parent.</para>
        /// <para>It also ensures that after the managed dialog is closed the proper ActionResult is returned.</para>
        /// </summary>
        /// <param name="dialog">The dialog.</param>
        /// <returns>ActionResult value</returns>
        public static ActionResult ShowAsMsiDialog(WixCLRDialog dialog)
        {
            ActionResult retval = ActionResult.Success;

            try
            {
                using (dialog)
                {
                    DialogResult result = dialog.ShowDialog();
                    if (result == DialogResult.OK)
                    {
                        dialog.session["Custom_UI_Command"] = "next";
                        retval = ActionResult.Success;
                    }
                    else if (result == DialogResult.Cancel)
                    {
                        dialog.session["Custom_UI_Command"] = "abort";
                        retval = ActionResult.UserExit;
                    }
                    if (result == DialogResult.Retry)
                    {
                        dialog.session["Custom_UI_Command"] = "back";
                        retval = ActionResult.Success;
                    }
                }
            }
            catch (Exception e)
            {
                dialog.session.Log("Error: " + e.ToString());
                retval = ActionResult.Failure;
            }

#if DEBUG

            //System.Diagnostics.Debugger.Launch();
#endif

            return retval;
        }

        /// <summary>
        /// Gets the embedded MSI binary stream.
        /// </summary>
        /// <param name="binaryId">The binary id.</param>
        /// <returns>Stream instance</returns>
        public Stream GetMSIBinaryStream(string binaryId)
        {
            using (var sqlView = this.session.Database.OpenView("select Data from Binary where Name = '" + binaryId + "'"))
            {
                sqlView.Execute();
                Stream data = sqlView.Fetch().GetStream(1);

                var retval = new MemoryStream();

                int Length = 256;
                var buffer = new Byte[Length];
                int bytesRead = data.Read(buffer, 0, Length);
                while (bytesRead > 0)
                {
                    retval.Write(buffer, 0, bytesRead);
                    bytesRead = data.Read(buffer, 0, Length);
                }
                return retval;
            }
        }
    }

    internal class MsiWindowFinder
    {
        public static IntPtr GetMsiDialog(int processId)
        {
            var windows = EnumerateProcessWindowHandles(processId).Select(h =>
            {
                var message = new StringBuilder(1000);
                SendMessage(h, WM_GETTEXT, message.Capacity, message);
                var wndText = message.ToString();
                message.Length = 0; //clear prev content

                GetClassName(h, message, message.Capacity);
                var wndClass = message.ToString();

                return new { Class = wndClass, Title = wndText, Handle = h };
            });

            var interactiveWindows = windows.Where(x => !string.IsNullOrEmpty(x.Title));

            if (interactiveWindows.Any())
            {
                if (interactiveWindows.Count() == 1)
                    return interactiveWindows.First().Handle;
                else
                {
                    var wellKnownDialogWindow = interactiveWindows.FirstOrDefault(x => x.Class == "MsiDialogCloseClass");
                    if (wellKnownDialogWindow != null)
                        return wellKnownDialogWindow.Handle;
                }
            }

            return IntPtr.Zero;
        }

        private const uint WM_GETTEXT = 0x000D;

        [DllImport("user32.dll", CharSet = CharSet.Auto)]
        static extern IntPtr SendMessage(IntPtr hWnd, uint Msg, int wParam, StringBuilder lParam);

        [DllImport("user32.dll")]
        static extern int GetClassName(IntPtr hWnd, StringBuilder lpClassName, int nMaxCount);

        delegate bool EnumThreadDelegate(IntPtr hWnd, IntPtr lParam);

        [DllImport("user32.dll")]
        static extern bool EnumThreadWindows(int dwThreadId, EnumThreadDelegate lpfn,
            IntPtr lParam);

        static IEnumerable<IntPtr> EnumerateProcessWindowHandles(int processId)
        {
            var handles = new List<IntPtr>();

            foreach (ProcessThread thread in Process.GetProcessById(processId).Threads)
                EnumThreadWindows(thread.Id,
                    (hWnd, lParam) => { handles.Add(hWnd); return true; }, IntPtr.Zero);

            return handles;
        }
    }
//public static class ProcessExtensions
//{
//    private static string FindIndexedProcessName(int pid)
//    {
//        var processName = Process.GetProcessById(pid).ProcessName;
//        var processesByName = Process.GetProcessesByName(processName);
//        string processIndexdName = null;

//        for (var index = 0; index < processesByName.Length; index++)
//        {
//            processIndexdName = index == 0 ? processName : processName + "#" + index;
//            var processId = new PerformanceCounter("Process", "ID Process", processIndexdName);
//            if ((int) processId.NextValue() == pid)
//            {
//                return processIndexdName;
//            }
//        }

//        return processIndexdName;
//    }

//    private static Process FindPidFromIndexedProcessName(string indexedProcessName)
//    {
//        var parentId = new PerformanceCounter("Process", "Creating Process ID", indexedProcessName);
//        return Process.GetProcessById((int) parentId.NextValue());
//    }

//    public static Process Parent(this Process process)
//    {
//        return FindPidFromIndexedProcessName(FindIndexedProcessName(process.Id));
//    }
//}
}
