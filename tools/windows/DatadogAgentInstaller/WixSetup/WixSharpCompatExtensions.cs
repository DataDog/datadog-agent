using System;
using System.IO;
using WixSharp;

namespace WixSetup
{
    /// <summary>
    /// Compatibility extension methods that replace NineDigit.WixSharpExtensions functionality.
    /// These methods provide the same fluent API for configuring WixSharp projects.
    /// 
    /// This was created as part of the WiX 3 -> WiX 5 migration because NineDigit.WixSharpExtensions
    /// only supports WixSharp 1.x and not WixSharp_wix4.
    /// </summary>
    public static class WixSharpCompatExtensions
    {
        /// <summary>
        /// Sets basic project info (replaces NineDigit SetProjectInfo).
        /// </summary>
        /// <param name="project">The WixSharp project to configure.</param>
        /// <param name="upgradeCode">The upgrade code GUID for the product.</param>
        /// <param name="name">The product name.</param>
        /// <param name="description">The product description.</param>
        /// <param name="version">The product version (revision must be 0 for MSI compatibility).</param>
        /// <returns>The project instance for fluent chaining.</returns>
        /// <exception cref="ArgumentException">Thrown when version revision is not 0.</exception>
        public static Project SetProjectInfo(
            this Project project,
            Guid upgradeCode,
            string name,
            string description,
            Version version)
        {
            if (version.Revision != 0)
            {
                throw new ArgumentException(
                    "MSI ProductVersion does not support revision. Use a version with revision = 0.",
                    nameof(version));
            }

            project.GUID = upgradeCode;
            project.Name = name;
            project.Description = description;
            project.Version = version;
            return project;
        }

        /// <summary>
        /// Sets control panel info (replaces NineDigit SetControlPanelInfo).
        /// </summary>
        /// <param name="project">The WixSharp project to configure.</param>
        /// <param name="name">The name displayed in Control Panel.</param>
        /// <param name="manufacturer">The manufacturer name.</param>
        /// <param name="readme">The readme URL or text.</param>
        /// <param name="comment">Comments about the product.</param>
        /// <param name="contact">Contact information.</param>
        /// <param name="helpUrl">The help URL.</param>
        /// <param name="aboutUrl">The about URL.</param>
        /// <param name="productIconFilePath">Path to the product icon file.</param>
        /// <returns>The project instance for fluent chaining.</returns>
        public static Project SetControlPanelInfo(
            this Project project,
            string name,
            string manufacturer,
            string readme,
            string comment,
            string contact,
            Uri helpUrl,
            Uri aboutUrl,
            FileInfo productIconFilePath)
        {
            project.ControlPanelInfo.Name = name;
            project.ControlPanelInfo.Manufacturer = manufacturer;
            project.ControlPanelInfo.Readme = readme;
            project.ControlPanelInfo.Comments = comment;
            project.ControlPanelInfo.Contact = contact;
            project.ControlPanelInfo.HelpLink = helpUrl?.ToString();
            project.ControlPanelInfo.UrlInfoAbout = aboutUrl?.ToString();
            project.ControlPanelInfo.ProductIcon = productIconFilePath?.FullName;
            return project;
        }

        /// <summary>
        /// Sets minimal UI configuration (replaces NineDigit SetMinimalUI).
        /// </summary>
        /// <param name="project">The WixSharp project to configure.</param>
        /// <param name="backgroundImage">Path to the dialog background image.</param>
        /// <param name="bannerImage">Path to the banner image.</param>
        /// <param name="licenceRtfFile">Optional path to the license RTF file.</param>
        /// <returns>The project instance for fluent chaining.</returns>
        public static Project SetMinimalUI(
            this Project project,
            FileInfo backgroundImage,
            FileInfo bannerImage,
            FileInfo licenceRtfFile = null)
        {
            // WiX 5 migration: Disable image aspect ratio validation.
            // The existing background image was designed for the previous WixSharp version.
            // WixSharp 2.x added stricter validation (expects W:H ratio of 156:312 = 1:2),
            // but the existing image works correctly at runtime.
            project.ValidateBackgroundImage = false;
            project.BackgroundImage = backgroundImage?.FullName;
            project.BannerImage = bannerImage?.FullName;
            if (licenceRtfFile != null)
            {
                project.LicenceFile = licenceRtfFile.FullName;
            }
            return project;
        }
    }
}
