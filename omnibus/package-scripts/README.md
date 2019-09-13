The order in which these script are executed varies between APT and YUM which can
lead to some rather sneaky bugs. Here's the standard order for updates.

# APT
## source https://debian-handbook.info/browse/stable/sect.package-meta-information.html

 * `prerm` script of the old package (with arguments: `upgrade <new-version>`)
 * `preinst` script of the new package (with arguments: `upgrade <old-version>`)
 * New files get unpacked based on the file list embedded in the `.deb` package
 * `postrm` script from the old package (with arguments `upgrade <new-version>`)
 * `dpkg` updates the files list, removes the files that don't exist anymore, etc.
 * `postinst` of the new script is run (with arguments `configure <lst-configured-version>`

# YUM
## source: https://fedoraproject.org/wiki/Packaging:Scriptlets

 * `pretrans` of new package
 * `preinst` of new package
 * Files in the list get copied
 * `postinst` of new package
 * `prerm` of old package
 * Files in the old package file list that are not in the new one's get removed
 * `postrm` of the old package gets run
 * `posttrans` of the _new_ package is run

**Note:** if you remove files or other components in the `postrm` script, updates
won't work as expected with YUM.


# Getting repo up

/etc/yum.repos.d/myrepo.repo

```
[myrepo]
name=This is my repo
baseurl=https://repobase/master
enabled=1
gpgcheck=0
```

yum makecache --disablerepo=* --enablerepo=myrepo

# Checking packages in repo

yum --disablerepo="*" --enablerepo="myrepo" list available
