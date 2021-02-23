# Setting up your development environment on Windows

## Git

You will need a working installation of `git`.  Either of these will do: 
 - [Git for Windows](https://git-for-windows.github.io/)
 - [GitHub Desktop](https://desktop.github.com/)

## GCC Toolchain

Agent 6 requires a GCC toolchain in addition to Go.  You can download the package from [here](http://win-builds.org/).  This will download a graphical package manager; there is no unattended install.  Select all of the available packages, and install.  Add the installation folder `bin` directory to the path.

## Python

On Windows, the [official installer](https://www.pywww.thon.org/downloads/) will
provide all the files needed.  You will need Python 2.7.

## Visual C for Python

You will need to download and install [Visual C for Python](https://www.microsoft.com/en-us/download/details.aspx?id=44266)

## Python Package Config file

You may need to edit the Python package config file to point to your Python installation.  Edit the file python-27.pc in `pkg-config\windows\system`. Set the [prefix](https://github.com/DataDog/datadog-agent/blob/master/pkg-config/windows/system/python-2.7.pc#L1) entry to the root of your Python installation.

## Path

After the installations, you will need to add the following to your path:
 - The Python directory
 - `<pythondir>\scripts`
 - `%GOPATH%\bin`
 - `<winbuildsroot>\bin`
