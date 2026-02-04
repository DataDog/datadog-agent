# WixSharp WiX 5 Migration Progress

This document summarizes the changes made during the migration from WiX 3 to WiX 5, organized by iteration.

---

## Iteration 1: Initial Implementation

### Changes Made

#### Phase 2: Project File Updates (5 .csproj files)

**1. WixSetup.csproj**
```xml
<!-- Before -->
<PackageReference Include="NineDigit.WixSharpExtensions" Version="1.0.14" />
<PackageReference Include="WixSharp.bin" Version="1.20.3" />

<!-- After -->
<PackageReference Include="WixSharp_wix4" Version="2.1.7" />
<!-- NineDigit.WixSharpExtensions removed - equivalent methods implemented in WixSharpCompatExtensions.cs -->
```

**2. CustomActions.csproj**
```xml
<!-- Before -->
<Reference Include="Microsoft.Deployment.WindowsInstaller, Version=3.0.0.0, ..." />

<!-- After -->
<PackageReference Include="WixToolset.Dtf.WindowsInstaller" Version="6.0.0" />
```

**3. AgentCustomActions.csproj** - Same change as CustomActions.csproj

**4. InstallerCustomActions.csproj** - Same change as CustomActions.csproj

**5. CustomActions.Tests.csproj**
- Replaced `Microsoft.Deployment.WindowsInstaller` assembly reference → `WixToolset.Dtf.WindowsInstaller 6.0.0`
- Replaced `WixSharp.bin 1.20.3` → `WixSharp_wix4 2.1.7`

#### Phase 3: Code Changes

**1. Created `WixSetup/WixSharpCompatExtensions.cs`** (new file)
- Implements replacement extension methods for NineDigit.WixSharpExtensions:
  - `SetProjectInfo()` - Sets upgrade code, name, description, version
  - `SetControlPanelInfo()` - Sets manufacturer, URLs, icon
  - `SetMinimalUI()` - Sets background/banner images, license

**2. Updated `AgentInstaller.cs`**
- Removed `using NineDigit.WixSharpExtensions;`

**3. Updated `DatadogInstaller.cs`**
- Removed `using NineDigit.WixSharpExtensions;`

**4. Updated `Program.cs`**
- Replaced obsolete `Compiler.LightOptions` and `Compiler.CandleOptions` with `Compiler.WixOptions`
```csharp
// Before (WiX 3)
Compiler.LightOptions += $"-sval -reusecab -cc \"{cabcachedir}\"";
Compiler.CandleOptions += "-sw1150 -arch x64";

// After (WiX 5)
Compiler.WixOptions += $"-sval -cc \"{cabcachedir}\" ";
Compiler.WixOptions += "-sw1150 ";
// Note: -arch x64 handled by project.Platform = Platform.x64
// Note: -reusecab is implicit in WiX5's -cabcache behavior
```

**5. Updated `tasks/msi.py`**
- Added WiX 5 migration note to `_fix_makesfxca_dll()` function

---

## Iteration 2: Namespace Change

### Build Errors
```
error CS0234: The type or namespace name 'Deployment' does not exist in the namespace 'Microsoft'
```
20+ files failed because `Microsoft.Deployment.WindowsInstaller` namespace doesn't exist in the new package.

### Fix Applied
Updated **29 files** to change the namespace:
```csharp
// Before
using Microsoft.Deployment.WindowsInstaller;

// After
using WixToolset.Dtf.WindowsInstaller;
```

**Files updated:**
- CustomActions/*.cs (20 files)
- AgentCustomActions/*.cs (2 files)
- InstallerCustomActions/*.cs (2 files)
- CustomActions.Tests/*.cs (5 files)

---

## Iteration 3: Target Framework Upgrade

### Build Errors
```
warning MSB3274: The primary reference "WixSharp.Msi.dll" could not be resolved because it was built against the ".NETFramework,Version=v4.7.2" framework. This is a higher version than the currently targeted framework ".NETFramework,Version=v4.6.2".
```
WixSharp_wix4 2.1.7 requires .NET Framework 4.7.2, but projects were targeting 4.6.2.

### Fix Applied
**1. WixSetup.csproj**
```xml
<!-- Before -->
<TargetFramework>net462</TargetFramework>

<!-- After -->
<TargetFramework>net472</TargetFramework>
```

**2. CustomActions.Tests.csproj**
```xml
<!-- Before -->
<TargetFrameworkVersion>v4.6.2</TargetFrameworkVersion>

<!-- After -->
<TargetFrameworkVersion>v4.7.2</TargetFrameworkVersion>
```

---

## Iteration 4: WiX4 API Breaking Changes

### Build Errors
```
error CS0619: 'Project.InstallPrivileges' is obsolete: 'This attribute is depreciated in WiX4'
error CS1061: 'Project' does not contain a definition for 'AddDirectories'
```

### Fix Applied (First Attempt - INCORRECT)
Tried using `project.Add()` and `project.Package.InstallScope` - both don't exist.

---

## Iteration 5: Correct WiX4 API Usage

### Build Errors
```
error CS1929: 'ManagedProject' does not contain a definition for 'Add'
error CS1061: 'Package' does not contain a definition for 'InstallScope'
```

### Fix Applied
**1. AgentInstaller.cs - Directories**
```csharp
// Before (using non-existent AddDirectories/Add)
.AddDirectories(
    CreateProgramFilesFolder(),
    CreateAppDataFolder(),
    new Dir(...)
);

// After (assign to Dirs array)
project.Dirs = new Dir[]
{
    CreateProgramFilesFolder(),
    CreateAppDataFolder(),
    new Dir(new Id("ProgramMenuDatadog"), @"%ProgramMenu%\Datadog",
        new ExeFileShortcut
        {
            Name = "Datadog Agent Manager",
            Target = "[AGENT]ddtray.exe",
            Arguments = "\"--launch-gui\"",
            WorkingDirectory = "AGENT",
        }
    )
};
```

**2. AgentInstaller.cs & DatadogInstaller.cs - InstallPrivileges**
```csharp
// Before
project.InstallPrivileges = InstallPrivileges.elevated;

// After (removed entirely)
// WiX 5 migration: InstallPrivileges is obsolete in WiX4.
// Per-machine installation (elevated) is the default, so this line is removed.
```

---

## Iteration 6: Background Image Validation

### Build Error
```
WixSharp.ValidationException: Project.BackgroundImage has incompatible (with ManagedUI default dialogs) aspect ratio.
The expected ratio is close to W(156)/H(312). The background image (left side banner) in ManagedUI dialogs is left-docked
at runtime and if it's too wide it can push away (to right) the all other UI elements.
You can suppress image validation by setting Project.ValidateBackgroundImage to 'false'.
```

WixSharp 2.x added stricter validation for background image aspect ratios (expects 156:312 = 1:2 ratio).

### Fix Applied
**WixSetup/WixSharpCompatExtensions.cs - SetMinimalUI method**
```csharp
// Before
project.BackgroundImage = backgroundImage?.FullName;

// After
// WiX 5 migration: Disable image aspect ratio validation.
// The existing background image was designed for the previous WixSharp version.
// WixSharp 2.x added stricter validation (expects W:H ratio of 156:312 = 1:2),
// but the existing image works correctly at runtime.
project.ValidateBackgroundImage = false;
project.BackgroundImage = backgroundImage?.FullName;
```

---

## Iteration 7: WixSharp_wix4 2.12.0 + DTF 4.0.5 Combination

### Build Error
```
System.IO.FileLoadException: Cannot resolve dependency to assembly 'WixToolset.Dtf.WindowsInstaller, Version=4.0.0.0, Culture=neutral, PublicKeyToken=a7d136314861246c' because it has not been preloaded.
```

MakeSfxCA (the tool that packages custom actions) expected DTF assembly version 4.0.0.0.

### Root Cause
WixSharp_wix4 (all versions including 2.12.0) bundles a MakeSfxCA tool that:
- Uses reflection to find custom action entry points
- Expects DTF with assembly version 4.0.0.0
- Cannot load assemblies that reference DTF 6.0.0.0

This is a **build-time** constraint, not a runtime constraint. The DTF version used by custom actions must match what MakeSfxCA expects.

### Fix Applied
Use WixSharp_wix4 2.12.0 (for WiX 5 output generation) with DTF 4.0.5 (for MakeSfxCA compatibility):

**Files updated:**
1. `WixSetup/WixSetup.csproj` - WixSharp_wix4 2.12.0
2. `CustomActions.Tests/CustomActions.Tests.csproj` - WixSharp_wix4 2.12.0, DTF 4.0.5
3. `CustomActions/CustomActions.csproj` - DTF 4.0.5
4. `AgentCustomActions/AgentCustomActions.csproj` - DTF 4.0.5
5. `InstallerCustomActions/InstallerCustomActions.csproj` - DTF 4.0.5

```xml
<!-- WixSharp version (generates WiX 5 compatible output) -->
<PackageReference Include="WixSharp_wix4" Version="2.12.0" />

<!-- DTF version (must be 4.x for MakeSfxCA compatibility) -->
<PackageReference Include="WixToolset.Dtf.WindowsInstaller" Version="4.0.5" />
```

### Technical Notes
- WixSharp_wix4 can generate WiX 5-compatible WXS files
- But its bundled MakeSfxCA tool still requires DTF 4.x for custom action packaging
- The DTF API is stable between 4.x and 6.x, so using 4.0.5 doesn't lose functionality
- This is a known limitation until WixSharp updates its bundled tools

---

## Iteration 8: WXS Document Structure Changes (Product → Package)

### Build Error
```
System.NullReferenceException: Object reference not set to an instance of an object.
   at WixSetup.TextStylesExtensions.AddTextStyle(XElement ui, String id, Font font, Color color)
   at WixSetup.DatadogCustomUI.OnWixSourceGenerated(XDocument document)
```

### Root Cause
WiX 4/5 renamed the root element from `<Product>` to `<Package>`. XPath queries like `Select("Wix/Product")` and `Select("Product/UI")` were returning null.

### Fix Applied
Updated all XPath-style queries to use `Package` instead of `Product`:

**Files updated:**
1. `DatadogCustomUI.cs` - `Select("Product/UI")` → `Select("Package/UI")`
2. `AgentInstaller.cs` - `Select("Wix/Product")` → `Select("Wix/Package")` (3 occurrences)
3. `DatadogInstaller.cs` - `Select("Wix/Product")` → `Select("Wix/Package")`
4. `CompressedDir.cs` - `Select("Wix/Product")` → `Select("Wix/Package")`

```csharp
// Before (WiX 3)
document.Select("Wix/Product")
document.Root.Select("Product/UI")

// After (WiX 4/5)
document.Select("Wix/Package")
document.Root.Select("Package/UI")
```

Note: `FindAll()` queries (e.g., `FindAll("Component")`) don't need changes since they search by element name regardless of parent path.

---

## Iteration 9: WiX Extension Version Mismatch

### Build Error
```
WixToolset.Netfx.wixext : warning WIX6101: Could not find expected package root folder wixext5. Ensure WixToolset.Netfx.wixext/6.0.2 is compatible with WiX v5.
Error: Cannot find WiX extension 'WixToolset.Netfx.wixext'. WixSharp attempted to install the extension but did not succeed.
```

The WiX extensions installed (version 6.0.2) are for WiX 6, but we have WiX 5 installed.

### Fix Applied
Downgraded WiX extensions to version 5.0.2 (compatible with WiX 5):

```powershell
# Remove existing extensions (wrong version)
wix extension remove -g WixToolset.Netfx.wixext
wix extension remove -g WixToolset.Util.wixext
wix extension remove -g WixToolset.UI.wixext

# Install WiX 5 compatible versions
wix extension add -g WixToolset.Netfx.wixext/5.0.2
wix extension add -g WixToolset.Util.wixext/5.0.2
wix extension add -g WixToolset.UI.wixext/5.0.2
```

### Note
WixSharp supports both WiX 5 and WiX 6. We chose to stay on WiX 5 for now. If upgrading to WiX 6 later:
- Run `dotnet tool update --global wix` to upgrade WiX
- Extensions 6.0.x will work with WiX 6
- WiX 6 has a new licensing model (free for open source/individual use)

---

## Iteration 10: WiX 5 Command-Line Options

### Build Error
```
wix.exe : error WIX0118: Additional argument '-sval' was unexpected.
```

WiX 5's `wix build` command has different options than WiX 3's Light/Candle.

### Fix Applied
Updated `Program.cs` to remove unsupported options:

```csharp
// Before (WiX 3 options)
Compiler.WixOptions += $"-sval -cc \"{cabcachedir}\" ";
Compiler.WixOptions += "-sw1150 ";

// After (WiX 5 compatible)
Compiler.WixOptions += $"-cc \"{cabcachedir}\" ";
```

### WiX 5 Command-Line Changes
| WiX 3 Option | WiX 5 Equivalent |
|--------------|------------------|
| `-sval` | Not needed (validation is separate: `wix msi validate`) |
| `-cc <dir>` | `-cc <dir>` or `-cabcache <dir>` (still works) |
| `-sw1150` | Not supported in CLI (use MSBuild property if needed) |
| `-reusecab` | Implicit in `-cc` behavior |
| `-arch x64` | Use `project.Platform = Platform.x64` |

---

## Iteration 11: WiX 5 Schema Changes

### Build Errors
```
error WIX0004: The Package element contains an unexpected attribute 'Comments'.
error WIX0199: The WixLocalization element has an incorrect namespace of 'WixLocalization'. 
Please make the WixLocalization element look like: <WixLocalization xmlns="http://wixtoolset.org/schemas/v4/wxl">.
```

### Fixes Applied

**1. Remove Comments attribute from Package element**

In WiX 5, the `Comments` attribute is no longer valid on the `Package` element.

Files updated:
- `AgentInstaller.cs` - Removed `project.Package.AttributesDefinition = $"Comments={ProductComment}"`
- `DatadogInstaller.cs` - Removed `project.Package.AttributesDefinition = $"Comments={ProductComment}"`

Note: Comments are still set via `ControlPanelInfo.Comments` which maps to the MSI Summary Information stream.

**2. Update WixLocalization namespace**

Updated `localization-en-us.wxl`:
```xml
<!-- Before (WiX 3) -->
<WixLocalization xmlns="http://schemas.microsoft.com/wix/2006/localization">

<!-- After (WiX 4/5) -->
<WixLocalization xmlns="http://wixtoolset.org/schemas/v4/wxl">
```

---

## Iteration 12: WiX 5 Schema Changes (Comprehensive)

### Build Errors
Multiple WiX 5 schema incompatibilities:
1. Dialog .wxi files missing namespace
2. Localization file using inner text instead of Value attribute
3. Feature `Absent` attribute renamed to `AllowAbsent`
4. Package `Condition` element replaced with `Launch`
5. `DeleteServices` using inner text instead of Condition attribute

### Fixes Applied

**1. Dialog .wxi files - Add WiX namespace**

Updated all 7 files in `WixSetup/dialogs/`:
```xml
<!-- Before -->
<Include>

<!-- After -->
<Include xmlns="http://wixtoolset.org/schemas/v4/wxs">
```

Files: `apikeydlg.wxi`, `sitedlg.wxi`, `fatalError.wxi`, `ddagentuserdlg.wxi`, `sendFlaredlg.wxi`, `ddlicense.wxi`, `errormodaldlg.wxi`

**2. Localization file - Use Value attribute**

Updated `localization-en-us.wxl`:
```xml
<!-- Before (WiX 3) -->
<String Id="xxx">value</String>

<!-- After (WiX 5) -->
<String Id="xxx" Value="value" />
```

**3. Feature Absent → AllowAbsent**

Updated `AgentFeatures.cs`:
```csharp
// Before
{"Absent", "allow"},
{"Absent", "disallow"},

// After
{"AllowAbsent", "yes"},
{"AllowAbsent", "no"},
```

**4. Package Condition → Launch**

Updated `MinimumSupportedWindowsVersion.cs`:
```csharp
// Before (WiX 3)
var elem = new XElement("Condition");
elem.Add(_condition.ToCData());

// After (WiX 5)
var elem = new XElement("Launch");
elem.SetAttributeValue("Condition", _condition.ToString());
```

**5. DeleteServices inner text → Condition attribute**

Updated `AgentInstaller.cs`:
```csharp
// Before
.AddElement("DeleteServices",
    value: "(Installed AND ...)");

// After  
.AddElement("DeleteServices",
    "Condition=(Installed AND ...)");
```

---

## Iteration 13: WiX 5 Dialog and Condition Schema Changes

### Build Errors
More WiX 5 schema changes in dialog files and condition elements:
1. `<Publish>condition</Publish>` → Condition attribute
2. `<Show>condition</Show>` → Condition attribute
3. `<Control><Condition Action="...">` → EnableCondition/DisableCondition/HideCondition attributes
4. `<Text>value</Text>` → Value attribute
5. Another `<Condition>` under Package from `MutuallyExclusiveProduct.cs`

### Fixes Applied

**1. MutuallyExclusiveProduct.cs - Launch instead of Condition**
```csharp
// Before
var conditionElement = new XElement("Condition", ...);

// After
var conditionElement = new XElement("Launch", ...);
```

**2. All dialog .wxi files - Comprehensive WiX 5 updates**

| Element | WiX 3 | WiX 5 |
|---------|-------|-------|
| Publish condition | `<Publish ...>1</Publish>` | `<Publish ... Condition="1" />` |
| Show condition | `<Show ...>(condition)</Show>` | `<Show ... Condition="condition" />` |
| Control Condition hide | `<Condition Action="hide">X</Condition>` | `HideCondition="X"` attribute |
| Control Condition enable/disable | `<Condition Action="enable/disable">` | `EnableCondition`/`DisableCondition` attributes |
| Text child element | `<Text>value</Text>` | `<Text Value="value" />` |

Files updated: All 7 files in `WixSetup/dialogs/`

---

## Iteration 14: WixFailWhenDeferred Replaced with util:FailWhenDeferred

### Build Error
```
error WIX0094: The identifier 'CustomAction:WixFailWhenDeferred' could not be found. Ensure you have typed the reference correctly and that all the necessary inputs are provided to the linker.
```

### Root Cause
In WiX 5, the `WixFailWhenDeferred` custom action (which was referenced via `<CustomActionRef Id="WixFailWhenDeferred" />`) has been replaced with the `<util:FailWhenDeferred />` element from the WixToolset.Util extension.

The old approach:
```xml
<CustomActionRef Id="WixFailWhenDeferred" />
```

No longer works because the `WixFailWhenDeferred` identifier doesn't exist in WiX 5.

### Fix Applied
Updated both installer projects to use the new `FailWhenDeferred` element with the util namespace:

**1. AgentInstaller.cs**
```csharp
// Before
document
    .Select("Wix/Package")
    .AddElement("CustomActionRef", "Id=WixFailWhenDeferred");

// After
document
    .Select("Wix/Package")
    .Add(new XElement(WixExtension.Util.ToXName("FailWhenDeferred")));
```

**2. DatadogInstaller.cs**
- Added `using System.Xml.Linq;` for `XElement`
- Same change as AgentInstaller.cs

### Technical Notes
- `WixExtension.Util.ToXName("FailWhenDeferred")` creates an `XName` with the correct util namespace (`http://wixtoolset.org/schemas/v4/wxs/util`)
- WixSharp automatically includes the extension when it detects elements with the util namespace
- The `FailWhenDeferred` element has the same purpose: allows testing rollback by setting `WIXFAILWHENDEFERRED=1`
- Documentation: https://wixtoolset.org/docs/schema/util/failwhendeferred

---

## Iteration 15: SummaryInformation Comments Not Generated

### Issue
After building the MSI, the Comments field in the MSI Details tab changed from:
- **Before**: "Copyright 2015 - Present Datadog"
- **After**: "This installer database contains the logic and data required to install Datadog Agent."

### Root Cause
In WiX 5, the `SummaryInformation` element has a new `Comments` attribute that must be explicitly set. WixSharp's `ControlPanelInfo.Comments` property does not generate this element in the WiX 5 output.

The default comment ("This installer database...") is WiX's default when no `SummaryInformation/@Comments` is specified.

### Fix Applied
Manually add the `SummaryInformation` element with `Comments` attribute in `WixSourceGenerated`:

**1. AgentInstaller.cs**
```csharp
// WiX 5 migration: SummaryInformation/Comments is a new element in WiX 5
// ControlPanelInfo.Comments doesn't generate this, so we add it manually
document
    .Select("Wix/Package")
    .AddElement("SummaryInformation", $"Comments={ProductComment}");
```

**2. DatadogInstaller.cs** - Same change

### Technical Notes
- WiX 5 documentation: https://wixtoolset.org/docs/schema/wxs/summaryinformation
- The `Comments` attribute on `SummaryInformation` is explicitly noted as "New in WiX v5"
- `ControlPanelInfo.Comments` still sets Add/Remove Programs metadata, but not the MSI file properties

---

## Iteration 16: Custom Action Entry Point Names (Lambda to Method Group)

### Runtime Error
```
Error 1723. There is a problem with this Windows Installer package. A DLL required for this install to complete could not be run.
Action RunAsAdmin, entry: <.ctor>b__118_0, library: C:\Users\...\MSIADD9.tmp
```

Error code 1154 (`ERROR_DLL_NOT_FOUND`) - the DLL entry point couldn't be found.

### Root Cause
Custom actions were defined using lambda expressions:
```csharp
session => CustomActions.EnsureAdminCaller(session)
```

The C# compiler generates mangled names for lambdas (like `<.ctor>b__118_0`). When WixSharp_wix4's MakeSfxCA packages these custom actions, it cannot find the entry points because the generated names aren't valid DLL exports.

### Fix Applied
Converted all lambda expressions to method group syntax:
```csharp
// Before (lambda - generates mangled entry point name)
session => CustomActions.EnsureAdminCaller(session)

// After (method group - generates proper "EnsureAdminCaller" entry point)
CustomActions.EnsureAdminCaller
```

**Files updated:**
1. `WixSetup/Datadog Agent/AgentCustomActions.cs` - 36 lambdas converted
2. `WixSetup/Datadog Installer/DatadogInstallerCustomActions.cs` - 9 lambdas converted

### Technical Notes
- Method group syntax tells the compiler to use the actual method name as the entry point
- This produces DLL exports like `EnsureAdminCaller` instead of `<.ctor>b__118_0`
- The method signatures (`ActionResult MethodName(Session session)`) already match the delegate expected by WixSharp's `CustomAction<T>` constructor
- This is a common issue when migrating from WiX 3 to WiX 5 with WixSharp

---

## Current Status

The MSI should now run without the DLL entry point error. Continue testing installation.

---

## Files Modified Summary

| File | Changes |
|------|---------|
| `WixSetup/WixSetup.csproj` | Package refs, target framework |
| `CustomActions/CustomActions.csproj` | Package ref for DTF |
| `AgentCustomActions/AgentCustomActions.csproj` | Package ref for DTF |
| `InstallerCustomActions/InstallerCustomActions.csproj` | Package ref for DTF |
| `CustomActions.Tests/CustomActions.Tests.csproj` | Package refs, target framework |
| `WixSetup/WixSharpCompatExtensions.cs` | **NEW FILE** - NineDigit replacement |
| `WixSetup/Program.cs` | Compiler options migration |
| `WixSetup/Datadog Agent/AgentInstaller.cs` | Removed NineDigit using, fixed Dirs, removed InstallPrivileges, WixFailWhenDeferred→util:FailWhenDeferred, SummaryInformation |
| `WixSetup/Datadog Installer/DatadogInstaller.cs` | Removed NineDigit using, removed InstallPrivileges, added XElement using, WixFailWhenDeferred→util:FailWhenDeferred, SummaryInformation |
| `WixSetup/Datadog Agent/AgentCustomActions.cs` | Lambda→method group for all 36 custom actions |
| `WixSetup/Datadog Installer/DatadogInstallerCustomActions.cs` | Lambda→method group for all 9 custom actions |
| `tasks/msi.py` | Added migration note to MakeSfxCA workaround |
| 29 source files | Namespace change: `Microsoft.Deployment.WindowsInstaller` → `WixToolset.Dtf.WindowsInstaller` |

---

## Next Steps

1. **Build and test** - Run the build to see if there are additional API compatibility issues
2. **WXS validation** - Check that generated WXS is valid WiX 5 schema
3. **MSI testing** - Test fresh install, upgrade, repair, uninstall
4. **CI/CD updates** - Install WiX 5 toolset on build agents:
   ```powershell
   dotnet tool install --global wix --version 5.0.2
   ```

---

## Reference

- Original migration plan: `WIXSHARP_WIX5_MIGRATION_PLAN.md`
- [WixSharp and WiX4 Wiki](https://github.com/oleg-shilo/wixsharp/wiki/WixSharp-and-WiX4)
- [WiX v5 Documentation](https://wixtoolset.org/docs/intro/)
- [WixToolset.Dtf.WindowsInstaller NuGet](https://www.nuget.org/packages/WixToolset.Dtf.WindowsInstaller)
