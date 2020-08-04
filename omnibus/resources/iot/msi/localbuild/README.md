## LocalInstall

The files here provide a means to more quickly test iterations of the Windows IoT Agent installer by creating an environment in which an installer can be recreated without an entire omnibus build.

### Requirements

Assumes that an omnibus build has been successfully completed; the script here will use the artifacts from that build.

Assumes that the omnibus output directory is `c:\omnibus-ruby`, and the datadog installation directory is `c:\opt\datadog-agent`.  If your environment is different, you can modify the parameters to `heat` and `light` in `rebuild.bat`, and the file `parameters.wxi` in this directory.

### rebuild.bat

The batch file mimics the behavior of the omnibus build script, after all of the pieces have been put into
place.  Executes `light` (which creates an XML representation of the directory structure) over `c:\opt\datadog-agent`
and the configuration files directory, `c:\omnibus-ruby\src\etc\datadog-agent\extra_package_files`

It includes `parameters.wxi`, where the variables such as the installation version can be changed (note that the included binaries will not be rebuilt, so the included binaries will not necessarily report the version specified in `parameters.wxi`.  However, the MSI itself will report itself as that version).

