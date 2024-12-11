using System.Xml.Linq;
using WixSharp;

namespace WixSetup
{
    public enum InstallEvent
    {
        install,
        uninstall,
        both
    }

    /// <summary>
    /// WixSharp integration for util:RemoveFolderEx extension
    /// https://wixtoolset.org/docs/v3/xsd/util/removefolderex/
    /// https://www.hass.de/content/wix-how-use-removefolderex-your-xml-scripts
    /// </summary>
    public class RemoveFolderEx : WixEntity, IGenericEntity
    {
        [Xml] public InstallEvent? On;

        [Xml] public string Property;

        [Xml]
        public new string Id
        {
            get => base.Id;
            set => base.Id = value;
        }

        public void Process(ProcessingContext context)
        {
            // ensure WixUtilExtension.dll is included
            context.Project.Include(WixExtension.Util);
            XElement element = this.ToXElement((WixExtension.Util.ToXName("RemoveFolderEx")));
            context.XParent
                .FindFirst("Component")
                .Add(element);
        }
    }
}
