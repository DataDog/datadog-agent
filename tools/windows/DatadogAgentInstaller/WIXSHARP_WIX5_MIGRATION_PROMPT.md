# WixSharp and WiX 5 Migration Prompt

## Objective

Upgrade the Datadog Agent Windows MSI installer from:
- **WiX Toolset 3.11** → **WiX Toolset 5.0.2**
- **WixSharp 1.20.3** → **WixSharp_wix4** (latest version supporting WiX 5)

## Background

Per the [WixSharp and WiX4 wiki](https://github.com/oleg-shilo/wixsharp/wiki/WixSharp-and-WiX4):
> "WiX Toolset version 3 is no longer actively maintained by the WiX team. The latest release, WiX v3.14.1, was provided to address specific vulnerabilities, with a strong recommendation to upgrade to WiX v4.0 or later. For up-to-date features, security updates, and support, it's advisable to migrate to WiX v4.0 or the latest version, WiX v5.0.2, released on October 4, 2024."

WixSharp has migrated to support WiX4/WiX5 with the `WixSharp_wix4` NuGet package, which automatically targets WiX tools present in the build environment.

---

## Current State

### Version Summary
| Component | Current Version |
|-----------|-----------------|
| WiX Toolset | 3.11 |
| WixSharp.bin | 1.20.3 |
| NineDigit.WixSharpExtensions | 1.0.14 |
| Microsoft.Deployment.WindowsInstaller | 3.0.0.0 |
| Target Framework | .NET Framework 4.6.2 |

### Project Structure
```
tools/windows/DatadogAgentInstaller/
├── DatadogAgentInstaller.sln
├── WixSetup/                           # Main MSI builder (WixSetup.exe)
│   ├── WixSetup.csproj                 # References WixSharp.bin 1.20.3
│   ├── Program.cs                      # Entry point, calls Compiler.* APIs
│   ├── Datadog Agent/
│   │   ├── AgentInstaller.cs           # Main agent MSI configuration
│   │   ├── AgentCustomActions.cs
│   │   ├── AgentBinaries.cs
│   │   ├── AgentFeatures.cs
│   │   ├── AgentInstallerUI.cs
│   │   └── AgentVersion.cs
│   ├── Datadog Installer/
│   │   ├── DatadogInstaller.cs
│   │   ├── DatadogInstallerCustomActions.cs
│   │   └── DatadogInstallerUI.cs
│   ├── CompressedDir.cs                # Custom WixSharp.File subclass
│   ├── CustomAction.cs
│   ├── Conditions.cs
│   ├── DatadogCustomUI.cs
│   ├── Dialogs.cs
│   ├── dialogs/*.wxi                   # Custom dialog definitions
│   └── assets/                         # Banner, icons, license
├── CustomActions/                       # Shared custom action logic
│   ├── CustomActions.csproj            # Uses Microsoft.Deployment.WindowsInstaller 3.0
│   └── *.cs                            # Various custom action implementations
├── AgentCustomActions/                  # Agent-specific custom actions
│   └── AgentCustomActions.csproj       # Uses Microsoft.Deployment.WindowsInstaller 3.0
├── InstallerCustomActions/              # Installer-specific custom actions
│   └── InstallerCustomActions.csproj   # Uses Microsoft.Deployment.WindowsInstaller 3.0
├── CustomActions.Tests/
│   └── CustomActions.Tests.csproj      # Also references WixSharp.bin 1.20.3
└── WixSetup.Tests/
    └── WixSetup.Tests.csproj
```

### Current NuGet Package References

**WixSetup.csproj:**
```xml
<PackageReference Include="NineDigit.WixSharpExtensions" Version="1.0.14" />
<PackageReference Include="WixSharp.bin" Version="1.20.3" />
```

**CustomActions.Tests.csproj:**
```xml
<PackageReference Include="WixSharp.bin" Version="1.20.3" />
```

### Microsoft.Deployment.WindowsInstaller References (WiX 3 DTF)

These 4 projects reference `Microsoft.Deployment.WindowsInstaller` Version 3.0.0.0:
- `CustomActions/CustomActions.csproj`
- `AgentCustomActions/AgentCustomActions.csproj`
- `InstallerCustomActions/InstallerCustomActions.csproj`
- `CustomActions.Tests/CustomActions.Tests.csproj`

### Build System
The build is orchestrated by `tasks/msi.py` using the `dda inv msi.build` command:
1. Copies source to `C:\dev\msi\DatadogAgentInstaller`
2. Runs `msbuild` to build `WixSetup.exe`
3. Runs `WixSetup.exe` to generate WXS and create build command
4. Uses `Compiler.CandleOptions` and `Compiler.LightOptions` (WiX 3 tools)
5. Builds final MSI

### Current WXS Output Schema
The generated WXS uses WiX 3 namespace:
```xml
<Wix xmlns="http://schemas.microsoft.com/wix/2006/wi" 
     xmlns:netfx="http://schemas.microsoft.com/wix/NetFxExtension">
```

Uses WiX extensions:
- `http://schemas.microsoft.com/wix/UtilExtension` (RemoveFolderEx, EventSource, ServiceConfig)
- `http://schemas.microsoft.com/wix/NetFxExtension` (.NET Framework detection)

---

## Target State

### Version Summary
| Component | Target Version |
|-----------|----------------|
| WiX Toolset | 5.0.2 |
| WixSharp_wix4 | Latest (check NuGet) |
| NineDigit.WixSharpExtensions | Check compatibility or remove |
| WixToolset.Dtf.WindowsInstaller | Latest (replaces Microsoft.Deployment.WindowsInstaller) |

### Key Package Changes
| Old Package | New Package |
|-------------|-------------|
| `WixSharp.bin` | `WixSharp_wix4` |
| `Microsoft.Deployment.WindowsInstaller` (assembly ref) | `WixToolset.Dtf.WindowsInstaller` (NuGet) |
| WiX 3 tools (candle.exe, light.exe) | WiX 5 tools (.NET tools via `wix` CLI) |

---

## Known Migration Considerations

### 1. WiX Schema Changes
WiX 4/5 uses a new XML namespace:
- Old: `http://schemas.microsoft.com/wix/2006/wi`
- New: `http://wixtoolset.org/schemas/v4/wxs`

WixSharp should handle this automatically, but verify the generated WXS.

### 2. DTF (Deployment Tools Foundation) Changes
The custom actions use `Microsoft.Deployment.WindowsInstaller` from WiX 3 DTF. In WiX 4/5:
- Replace with `WixToolset.Dtf.WindowsInstaller` NuGet package
- API is largely compatible but some namespace changes may be required
- Assembly name changes from `Microsoft.Deployment.WindowsInstaller` to `WixToolset.Dtf.WindowsInstaller`

### 3. Extension Changes
WiX extensions have been reorganized:
- Util extension: `WixToolset.Util.wixext`
- NetFx extension: May have different detection mechanisms

### 4. Compiler Options
Current `Program.cs` uses:
```csharp
Compiler.LightOptions += "-sval -reusecab -cc \"...\"";
Compiler.CandleOptions += "-sw1150 -arch x64";
```
WiX 5 uses a unified `wix build` command - WixSharp_wix4 should abstract this.

### 5. NineDigit.WixSharpExtensions
Check if this extension is compatible with WixSharp_wix4. If not, may need to:
- Find an updated version
- Implement equivalent functionality directly
- Remove if no longer needed

### 6. Driver Support
Per WixSharp wiki: "Starting from WiX4 support for `Driver` has been dropped."
- Verify the Datadog Agent installer doesn't use Driver elements
- The DDNPM and DDPROCMON driver merge modules (.msm) are external and should still work

### 7. MakeSfxCA Changes
The `_fix_makesfxca_dll` function in `msi.py` works around a WiX 3.11 issue. This may be fixed in WiX 5 - verify if the workaround is still needed.

---

## Files Requiring Modification

### Project Files (NuGet Package Updates)
1. `WixSetup/WixSetup.csproj`
   - Change `WixSharp.bin` → `WixSharp_wix4`
   - Verify `NineDigit.WixSharpExtensions` compatibility

2. `CustomActions/CustomActions.csproj`
   - Replace `Microsoft.Deployment.WindowsInstaller` assembly reference with NuGet package

3. `AgentCustomActions/AgentCustomActions.csproj`
   - Replace `Microsoft.Deployment.WindowsInstaller` assembly reference with NuGet package

4. `InstallerCustomActions/InstallerCustomActions.csproj`
   - Replace `Microsoft.Deployment.WindowsInstaller` assembly reference with NuGet package

5. `CustomActions.Tests/CustomActions.Tests.csproj`
   - Update both `WixSharp.bin` and `Microsoft.Deployment.WindowsInstaller`

### Source Files (Potential API Changes)
6. `WixSetup/Program.cs`
   - Review `Compiler.CandleOptions` and `Compiler.LightOptions` usage
   - May need to use WiX 5 equivalent options

7. All files using `Microsoft.Deployment.WindowsInstaller`:
   - May need namespace updates if assembly name changes
   - Files: `SessionWrapper.cs`, `ServiceCustomAction.cs`, `ConfigCustomActions.cs`, etc.

### Build Scripts
8. `tasks/msi.py`
   - Review `_fix_makesfxca_dll` function (may no longer be needed)
   - Verify WiX tool invocation still works

### Dialog Files
9. `WixSetup/dialogs/*.wxi`
   - Verify WiX 5 compatibility of custom dialog definitions

---

## Migration Steps

### Phase 1: Environment Setup
1. Install WiX 5.0.2 as a .NET tool: `dotnet tool install --global wix --version 5.0.2`
2. Verify WiX 5 is available: `wix --version`

### Phase 2: Package Updates
1. Update `WixSetup.csproj`:
   ```xml
   <PackageReference Include="WixSharp_wix4" Version="X.Y.Z" />
   ```
2. Add DTF NuGet package to CustomActions projects:
   ```xml
   <PackageReference Include="WixToolset.Dtf.WindowsInstaller" Version="X.Y.Z" />
   ```
3. Remove old assembly references to `Microsoft.Deployment.WindowsInstaller`

### Phase 3: Code Updates
1. Update any namespace references if needed
2. Review and update Compiler options in `Program.cs`
3. Test build and fix any compile errors
4. Review WixSharp API changes per migration guide

### Phase 4: Build & Validation
1. Run `dda inv msi.build --debug` to build locally
2. Compare generated WXS between old and new versions
3. Verify all components, features, and custom actions are present
4. Test installation on Windows VM
5. Test upgrade scenarios
6. Test uninstallation

### Phase 5: CI/CD Updates
1. Update CI build agents to have WiX 5 tools installed
2. Update any CI scripts that reference WiX tools directly
3. Run full E2E test suite

---

## Testing Checklist

- [ ] MSI builds successfully with no errors
- [ ] Generated WXS contains all expected components
- [ ] Fresh install works correctly
- [ ] Upgrade from previous version works
- [ ] Downgrade is blocked appropriately  
- [ ] Uninstall removes all files and registry entries
- [ ] Custom actions execute correctly:
  - [ ] User creation/configuration
  - [ ] Service installation
  - [ ] Config file generation
  - [ ] Telemetry
- [ ] All features install correctly:
  - [ ] Core Agent
  - [ ] Process Agent
  - [ ] Trace Agent
  - [ ] Security Agent
  - [ ] System Probe
- [ ] FIPS flavor builds correctly
- [ ] Datadog Installer MSI builds correctly
- [ ] Driver merge modules integrate correctly

---

## Rollback Plan

If issues are encountered:
1. Revert package reference changes
2. Keep WiX 3.11 installed alongside WiX 5 for fallback
3. Document any blockers for future resolution

---

## Reference Links

- [WixSharp and WiX4 Wiki](https://github.com/oleg-shilo/wixsharp/wiki/WixSharp-and-WiX4)
- [WiX v5 Documentation](https://wixtoolset.org/docs/intro/)
- [WiX v4 Migration Guide](https://wixtoolset.org/docs/fourthree/)
- [WixSharp_wix4 NuGet](https://www.nuget.org/packages/WixSharp_wix4)
- [WixToolset.Dtf.WindowsInstaller NuGet](https://www.nuget.org/packages/WixToolset.Dtf.WindowsInstaller)
