using System.Xml.Linq;

namespace WixSetup
{
    public interface IWixSourceGeneratedListener
    {
        void OnWixSourceGenerated(XDocument document);
    }
}
