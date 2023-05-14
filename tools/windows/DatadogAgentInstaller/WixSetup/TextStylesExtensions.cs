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
                // font.FontFamily.Name may be substituted by OS with the compatible font name
                // this can happen when we build in a container.
                // See: https://github.com/oleg-shilo/wixsharp/commit/7bdbfa30ed13d1f2c319ee4c42d374f7ac2c9134
                new XAttribute("FaceName", font.OriginalFontName),
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
