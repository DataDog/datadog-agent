using WixSharp;

namespace WixSetup
{
    public interface IWixProjectEvents
    {
        /// <summary>
        /// Occurs when WiX source code generated. Use this event if you need to modify generated XML (XDocument)
        /// before it is compiled into MSI.
        /// </summary>
        event XDocumentGeneratedDlgt WixSourceGenerated;

        /// <summary>
        /// Occurs when WiX source file is saved. Use this event if you need to do any post-processing of the generated/saved file.
        /// </summary>
        event XDocumentSavedDlgt WixSourceSaved;

        /// <summary>
        /// Occurs when WiX source file is formatted and ready to be saved. Use this event if you need to do any custom formatting
        /// of the XML content before it is saved by the compiler.
        /// </summary>
        event XDocumentFormatedDlgt WixSourceFormated;
    }
}
