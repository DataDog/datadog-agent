
### Third party notice

Two third party files are used and adapted in this PR:
- `devenv\scripts\Install-GuestTools.ps1` from https://github.com/luciusbono/Packer-Windows10/blob/master/install-guest-tools.ps1
- `devenv\scripts\Enable-WinRM.ps1` from https://github.com/StefanScherer/packer-windows/blob/my/scripts/enable-winrm.ps1

### Usage of the Powershell script

Copy the script `devenv\scripts\Install-DevEnv.ps1` on the target machine and then in an Administrator Powershell prompt:

`Set-ExecutionPolicy Bypass -Scope Process -Force; <path_to_ps1>\Install-DevEnv.ps1`

### Generating the base boxes

Since the Packer's json file contained a lot of duplicated content, I've made a Ruby template file.
To generate the Packer file and then invoke Packer, I've used the `Invoke!` library.

To generate the `packer.json` file (here for Windows 10):
`inv packer.build --os=windows-10 --provider=virtualbox-iso`

Where the valid `os` values are:

- `windows-10`
- `windows-server`

And the valid `provider` values are:

- `virtualbox-iso`
- `vmware-iso`
- `parallels-iso`

The default values are `windows-10` and `virtualbox-iso`.

Then, it's just a matter of building the images:
`packer build packer.json`

**Note:** For Parallels, you'll also need to install the Virtualization SDK:
`brew cask install parallels-virtualization-sdk`

**Note:** By default this will launch the VM for the selected provider (Virtualbox, VMWare, Parallels) and the VM will consume 2GB of RAM. The provider must be installed on the machine. Parallel building is not supported because it is a massive strain on the building machine and frequently resulted in crashes.

**Note:** The base boxes are based on Windows 10 Enterprise Evaluation (1903) and Windows Server 2019 Evaluation ISOs. They are good for 90 days, after that a valid license must be provided.

### Vagrant usage

The provided `Vagrantfile` expects the box to exist in the same directory.

To start a VM:
`vagrant up win-10-prl --provider=parallels`

Then, to run commands inside the VM:
`vagrant winrm -c "ipconfig" win-10-prl`

The VMs are customized to:
- Allocate 4GB of RAM instead of 2GB default.
- Enable `linked clones` so that multiple versions of the same VMs can share much of the storage.
- Enable `nested virtualization` to allow running docker containers in the VM. This **does not work** for Virtualbox.

**Note:** By default the `Vagrantfile` will attempt to mount your  "$GOPATH/src/github.com/DataDog" folder in the VM in "/Users/dogdev/go/src/github.com/DataDog"

### VMWare Vagrant Issue

[The Vagrant VMWare integration is a paid module](https://www.vagrantup.com/vmware/index.html) (separate from the VMWare license).

There are a few alternatives to this:
- Use [the FOSS equivalent](https://github.com/orenmazor/vagrant-vmware-provider)
- Use [mech](https://github.com/mechboxes/mech), which replaces Vagrant, in a way
- Use tar to extract the VMWare files and use them directly:
```
mkdir vm
cd vm
tar zxvf ../windows_10_ent_vmware.box
```