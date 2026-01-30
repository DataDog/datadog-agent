# WixSharp and WiX 5 Migration Plan

## Executive Summary

This document details the implementation plan for migrating the Datadog Agent Windows MSI installer from WiX Toolset 3.11 to WiX Toolset 5.0.2 and from WixSharp.bin 1.20.3 to WixSharp_wix4.

**Target Versions:**
| Component | Current | Target |
|-----------|---------|--------|
| WiX Toolset | 3.11 | 5.0.2 |
| WixSharp.bin | 1.20.3 | WixSharp_wix4 2.1.7 |
| Microsoft.Deployment.WindowsInstaller | 3.0.0.0 (assembly ref) | WixToolset.Dtf.WindowsInstaller 6.0.0 (NuGet) |
| NineDigit.WixSharpExtensions | 1.0.14 | Evaluate compatibility / implement alternatives |

---

## Investigation Findings

### 1. Project Structure Analysis

The installer solution contains 6 projects:

| Project | Framework | Key Dependencies |
|---------|-----------|------------------|
| WixSetup | .NET Framework 4.6.2 | WixSharp.bin 1.20.3, NineDigit.WixSharpExtensions 1.0.14 |
| CustomActions | .NET Framework 4.6.2 | Microsoft.Deployment.WindowsInstaller (assembly ref) |
| AgentCustomActions | .NET Framework 4.6.2 | Microsoft.Deployment.WindowsInstaller (assembly ref) |
| InstallerCustomActions | .NET Framework 4.6.2 | Microsoft.Deployment.WindowsInstaller (assembly ref) |
| CustomActions.Tests | .NET Framework 4.6.2 | WixSharp.bin 1.20.3, Microsoft.Deployment.WindowsInstaller (assembly ref) |
| WixSetup.Tests | .NET Framework 4.7.2 | (No WiX dependencies) |

### 2. Microsoft.Deployment.WindowsInstaller Usage

**29 files** use `Microsoft.Deployment.WindowsInstaller`. Key API surface used:

- `Session` class (property access, logging, messaging)
- `Record` class (for action records)
- `InstallMessage` enum
- `MessageResult` enum
- `ActionResult` enum
- `ComponentInfoCollection`
- `FeatureInfoCollection`
- `CustomActionData`

**Key Files:**
- `SessionWrapper.cs` - Wraps the Session class for testability
- `ISession.cs` - Interface exposing the Session API surface
- `ServiceCustomAction.cs` - Heavy usage of Record, InstallMessage
- All CustomAction files use ActionResult return types

### 3. NineDigit.WixSharpExtensions Usage

**2 files** use NineDigit.WixSharpExtensions:

1. `AgentInstaller.cs` - Uses:
   - `SetProjectInfo()` - Sets upgrade code, name, description, version
   - `SetControlPanelInfo()` - Sets manufacturer, URLs, icon
   - `SetMinimalUI()` - Sets background/banner images, license

2. `DatadogInstaller.cs` - Uses same methods

**Compatibility Assessment:**
- NineDigit.WixSharpExtensions 1.0.14 targets WixSharp 1.x
- No WixSharp_wix4 compatible version appears available
- **Action Required:** Must implement equivalent extension methods or find alternatives

### 4. WiX 3 vs WiX 5 Compiler Options - BREAKING CHANGE CONFIRMED

Current `Program.cs` usage:
```csharp
Compiler.LightOptions += $"-sval -reusecab -cc \"{cabcachedir}\"";
Compiler.CandleOptions += "-sw1150 -arch x64";
```

**WiX 5 Changes (from WixSharp_wix4 source code investigation):**

The `Compiler.LightOptions` and `Compiler.CandleOptions` properties are **obsolete with error** in WixSharp_wix4:
```csharp
[Obsolete("WiX4 (and higher) does not use Candle/Light any more but wix.exe. Thus use WixOptions instead", true)]
public static string LightOptions;
```

**New property:** `Compiler.WixOptions`
- Default value: `"-sw1026 -sw1076 -sw1079 -sw1149 "`
- Used for all WiX build options

**Required Migration:**
| WiX3 Property | WiX5 Property | Notes |
|---------------|---------------|-------|
| `Compiler.LightOptions` | `Compiler.WixOptions` | Will cause compile error if not changed |
| `Compiler.CandleOptions` | `Compiler.WixOptions` | Will cause compile error if not changed |
| `-arch x64` | `project.Platform = Platform.x64` | Already set in AgentInstaller.cs |

**WiX5 Option Equivalents (CONFIRMED):**
- `-sval` (skip validation) → Same option, add to `Compiler.WixOptions` ✅
- `-cc <dir>` (cabinet caching) → Same option (`-cabcache` or `-cc`), add to `Compiler.WixOptions` ✅
- `-reusecab` → Built into `-cc` behavior in WiX5 (automatic reuse) ✅

### 5. Driver Support

**Finding:** No `Driver` elements found in the codebase. Driver merge modules (DDNPM.msm, DDPROCMON.msm) are external and integrated via WiX `Merge` elements, which remain supported.

### 6. Dialog Files (WXI)

**7 WXI dialog files** found:
- `apikeydlg.wxi`
- `ddagentuserdlg.wxi`
- `ddlicense.wxi`
- `errormodaldlg.wxi`
- `fatalError.wxi`
- `sendFlaredlg.wxi`
- `sitedlg.wxi`

**Compatibility Assessment:**
- Dialog files use WiX 3 schema (`<Include>` root element)
- WiX 5 uses different namespace but structure is similar
- WixSharp should handle schema conversion automatically
- **Action Required:** Test that dialogs render correctly after migration

### 7. MakeSfxCA Workaround

The `_fix_makesfxca_dll()` function in `msi.py` works around a WiX 3.11 bug (GitHub issue #6089) where certificate table directory entries are corrupted.

**Assessment:**
- This bug is specific to WiX 3.11's sfxca.dll
- WiX 5 may have different sfxca.dll behavior
- **Action Required:** Test if workaround is still needed; remove if fixed

### 8. WiX Source Generation Events

The installer uses WixSharp's source generation events:
- `WixSourceGenerated` - Modifies WXS document (removes CreateFolder entries, adds merge modules)
- `WixSourceFormated` - Formatting hook
- `WixSourceSaved` - Saves copy of generated WXS

**Potential Issues:**
- XPath selectors use `"Wix/Product"` which may change in WiX 5 schema
- Need to verify `document.Select()` and `document.FindAll()` work with new namespaces

---

## Detailed Implementation Plan

### Phase 1: Environment Setup

#### 1.1 Install WiX 5 Toolset
```powershell
# Install WiX 5 as a .NET global tool
dotnet tool install --global wix --version 5.0.2

# Verify installation
wix --version
```

#### 1.2 CI/CD Environment Updates
- Update Windows build agents to include WiX 5 .NET tool
- Ensure .NET 6+ SDK is available (required for WiX 5)
- May need to keep WiX 3.11 available temporarily for rollback

### Phase 2: Project File Updates

#### 2.1 WixSetup.csproj Changes

**Current:**
```xml
<PackageReference Include="NineDigit.WixSharpExtensions" Version="1.0.14" />
<PackageReference Include="WixSharp.bin" Version="1.20.3" />
```

**Target:**
```xml
<PackageReference Include="WixSharp_wix4" Version="2.1.7" />
<!-- Remove NineDigit.WixSharpExtensions - implement alternatives -->
```

#### 2.2 CustomActions.csproj Changes

**Current:**
```xml
<Reference Include="Microsoft.Deployment.WindowsInstaller, Version=3.0.0.0, Culture=neutral, PublicKeyToken=ce35f76fcda82bad, processorArchitecture=MSIL" />
```

**Target:**
```xml
<!-- Remove assembly reference, add NuGet package -->
<PackageReference Include="WixToolset.Dtf.WindowsInstaller" Version="6.0.0" />
```

#### 2.3 AgentCustomActions.csproj Changes
Same pattern as CustomActions.csproj:
- Remove assembly reference to `Microsoft.Deployment.WindowsInstaller`
- Add NuGet package reference to `WixToolset.Dtf.WindowsInstaller 6.0.0`

#### 2.4 InstallerCustomActions.csproj Changes
Same pattern as CustomActions.csproj.

#### 2.5 CustomActions.Tests.csproj Changes

**Current:**
```xml
<Reference Include="Microsoft.Deployment.WindowsInstaller, Version=3.0.0.0, ..." />
<PackageReference Include="WixSharp.bin" Version="1.20.3" />
```

**Target:**
```xml
<PackageReference Include="WixToolset.Dtf.WindowsInstaller" Version="6.0.0" />
<PackageReference Include="WixSharp_wix4" Version="2.1.7" />
```

### Phase 3: Code Changes

#### 3.1 Namespace Updates (If Required)

The DTF library assembly name changes from `Microsoft.Deployment.WindowsInstaller` to `WixToolset.Dtf.WindowsInstaller`. However, the namespace should remain compatible.

**Files to verify (29 total):**
- CustomActions/*.cs
- AgentCustomActions/*.cs
- InstallerCustomActions/*.cs
- CustomActions.Tests/*.cs

**Expected Change:**
The API is largely compatible. Test compilation to identify any breaking changes.

#### 3.2 NineDigit.WixSharpExtensions Compatibility

**Investigation Results:**

NineDigit.WixSharpExtensions 1.0.14 has a NuGet dependency on `WixSharp (>= 1.20.1)` - NOT `WixSharp_wix4`. This creates a potential compatibility issue:

1. **NuGet Conflict Risk**: Installing both `WixSharp_wix4` and `NineDigit.WixSharpExtensions` may cause NuGet to pull in the old `WixSharp` package as a transitive dependency, leading to assembly conflicts.

2. **API Compatibility**: According to the WixSharp wiki, the APIs are compatible between WixSharp and WixSharp_wix4 (same type signatures), so the extension methods *should* work if the dependency issue is resolved.

**Recommended Approach - Try Option A First:**

**Option A: Test with both packages (Preferred if it works)**
```xml
<PackageReference Include="WixSharp_wix4" Version="2.1.7" />
<PackageReference Include="NineDigit.WixSharpExtensions" Version="1.0.14" />
```
- NuGet may resolve this cleanly since WixSharp_wix4 contains the same types
- May need binding redirects in App.config
- If there are compile errors or runtime conflicts, proceed to Option B or C

**Option B: Fork NineDigit.WixSharpExtensions source**
- Copy the relevant source files from [GitHub](https://github.com/ninedigit/wixsharpextensions) (MIT licensed)
- Update the `using` statements to use WixSharp_wix4 types
- Only copy the files for the methods actually used

**Option C: Implement equivalent extension methods locally (Fallback)**

Create a new file `WixSetup/WixSharpCompatExtensions.cs` to replace NineDigit.WixSharpExtensions functionality:

```csharp
using System;
using System.IO;
using WixSharp;

namespace WixSetup
{
    public static class WixSharpCompatExtensions
    {
        /// <summary>
        /// Sets basic project info (replaces NineDigit SetProjectInfo)
        /// </summary>
        public static Project SetProjectInfo(
            this Project project,
            Guid upgradeCode,
            string name,
            string description,
            Version version)
        {
            project.GUID = upgradeCode;
            project.Name = name;
            project.Description = description;
            project.Version = version;
            return project;
        }

        /// <summary>
        /// Sets control panel info (replaces NineDigit SetControlPanelInfo)
        /// </summary>
        public static Project SetControlPanelInfo(
            this Project project,
            string name,
            string manufacturer,
            string readme,
            string comment,
            string contact,
            Uri helpUrl,
            Uri aboutUrl,
            FileInfo productIconFilePath)
        {
            project.ControlPanelInfo.Name = name;
            project.ControlPanelInfo.Manufacturer = manufacturer;
            project.ControlPanelInfo.Readme = readme;
            project.ControlPanelInfo.Comments = comment;
            project.ControlPanelInfo.Contact = contact;
            project.ControlPanelInfo.HelpLink = helpUrl?.ToString();
            project.ControlPanelInfo.UrlInfoAbout = aboutUrl?.ToString();
            project.ControlPanelInfo.ProductIcon = productIconFilePath?.FullName;
            return project;
        }

        /// <summary>
        /// Sets minimal UI (replaces NineDigit SetMinimalUI)
        /// </summary>
        public static Project SetMinimalUI(
            this Project project,
            FileInfo backgroundImage,
            FileInfo bannerImage,
            FileInfo licenceRtfFile = null)
        {
            project.BackgroundImage = backgroundImage?.FullName;
            project.BannerImage = bannerImage?.FullName;
            if (licenceRtfFile != null)
            {
                project.LicenceFile = licenceRtfFile.FullName;
            }
            return project;
        }
    }
}
```

#### 3.3 Update AgentInstaller.cs and DatadogInstaller.cs

**Remove:**
```csharp
using NineDigit.WixSharpExtensions;
```

**Keep (if extension methods match):**
The `SetProjectInfo`, `SetControlPanelInfo`, and `SetMinimalUI` calls should work with the new extension methods if signatures match.

#### 3.4 Compiler Options Updates - CONFIRMED BREAKING CHANGE

**Investigation Results from WixSharp_wix4 source code:**

In WixSharp_wix4, the `Compiler.LightOptions` and `Compiler.CandleOptions` properties are **marked as obsolete with an error**:

```csharp
[Obsolete("WiX4 (and higher) does not use Candle/Light any more but wix.exe. Thus use WixOptions instead", true)]
public static string LightOptions;

[Obsolete("WiX4 (and higher) does not use Candle/Light any more but wix.exe. Thus use WixOptions instead", true)]
public static string CandleOptions;
```

The new property is `Compiler.WixOptions`:
```csharp
/// WiX compiler wix.exe options (e.g. "-define DEBUG"). Available only in WiX v4.*
/// The default value is "-sw1026 -sw1076 -sw1079 -sw1149 ";
public static string WixOptions = "-sw1026 -sw1076 -sw1079 -sw1149 ";
```

**Current Code (`Program.cs`):**
```csharp
Compiler.LightOptions += $"-sval -reusecab -cc \"{cabcachedir}\"";
Compiler.CandleOptions += "-sw1150 -arch x64";
```

**Required Changes:**

Update `Program.cs`:
```csharp
var cabcachedir = "cabcache";
if (!string.IsNullOrEmpty(Environment.GetEnvironmentVariable("AGENT_MSI_OUTDIR")))
{
    cabcachedir = Path.Combine(Environment.GetEnvironmentVariable("AGENT_MSI_OUTDIR"), cabcachedir);
}

// WiX5 uses unified 'wix build' command via Compiler.WixOptions
// Note: Compiler.LightOptions and Compiler.CandleOptions are OBSOLETE in WixSharp_wix4
//       Using them will cause compile errors!

// -sval: Skip MSI validation (same option works in WiX5)
// -cabcache/-cc: Cabinet caching (WiX5 equivalent of -reusecab -cc)
Compiler.WixOptions += $"-sval -cc \"{cabcachedir}\" ";

// -sw1150: Suppress ServiceConfig warning (same option works in WiX5)
Compiler.WixOptions += "-sw1150 ";

// -arch x64: Architecture - NOT needed in WixOptions
// This is handled by project.Platform = Platform.x64 (already set in AgentInstaller.cs line 297)

// AutoGeneration.InstallDirDefaultId remains unchanged
Compiler.AutoGeneration.InstallDirDefaultId = null;
```

**Complete Before/After Comparison:**

**Before (WiX3 / WixSharp.bin):**
```csharp
Compiler.LightOptions += $"-sval -reusecab -cc \"{cabcachedir}\"";
Compiler.CandleOptions += "-sw1150 -arch x64";
```

**After (WiX5 / WixSharp_wix4):**
```csharp
Compiler.WixOptions += $"-sval -cc \"{cabcachedir}\" -sw1150 ";
// Note: -arch x64 is handled by project.Platform = Platform.x64
// Note: -reusecab is implicit in -cc behavior in WiX5
```

**Option Migration Table:**

| WiX3 Option | Purpose | WiX5 Equivalent |
|-------------|---------|-----------------|
| `-sval` (Light) | Skip MSI validation | `-sval` in WixOptions ✅ |
| `-reusecab` (Light) | Reuse cabinet files | Built into `-cabcache` behavior ✅ |
| `-cc <dir>` (Light) | Cabinet cache directory | `-cabcache <dir>` or `-cc <dir>` ✅ |
| `-sw1150` (Candle) | Suppress warning 1150 | `-sw1150` in WixOptions ✅ |
| `-arch x64` (Candle) | Target architecture | `project.Platform = Platform.x64` ✅ |

**Cabinet Caching in WiX5:**
- WiX5 uses `-cabcache <path>` (or `-cc <path>`) for cabinet caching
- The `-reusecab` behavior is built into `-cabcache` - it automatically reuses cached cabinets when files haven't changed
- Cabinet cache validation checks: file count, names, order, and timestamps

**Build command generated by WixSharp_wix4:**
```
wix.exe build [WixOptions] [project.WixOptions] -o "output.msi" "input.wxs"
```

#### 3.5 WXS Document Manipulation Updates

Verify XPath selectors in `AgentInstaller.cs`:

**Current (WiX 3):**
```csharp
document.Select("Wix/Product")
document.FindAll("CreateFolder")
document.FindAll("Component")
```

**WiX 5 namespace:** `http://wixtoolset.org/schemas/v4/wxs`

WixSharp_wix4 should handle namespace differences internally. If not, update selectors to use namespace-aware queries.

### Phase 4: Build System Updates

#### 4.1 Update msi.py

**Review `_fix_makesfxca_dll()` function:**
1. Build test MSI with WiX 5
2. Check if certificate table issue persists
3. If fixed, remove or conditionally skip the workaround

**Verify build command compatibility:**
- WixSharp generates build commands that may reference different tools
- Ensure `Build_*.cmd` scripts work with WiX 5

### Phase 5: Testing

#### 5.1 Build Verification
- [ ] Solution builds without errors
- [ ] No new compiler warnings related to WiX/DTF
- [ ] WixSetup.exe runs successfully
- [ ] Generated WXS file is valid WiX 5 schema

#### 5.2 WXS Comparison
- [ ] Compare generated WXS between WiX 3 and WiX 5 builds
- [ ] Verify all components present
- [ ] Verify all features present
- [ ] Verify custom actions present and correctly sequenced
- [ ] Verify registry entries present
- [ ] Verify service definitions present

#### 5.3 MSI Validation
- [ ] MSI builds successfully
- [ ] MSI passes validation (ICE tests)
- [ ] MSI file size is comparable to previous builds
- [ ] MSI can be opened in Orca or similar tool for inspection

#### 5.4 Installation Testing
- [ ] **Fresh Install:** Clean Windows VM, install MSI
- [ ] **Upgrade:** Install previous version, then upgrade with new MSI
- [ ] **Downgrade Blocked:** Verify downgrade error message appears
- [ ] **Repair:** Trigger repair and verify success
- [ ] **Uninstall:** Complete uninstall and verify cleanup

#### 5.5 Feature Testing
- [ ] Core Agent installed and running
- [ ] Process Agent installed (if enabled)
- [ ] Trace Agent installed (if enabled)
- [ ] Security Agent installed (if enabled)
- [ ] System Probe installed (if enabled)
- [ ] Custom actions execute without errors in log
- [ ] User creation/configuration works
- [ ] Service permissions configured correctly
- [ ] Config file generation works
- [ ] Python distribution extraction works
- [ ] Telemetry reporting works

#### 5.6 FIPS Flavor Testing
- [ ] FIPS MSI builds successfully
- [ ] FIPS-specific features work correctly

#### 5.7 Datadog Installer MSI Testing
- [ ] Datadog Installer MSI builds successfully
- [ ] Fresh install works
- [ ] Upgrade works
- [ ] Uninstall works

### Phase 6: CI/CD Updates

#### 6.1 Build Agent Configuration
- Install WiX 5 .NET tool on Windows build agents
- Ensure PowerShell execution policy allows tool installation
- Update any cached tool versions

#### 6.2 Pipeline Updates
- Verify `.gitlab-ci.yml` works with new WiX tools
- Update any scripts that reference WiX tool paths directly
- Test pipeline end-to-end

---

## Risk Assessment

### High Risk Items
1. **NineDigit.WixSharpExtensions incompatibility** - Must implement replacements
2. **Compiler options changes** - May require significant adjustments
3. **WXS namespace changes** - Document manipulation code may break

### Medium Risk Items
4. **DTF API changes** - Minor namespace/API differences possible
5. **MakeSfxCA behavior changes** - Workaround may need adjustment
6. **Dialog rendering** - WiX 5 may render dialogs differently

### Low Risk Items
7. **Build time changes** - May be faster or slower
8. **MSI file size changes** - Compression differences possible

---

## Rollback Plan

If critical issues are encountered:

1. **Immediate:** Revert package reference changes in .csproj files
2. **Keep WiX 3.11 installed:** Can fall back to WiX 3 builds
3. **Feature flag:** Consider environment variable to select WiX version
4. **Document blockers:** Create issues for any fundamental incompatibilities

---

## Timeline Estimate

| Phase | Tasks | Dependencies |
|-------|-------|--------------|
| Phase 1 | Environment Setup | None |
| Phase 2 | Project File Updates | Phase 1 |
| Phase 3 | Code Changes | Phase 2 |
| Phase 4 | Build System Updates | Phase 3 |
| Phase 5 | Testing | Phase 4 |
| Phase 6 | CI/CD Updates | Phase 5 |

---

## Open Questions

1. **NineDigit.WixSharpExtensions:** Is there a WiX4-compatible version, or must we implement our own?
   - **Finding:** No WiX4-specific version exists. The package depends on `WixSharp (>= 1.20.1)`.
   - **Recommendation:** Try installing alongside WixSharp_wix4 first (APIs should be compatible). If conflicts occur, implement local replacements for the 3 methods used.

2. **Compiler Options:** What is the WixSharp_wix4 equivalent for `-sval`, `-reusecab`, `-cc`, `-sw1150`?
   - **Finding (CONFIRMED):** 
     - `Compiler.LightOptions` and `Compiler.CandleOptions` are **obsolete with error**
     - Use `Compiler.WixOptions` instead (new unified property)
     - `-sw1150` → Add to `Compiler.WixOptions`
     - `-arch x64` → Use `project.Platform = Platform.x64` (already set)
     - `-sval` → Try adding to `Compiler.WixOptions`
     - `-reusecab` / `-cc` → Needs further investigation for WiX5 equivalent

3. **Framework Upgrade:** Should we upgrade from .NET Framework 4.6.2 to .NET 6+?
   - **Consideration:** WixSharp_wix4 supports both, but .NET 6+ may be cleaner.

4. **WXS Schema:** Does WixSharp_wix4 handle namespace differences in `WixSourceGenerated` handlers?
   - **Action:** Test with a minimal project first. The WixSharp wiki says API remapping is handled automatically.

5. **Cabinet Caching in WiX5:** How to achieve equivalent performance to `-reusecab -cc <dir>`?
   - **Finding (CONFIRMED):** WiX5 uses `-cabcache <path>` (or `-cc <path>`) for cabinet caching. The `-reusecab` behavior is built into `-cabcache` - it automatically reuses cached cabinets when files haven't changed.
   - **Migration:** `Compiler.WixOptions += $"-cc \"{cabcachedir}\"";`

---

## Reference Links

- [WixSharp and WiX4 Wiki](https://github.com/oleg-shilo/wixsharp/wiki/WixSharp-and-WiX4)
- [WiX v5 Documentation](https://wixtoolset.org/docs/intro/)
- [WiX v4/v3 Migration Guide](https://wixtoolset.org/docs/fourthree/)
- [WixSharp_wix4 NuGet](https://www.nuget.org/packages/WixSharp_wix4)
- [WixToolset.Dtf.WindowsInstaller NuGet](https://www.nuget.org/packages/WixToolset.Dtf.WindowsInstaller)
- [WiX GitHub Issue #6089 (MakeSfxCA bug)](https://github.com/wixtoolset/issues/issues/6089)
