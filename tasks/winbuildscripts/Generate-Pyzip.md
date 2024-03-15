Generating Python Zips
======================

⚠️ This method is The Way to embed Python in the Agent. The Agent *no longer* uses the python binaries built from source from the fork https://github.com/DataDog/cpython ⚠️

The Windows builds require a prebuilt zip file which is extracted into the _embedded_ directory at build time. The zip files are constructed using the python installer from python.org.

How To Run The Script
---------------------

The script is run using the Agent build containers.  The full build container is required for the Python 3 implementation because Python 3 stopped including the necessary import libraries, so they must be generated during the zip creation process.  Furthermore, because it uses the build tools, the 32 bit zip must be created using the 32 bit container, and the 64 bit zip must be created using the 64 bit container

Script Arguments
----------------
The script takes 4 arguments, which are:
```
  * Version   The Python version to be created
  * md5sum    (optional) the MD5 sum of the installer package to be used
  * OutDir    The directory (relative to the container) into which to copy the resulting zip
  * x86       (switch)(optional) indicates that the 32 bit zip should be created.
```

Running the script
------------------

Putting it all together, you will have a command line such as
```powershell
docker run --rm -v "$(Get-Location):c:\mnt" <image> powershell -C "c:\mnt\tasks\winbuildscripts\generate-pyzip.ps1 -Version 3.8.1 -OutDir c:\mnt"
```

```
  * docker                          (run docker)
  * run                             (run the container)
  * --rm                            (remove the container when done)
  * -v                              (mount a volume in the container)
  * "$(Get-Location):c:\mnt"        (mount the current working directory to c:\mnt in the container)
  * <image>                         (fully qualified image name)
  * powershell                      (execute powershell in the container)
  * -C                              (indicates command to run in the container)
  * c:\mnt\generate-pyzip.ps1       (the path to the script to run.  Note this is the directory that was mounted earlier)
  * -Version 3.8.1                  (create a zip for python 3.8.1)
  * -OutDir c:\mnt                  (copy to c:\mnt.  This also must be the path mounted earlier)
```

  There is additional output, but the main screen output will be:

> Copying zip to c:\mnt

> ... Done.

> SHA256 Sum of resulting zip: 7fd3a854f87f9cb8c3f5eb784dc3488b1b536014442e7b31ebd5dbf4e48c73bc

And a file ` python-windows-3.8.1-amd64.zip` should appear in the current directory.
The SHA256 Sum can be copied into the omnibus definition.  The zip can be copied to the appropriate S3 bucket.

