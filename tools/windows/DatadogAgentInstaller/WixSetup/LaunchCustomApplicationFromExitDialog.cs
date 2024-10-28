using System.Linq;
using WixSharp;

namespace WixSetup
{
    public class LaunchCustomApplicationFromExitDialog : WixEntity, IGenericEntity
    {
        private readonly LaunchApplicationFromExitDialog _launchApplicationFromExitDialog;

        /// <summary>
        /// CheckBox text. <br />
        /// Default value is <value>"Launch"</value>.
        /// </summary>
        public string Description
        {
            get => _launchApplicationFromExitDialog.Description;
            set => _launchApplicationFromExitDialog.Description = value;
        }

        /// <summary>Exe ID.</summary>
        public string ExeId => _launchApplicationFromExitDialog.ExeId;

        public string ExeCommand { get; }
        public string Directory { get; }

        public LaunchCustomApplicationFromExitDialog(string exeId, string description, string directory, string exeCommand)
        {
            _launchApplicationFromExitDialog = new LaunchApplicationFromExitDialog(exeId, description);
            ExeCommand = exeCommand;
            Directory = directory;
        }

        public void Process(ProcessingContext context)
        {
            _launchApplicationFromExitDialog.Process(context);
            var customAction = context.XParent
                .FindAll("CustomAction")
                .First(x => x.HasAttribute("Id", value => value == "LaunchApplication"));
            customAction.RemoveAttributes();
            customAction.SetAttributes($"Id=LaunchApplication;Directory={Directory};Execute=immediate;Return=asyncNoWait;ExeCommand={ExeCommand}");
        }
    }
}
