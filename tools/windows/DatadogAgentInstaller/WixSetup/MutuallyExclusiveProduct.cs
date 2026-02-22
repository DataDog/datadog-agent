using System;
using System.Xml.Linq;
using WixSharp;

namespace WixSetup
{
    internal class MutuallyExclusiveProducts : WixEntity, IGenericEntity
    {
        private static int usageCounter = 0;

        public Guid UpgradeCode { get; set; }
        public string ProductName { get; set; }

        public MutuallyExclusiveProducts()
        {
        }

        public MutuallyExclusiveProducts(string productName, Guid upgradeCode)
        {
            UpgradeCode = upgradeCode;
            ProductName = productName;
        }

        /// <summary>
        /// Adds elements to the WiX to enforce that the product cannot be installed at the same time as another product.
        /// </summary>
        /// <remarks>
        /// The FindRelatedProducts action will set a property if a product matching the provided UpgradeCode
        /// is found on the system. We check this property in a LaunchCondition to prevent installation.
        /// See https://learn.microsoft.com/en-us/windows/win32/msi/findrelatedproducts-action
        ///
        /// Example WiX:
        ///   <Upgrade Id="PUT-GUID-HERE">
        ///     <UpgradeVersion Minimum="0.0.0" IncludeMinimum="yes" OnlyDetect="yes" Property="MUTUALLY_EXCLUSIVE_PRODUCTS_1" />
        ///   </Upgrade>
        ///   <Condition Message="This product cannot be installed at the same time as [ProductName]. Please uninstall [ProductName] before continuing.">NOT MUTUALLY_EXCLUSIVE_PRODUCTS_1</Condition>
        ///   <Property Id="MUTUALLY_EXCLUSIVE_PRODUCTS_1" Secure="yes" />
        /// </remarks>
        public void Process(ProcessingContext context)
        {
            // Append a unique number to the property name.
            //
            // Windows Installer appends each product code found to the property
            // so we could use a single property for all mutually exclusive products,
            // but using a unique property for each product lets us include the product name
            // in the condition message, which makes the message more user-friendly.
            // https://learn.microsoft.com/en-us/windows/win32/msi/upgrade-table
            usageCounter++;
            var propertyName = $"MUTUALLY_EXCLUSIVE_PRODUCTS_{usageCounter}";

            var upgradeElement = new XElement("Upgrade");
            upgradeElement.SetAttributeValue("Id", UpgradeCode);

            // Omitting Maximum means "no upper bound" - detects all versions >= Minimum.
            var upgradeVersionElement = new XElement("UpgradeVersion",
                new XAttribute("Minimum", "0.0.0"),
                new XAttribute("IncludeMinimum", "yes"),
                new XAttribute("OnlyDetect", "yes"),
                new XAttribute("Property", propertyName)
            );
            upgradeElement.Add(upgradeVersionElement);
            context.XParent.Add(upgradeElement);

            // WiX 5 migration: Condition element under Package was replaced with Launch element
            var conditionElement = new XElement("Launch",
                new XAttribute("Message",
                $"This product cannot be installed at the same time as {ProductName}. Please uninstall {ProductName} before continuing."),
                new XAttribute("Condition", $"NOT {propertyName}"));
            context.XParent.Add(conditionElement);

            // The property specified in this column must be a public property and the
            // package author must add the property to the SecureCustomProperties property.
            // https://learn.microsoft.com/en-us/windows/win32/msi/upgrade-table
            var propertyElement = new XElement("Property",
                new XAttribute("Id", propertyName),
                new XAttribute("Secure", "yes")
            );
            context.XParent.Add(propertyElement);
        }
    }
}
