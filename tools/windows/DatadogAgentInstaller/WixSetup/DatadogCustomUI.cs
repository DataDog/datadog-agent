using System.Drawing;
using System.Xml.Linq;
using WixSharp;
using WixSharp.Controls;

namespace WixSetup
{
    public abstract class DatadogCustomUI : CustomUI
    {
        protected CustomUI OnFreshInstall(string dialog, string control, params DialogAction[] handlers)
        {
            handlers.ForEach(h => h.Condition &= Conditions.FirstInstall);
            return On(dialog, control, handlers);
        }

        protected CustomUI OnUpgrade(string dialog, string control, params DialogAction[] handlers)
        {
            handlers.ForEach(h => h.Condition &= Conditions.Upgrading);
            return On(dialog, control, handlers);
        }

        protected CustomUI OnMaintenance(string dialog, string control, params DialogAction[] handlers)
        {
            handlers.ForEach(h => h.Condition &= Conditions.Maintenance);
            return On(dialog, control, handlers);
        }


        protected DatadogCustomUI(IWixProjectEvents wixProjectEvents)
        {
            wixProjectEvents.WixSourceGenerated += OnWixSourceGenerated;
        }

        protected void OnWixSourceGenerated(XDocument document)
        {
            var ui = document.Root.Select("Product/UI");
            // Need to customize here since color is not supported with standard methods
            ui.AddTextStyle("WixUI_Font_Normal_White", new Font("Tahoma", 8), Color.White);
            ui.AddTextStyle("WixUI_Font_Bigger_White", new Font("Tahoma", 12), Color.White);
            ui.AddTextStyle("WixUI_Font_Title_White", new Font("Tahoma", 9, FontStyle.Bold), Color.White);
        }
    }
}
