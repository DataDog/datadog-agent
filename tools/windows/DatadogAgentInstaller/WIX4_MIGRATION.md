# WiX 4+ Migration Guide

This document describes the migration from WiX 3 (via `WixSharp.bin`) to WiX 4+ (via `WixSharp_wix4.bin`).

## Summary of Breaking Changes

The migration from WiX 3 to WiX 4+ involves several breaking API changes in WixSharp:

1. **Target Framework**: Must use .NET Framework 4.7.2+ (up from 4.6.2)
2. **Package Reference**: `WixSharp.bin` → `WixSharp_wix4.bin`
3. **Compiler Options**: `Compiler.LightOptions`/`CandleOptions` → `Compiler.WixOptions`
4. **Install Scope**: `InstallPrivileges.elevated` → `InstallScope.perMachine`
5. **CustomAction Delegates**: Method references must be wrapped in lambdas

## Detailed Changes

### 1. Package and Framework Update
**File**: `WixSetup/WixSetup.csproj`

Changed from:
```xml
<TargetFramework>net462</TargetFramework>
<PackageReference Include="WixSharp.bin" Version="1.20.3" />
```

To:
```xml
<TargetFramework>net472</TargetFramework>
<PackageReference Include="WixSharp_wix4.bin" Version="2.12.0" />
```

**Reason**: WixSharp_wix4.bin requires .NET Framework 4.7.2 or higher.

### 2. WiX Toolset Installation
**File**: `tasks/msi.py`

The build script now automatically installs WiX 5.0.2 globally if not already present. WixSharp_wix4 supports WiX 4.x and 5.x.

### 3. Compiler Options API Change
**File**: `Program.cs`

WiX 4+ uses unified `wix.exe` instead of separate `candle.exe`/`light.exe`:
- Combined `Compiler.LightOptions` and `Compiler.CandleOptions` into single `Compiler.WixOptions`

### 4. CustomAction Delegate Signatures
**Files**: `AgentCustomActions.cs`, `DatadogInstallerCustomActions.cs`

Fixed all ~45 custom action delegates to match WiX 4+ signature requirements:
- Wrapped all method references in lambda expressions
- Pattern: `CustomActions.Method` → `session => CustomActions.Method(session)`

### 5. Build Script Enhancement
**File**: `tasks/msi.py`

Added `_ensure_wix_tools()` function that:
- Checks if WiX is installed globally
- Automatically installs WiX 5.0.2 globally if not found

The function is automatically called at the beginning of the `_build()` function.

## How to Build

### Prerequisites
- .NET SDK 8.0 or higher (required for WiX 4+ dotnet tool)
- The WiX tools will be automatically installed during the build process

### Local Development
```bash
# The build process will automatically ensure WiX tools are installed
dda inv msi.build
```

### Manual Tool Installation (Optional)
If you prefer to install the tools manually before building:

```bash
# Install WiX 5.0.2 globally (compatible with WixSharp_wix4)
dotnet tool install --global wix --version 5.0.2
```

### Verify WiX Installation
```bash
# Check if WiX is installed
dotnet wix --version

# Should output: WiX Toolset v5.0.2.0 or similar
```

## CI/CD Considerations

The `_ensure_wix_tools()` function handles tool installation automatically, so no explicit CI/CD changes are required. The function will:
1. Check for global installation
2. Install WiX 5.0.2 globally if needed

The Windows build containers should have .NET SDK 8.0+ to support the dotnet tool installation.

## Key Differences from WiX 3

### Tooling
- **WiX 3**: Bundled binaries in NuGet package (`candle.exe`, `light.exe`)
- **WiX 5**: Distributed as .NET global tool (`wix` command, version 5.0.2)

### Build Process
- **WiX 3**: WixSharp directly invoked bundled executables
- **WiX 5**: WixSharp invokes the `wix` tool from PATH (globally installed)

### Compiler Options
- The `Compiler.LightOptions` and `Compiler.CandleOptions` in `Program.cs` remain unchanged
- WixSharp abstracts the differences between WiX 3 and WiX 4/5 command-line interfaces

### WiX Version
- Using **WiX 5.0.2** because WixSharp_wix4 supports both WiX 4.x and 5.x
- WiX 6 is too new and not officially supported by WixSharp yet

## Driver Support Note

The migration assumes that the driver merge modules (`DDNPM.msm` and `ddprocmon.msm`) will continue to work with WiX 4+. These MSMs are integrated via the `Merge` element in the generated WiX XML and should be compatible.

**Note**: WiX 4 dropped the built-in `<Driver>` element, but since we use pre-built merge modules rather than defining drivers directly in our WiX source, this should not affect our build.

## Rollback Plan

If issues arise, rollback is straightforward:

1. Revert `WixSetup.csproj`:
   ```xml
   <PackageReference Include="WixSharp.bin" Version="1.20.3" />
   ```

2. Revert changes to `tasks/msi.py` (remove `_ensure_wix_tools()` function and its call)

3. Uninstall WiX 5 if desired:
   ```bash
   dotnet tool uninstall --global wix
   ```

4. Clean and rebuild:
   ```bash
   dda inv clean
   dda inv msi.build
   ```

## API Changes Reference

### Compiler Options (WiX 3 → WiX 4+)
```csharp
// WiX 3
Compiler.LightOptions += "-sval -reusecab";
Compiler.CandleOptions += "-sw1150 -arch x64";

// WiX 4+
Compiler.WixOptions += "-sval -reusecab -sw1150 -arch x64";
```

### Install Privileges (WiX 3 → WiX 4+)
```csharp
// WiX 3
project.InstallPrivileges = InstallPrivileges.elevated;

// WiX 4+
project.InstallScope = InstallScope.perMachine;
```

### CustomAction Delegates (WiX 3 → WiX 4+)
```csharp
// WiX 3
new CustomAction<CustomActions>(
    new Id("MyAction"),
    CustomActions.MyMethod,
    Return.check
)

// WiX 4+
new CustomAction<CustomActions>(
    new Id("MyAction"),
    session => CustomActions.MyMethod(session),
    Return.check
)
```

## Testing Checklist

Before considering the migration complete, test:

- [ ] MSI builds successfully in local development
- [ ] MSI builds successfully in CI/CD
- [ ] Driver installation works correctly (NPM and procmon)
- [ ] Upgrade scenarios work (from previous version to new)
- [ ] Fresh installation works
- [ ] Uninstallation works
- [ ] Rollback during installation works
- [ ] All custom actions execute correctly
- [ ] Services start correctly after installation
- [ ] Agent configuration is preserved during upgrades
- [ ] All ~45 custom actions work correctly

## References

- [WixSharp WiX4 Documentation](https://github.com/oleg-shilo/wixsharp/wiki/WixSharp-and-WiX4)
- [WiX Toolset v5 Documentation](https://wixtoolset.org/docs/intro/)
- [WixSharp_wix4.bin NuGet Package](https://www.nuget.org/packages/WixSharp_wix4.bin/)

## FAQ

**Q: Why WiX 5.0.2 and not WiX 6?**  
A: WixSharp_wix4 is tested and qualified with WiX 4.x and 5.x. WiX 6 is very new and not yet officially supported by WixSharp. Using WiX 5.0.2 ensures stability and compatibility.

**Q: How do I check which WiX version is installed?**  
A: Run `dotnet wix --version` to see the installed version.

**Q: Can I use a different WiX 4.x or 5.x version?**  
A: Yes, any WiX 4.x or 5.x version should work. The build script installs 5.0.2 by default, but if you have a different compatible version already installed, it will use that.

