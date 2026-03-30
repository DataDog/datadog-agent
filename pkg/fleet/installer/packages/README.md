# Package installation hooks

All the packaging tools we use (`dpkg`, `rpm`, the `installer`) allow package-specific code to execute at various stages of the package lifecycle.

## Regular installation / removal / upgrade

The following is valid for `deb`, `rpm`, and `oci` packages.

### Installation

![Install Hooks](https://gist.githubusercontent.com/arbll/13866f7e466706275274380b79a2bba4/raw/bfea4958a1e2fddfeac4649c55975d083d6693fd/install.svg)
[Source](https://docs.google.com/drawings/d/1wsqV_Id_utKt7VrT8DAAsP28edk4diBRae59TqnLOAk/edit)

1. v1's files are written to disk.
2. v1's `PostInstall` hook is executed.

### Removal

![Remove Hooks](https://gist.githubusercontent.com/arbll/13866f7e466706275274380b79a2bba4/raw/f6320aabc00a7a442da3dec11cab6fb83723b7bc/remove.svg)
[Source](https://docs.google.com/drawings/d/1FTWx4drnA_iQTCMUpxzVh-VWLTOgST552eRMkvrikGk/edit)

1. v1's `PreRemove` hook is executed.
2. v1's files are removed from disk.

### Upgrade

![Upgrade Hooks](https://gist.githubusercontent.com/arbll/13866f7e466706275274380b79a2bba4/raw/440e3cbc04d9762ee0f1864333ec1a004ec50159/upgrade.svg)
[Source](https://docs.google.com/drawings/d/17RHy35YWuriaeCXTQ5eQciC3goYgle2_Qwe2nhRzzho/edit)

1. v1's `PreRemove` hook is executed. Note that we inform the hook that the package is being upgraded.
2. v2's files are written to disk and v1's files are removed.
3. v2's `PostInstall` hook is executed. Note that we inform the hook that the package is being upgraded.

## Experiment upgrades

The following is only valid for `oci` packages.

The installer supports a safer upgrade path for `oci` packages called "experiments".

### Package upgrade

![Package Upgrade](https://gist.githubusercontent.com/arbll/13866f7e466706275274380b79a2bba4/raw/bfea4958a1e2fddfeac4649c55975d083d6693fd/experiment_package.svg)
[Source](https://docs.google.com/drawings/d/1j2k2vHQhBevPQxJLDAkC68RyMrKysBbJ1Z2ENrYNUxI/edit)

### Start experiment

1. v1's `PreStartExperiment` hook is executed.
2. v2's files are written to disk. v1's files are kept intact.
3. v2's `PostStartExperiment` hook is executed.

### Stop experiment

1. v2's `PreStopExperiment` hook is executed.
2. v2's files are removed from disk. v1's files are kept intact.
3. v1's `PostStopExperiment` hook is executed.

### Promote experiment

1. v2's `PrePromoteExperiment` hook is executed.
2. v1's files are removed from disk. v2's files are kept intact.
3. v1's `PostPromoteExperiment` hook is executed.

# Extensions installation hooks


## Regular installation / removal / upgrade

The following is valid for `deb`, `rpm`, and `oci` packages.

### Installation
When installing package's (v1) extension:

1. v1.extension's PreInstallExtension hook is executed.
2. v1.extension's files are written to disk.
3. v1.extesnion's PostInstallExtension hook is executed.

### Removal
When removing package's (v1) extension:

v1.extension's PreRemoveExtension hook is executed.
v1.extension's files are removed from disk.

### Upgrade

There is no concept of upgrade for an extension. When the package the extension is attached to gets upgraded, its pre-remove script should include removal & save of the installed extensions and its post-install script should include reinstallation of the saved extensions.
