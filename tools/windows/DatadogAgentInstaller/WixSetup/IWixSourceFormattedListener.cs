namespace WixSetup
{
    public interface IWixSourceFormattedListener
    {
        void OnWixSourceFormatted(ref string content);
    }
}
