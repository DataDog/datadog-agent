The order in which these script are executed varies between APT and YUM which can lead to some
rather sneaky bugs. Here's the standard order for updates:

APT (source https://debian-handbook.info/browse/stable/sect.package-meta-information.html):
-------------------------------------------------------------------------------------------
* `prerm` script of the old package (with arguments: `upgrade <new-version>`)
* `preinst` script of the new package (with arguments: `upgrade <old-version>`)
* New files get unpacked based on the file list embedded in the `.deb` package
* `postrm` script from the old package (with arguments `upgrade <new-version>`)
* `dpkg` updates the files list, removes the files that don't exist anymore, etc.
* `postinst` of the new script is run (with arguments `configure <lst-configured-version>`

YUM (source: various Stackoverflow posts + local experiments):
--------------------------------------------------------------

* `pretrans` of new package
* `preinst` of new package`
* Files in the list get copied
* `prerm` of old package
* Files in the old package file list that are not in the new one's get removed
* `postrm` of the old package gets run
* `posttrans` of the old package is ran

One thing to notice is that if you remove files or other components in the `postrm` script,
updates won't work as expected with YUM.
