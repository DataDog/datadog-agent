using System.Drawing;
using System.Xml.Linq;

namespace WixSetup
{
    public static class TextStylesExtensions
    {
        public static XElement AddTextStyle(this XElement ui, string id, Font font, Color color)
        {
            ui.Add(new XElement("TextStyle",
                new XAttribute("Id", id),
                new XAttribute("FaceName", font.FontFamily.Name),
                new XAttribute("Size", font.Size),
                new XAttribute("Red", color.R),
                new XAttribute("Green", color.G),
                new XAttribute("Blue", color.B),
                new XAttribute("Bold", font.Bold ? "yes" : "no"),
                new XAttribute("Italic", font.Italic ? "yes" : "no"),
                new XAttribute("Strike", font.Strikeout ? "yes" : "no"),
                new XAttribute("Underline", font.Underline ? "yes" : "no")
            ));
            return ui;
        }
    }
}
