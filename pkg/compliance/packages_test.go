// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package compliance

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPackagesDpkgResolving(t *testing.T) {
	path := filepath.Join(t.TempDir(), "status")
	if err := os.WriteFile(path, []byte(dpkgStatus), 0600); err != nil {
		t.Fatal(err)
	}

	{
		pkg := findDpkgPackage(path, []string{"adduser"})
		if assert.NotNil(t, pkg) {
			assert.Equal(t, "adduser", pkg.Name)
			assert.Equal(t, "3.118ubuntu5", pkg.Version)
			assert.Equal(t, "all", pkg.Arch)
		}
	}

	{
		pkg := findDpkgPackage(path, []string{"apt-transport-https"})
		if assert.NotNil(t, pkg) {
			assert.Equal(t, "apt-transport-https", pkg.Name)
			assert.Equal(t, "2.4.9", pkg.Version)
			assert.Equal(t, "all", pkg.Arch)
		}
	}

	{
		pkg := findDpkgPackage(path, []string{"foo"})
		assert.Nil(t, pkg)
	}
}

func TestPackagesApkResolving(t *testing.T) {
	path := filepath.Join(t.TempDir(), "installed")
	if err := os.WriteFile(path, []byte(apkInstalled), 0600); err != nil {
		t.Fatal(err)
	}

	{
		pkg := findApkPackage(path, []string{"alpine-baselayout"})
		if assert.NotNil(t, pkg) {
			assert.Equal(t, "alpine-baselayout", pkg.Name)
			assert.NotNil(t, "3.4.3-r1", pkg.Version)
			assert.NotNil(t, "aarch64", pkg.Arch)
		}
	}

	{
		pkg := findApkPackage(path, []string{"zlib"})
		if assert.NotNil(t, pkg) {
			assert.Equal(t, "zlib", pkg.Name)
			assert.Equal(t, "1.2.13-r1", pkg.Version)
			assert.Equal(t, "aarch64", pkg.Arch)
		}
	}

	{
		pkg := findDpkgPackage(path, []string{"foo"})
		assert.Nil(t, pkg)
	}
}

const dpkgStatus = `Package: adduser
Status: install ok installed
Priority: important
Section: admin
Installed-Size: 608
Maintainer: Ubuntu Developers <ubuntu-devel-discuss@lists.ubuntu.com>
Architecture: all
Multi-Arch: foreign
Version: 3.118ubuntu5
Depends: passwd, debconf (>= 0.5) | debconf-2.0
Suggests: liblocale-gettext-perl, perl, ecryptfs-utils (>= 67-1)
Conffiles:
 /etc/deluser.conf 773fb95e98a27947de4a95abb3d3f2a2
Description: add and remove users and groups
 This package includes the 'adduser' and 'deluser' commands for creating
 and removing users.
 .
  - 'adduser' creates new users and groups and adds existing users to
    existing groups;
  - 'deluser' removes users and groups and removes users from a given
    group.
 .
 Adding users with 'adduser' is much easier than adding them manually.
 Adduser will choose appropriate UID and GID values, create a home
 directory, copy skeletal user configuration, and automate setting
 initial values for the user's password, real name and so on.
 .
 Deluser can back up and remove users' home directories
 and mail spool or all the files they own on the system.
 .
 A custom script can be executed after each of the commands.
Original-Maintainer: Debian Adduser Developers <adduser@packages.debian.org>

Package: apparmor
Status: install ok installed
Priority: optional
Section: admin
Installed-Size: 2628
Maintainer: Ubuntu Developers <ubuntu-devel-discuss@lists.ubuntu.com>
Architecture: arm64
Version: 3.0.4-2ubuntu2.2
Replaces: fcitx-data (<< 1:4.2.9.1-1ubuntu2)
Depends: debconf, lsb-base, debconf (>= 0.5) | debconf-2.0, libc6 (>= 2.34)
Suggests: apparmor-profiles-extra, apparmor-utils
Breaks: apparmor-profiles-extra (<< 1.21), fcitx-data (<< 1:4.2.9.1-1ubuntu2), snapd (<< 2.44.3+20.04~)
Conffiles:
 /etc/apparmor.d/abi/3.0 f97e410509c5def279aa227c7de12e06
 /etc/apparmor.d/abi/kernel-5.4-outoftree-network 57b68acd4e6418fe5a06dc8c04713e3d
 /etc/apparmor.d/abi/kernel-5.4-vanilla 77047e6f0b014fa8bf27681998382044
Description: user-space parser utility for AppArmor
 apparmor provides the system initialization scripts needed to use the
 AppArmor Mandatory Access Control system, including the AppArmor Parser
 which is required to convert AppArmor text profiles into machine-readable
 policies that are loaded into the kernel for use with the AppArmor Linux
 Security Module.
Homepage: https://apparmor.net/
Original-Maintainer: Debian AppArmor Team <pkg-apparmor-team@lists.alioth.debian.org>

Package: apt
Status: install ok installed
Priority: important
Section: admin
Installed-Size: 3956
Maintainer: Ubuntu Developers <ubuntu-devel-discuss@lists.ubuntu.com>
Architecture: arm64
Version: 2.4.9
Replaces: apt-transport-https (<< 1.5~alpha4~), apt-utils (<< 1.3~exp2~)
Provides: apt-transport-https (= 2.4.9)
Depends: adduser, gpgv | gpgv2 | gpgv1, libapt-pkg6.0 (>= 2.4.9), ubuntu-keyring, libc6 (>= 2.34), libgcc-s1 (>= 3.3.1), libgnutls30 (>= 3.7.0), libseccomp2 (>= 2.4.2), libstdc++6 (>= 11), libsystemd0
Recommends: ca-certificates
Suggests: apt-doc, aptitude | synaptic | wajig, dpkg-dev (>= 1.17.2), gnupg | gnupg2 | gnupg1, powermgmt-base
Breaks: apt-transport-https (<< 1.5~alpha4~), apt-utils (<< 1.3~exp2~), aptitude (<< 0.8.10)
Conffiles:
 /etc/apt/apt.conf.d/01-vendor-ubuntu c69ce53f5f0755e5ac4441702e820505
 /etc/apt/apt.conf.d/01autoremove ab6540f7278a05a4b7f9e58afcaa5f46
 /etc/cron.daily/apt-compat 1400ab07a4a2905b04c33e3e93d42b7b
 /etc/logrotate.d/apt 179f2ed4f85cbaca12fa3d69c2a4a1c3
Description: commandline package manager
 This package provides commandline tools for searching and
 managing as well as querying information about packages
 as a low-level access to all features of the libapt-pkg library.

Package: apt-transport-https
Status: install ok installed
Priority: optional
Section: oldlibs
Installed-Size: 165
Maintainer: Ubuntu Developers <ubuntu-devel-discuss@lists.ubuntu.com>
Architecture: all
Multi-Arch: foreign
Source: apt
Version: 2.4.9
Depends: apt (>= 1.5~alpha4)
Description: transitional package for https support
 This is a dummy transitional package - https support has been moved into
 the apt package in 1.5. It can be safely removed.
Original-Maintainer: APT Development Team <deity@lists.debian.org>
`

const apkInstalled = `C:Q1gwkUCQyUBQ5ixUJMU9ugflJvaV8=
P:alpine-baselayout
V:3.4.3-r1
A:aarch64
S:8911
I:331776
T:Alpine base dir structure and init scripts
U:https://git.alpinelinux.org/cgit/aports/tree/main/alpine-baselayout
L:GPL-2.0-only
o:alpine-baselayout
m:Natanael Copa <ncopa@alpinelinux.org>
t:1683642107
c:65502ca9379dd29d1ac4b0bf0dcf03a3dd1b324a
D:alpine-baselayout-data=3.4.3-r1 /bin/sh
F:dev
F:dev/pts
F:dev/shm
F:etc
R:motd
Z:Q1SLkS9hBidUbPwwrw+XR0Whv3ww8=
F:etc/apk
F:etc/conf.d
F:etc/crontabs
R:root
a:0:0:600
Z:Q1vfk1apUWI4yLJGhhNRd0kJixfvY=
F:etc/init.d
F:etc/modprobe.d
R:aliases.conf
Z:Q1WUbh6TBYNVK7e4Y+uUvLs/7viqk=
R:blacklist.conf
Z:Q14TdgFHkTdt3uQC+NBtrntOnm9n4=
R:i386.conf
Z:Q1pnay/njn6ol9cCssL7KiZZ8etlc=
R:kms.conf
Z:Q1ynbLn3GYDpvajba/ldp1niayeog=
F:etc/modules-load.d
F:etc/network
F:etc/network/if-down.d
F:etc/network/if-post-down.d
F:etc/network/if-pre-up.d
F:etc/network/if-up.d
F:etc/opt
F:etc/periodic
F:etc/periodic/15min
F:etc/periodic/daily
F:etc/periodic/hourly
F:etc/periodic/monthly
F:etc/periodic/weekly
F:etc/profile.d
R:20locale.sh
Z:Q1lq29lQzPmSCFKVmQ+bvmZ/DPTE4=
R:README
Z:Q135OWsCzzvnB2fmFx62kbqm1Ax1k=
R:color_prompt.sh.disabled
Z:Q11XM9mde1Z29tWMGaOkeovD/m4uU=
F:etc/sysctl.d
F:home
F:lib
F:lib/firmware
F:lib/mdev
F:lib/modules-load.d
F:lib/sysctl.d
R:00-alpine.conf
Z:Q1HpElzW1xEgmKfERtTy7oommnq6c=
F:media
F:media/cdrom
F:media/floppy
F:media/usb
F:mnt
F:opt
F:proc
F:root
M:0:0:700
F:run
F:sbin
F:srv
F:sys
F:tmp
M:0:0:1777
F:usr
F:usr/lib
F:usr/lib/modules-load.d
F:usr/local
F:usr/local/bin
F:usr/local/lib
F:usr/local/share
F:usr/sbin
F:usr/share
F:usr/share/man
F:usr/share/misc
F:var
R:run
a:0:0:777
Z:Q11/SNZz/8cK2dSKK+cJpVrZIuF4Q=
F:var/cache
F:var/cache/misc
F:var/empty
M:0:0:555
F:var/lib
F:var/lib/misc
F:var/local
F:var/lock
F:var/lock/subsys
F:var/log
F:var/mail
F:var/opt
F:var/spool
R:mail
a:0:0:777
Z:Q1dzbdazYZA2nTzSIG3YyNw7d4Juc=
F:var/spool/cron
R:crontabs
a:0:0:777
Z:Q1OFZt+ZMp7j0Gny0rqSKuWJyqYmA=
F:var/tmp
M:0:0:1777

C:Q1VFG2SuJdASGlXwlXF3975TbbpQU=
P:alpine-baselayout-data
V:3.4.3-r1
A:aarch64
S:11703
I:77824
T:Alpine base dir structure and init scripts
U:https://git.alpinelinux.org/cgit/aports/tree/main/alpine-baselayout
L:GPL-2.0-only
o:alpine-baselayout
m:Natanael Copa <ncopa@alpinelinux.org>
t:1683642107
c:65502ca9379dd29d1ac4b0bf0dcf03a3dd1b324a
r:alpine-baselayout
F:etc
R:fstab
Z:Q11Q7hNe8QpDS531guqCdrXBzoA/o=
R:group
Z:Q13K+olJg5ayzHSVNUkggZJXuB+9Y=
R:hostname
Z:Q16nVwYVXP/tChvUPdukVD2ifXOmc=
R:hosts
Z:Q1BD6zJKZTRWyqGnPi4tSfd3krsMU=
R:inittab
Z:Q1TsthbhW7QzWRe1E/NKwTOuD4pHc=
R:modules
Z:Q1toogjUipHGcMgECgPJX64SwUT1M=
R:mtab
a:0:0:777
Z:Q1kiljhXXH1LlQroHsEJIkPZg2eiw=
R:nsswitch.conf
Z:Q19DBsMnv0R2fajaTjoTv0C91NOqo=
R:passwd
Z:Q1TchuuLUfur0izvfZQZxgN/LJhB8=
R:profile
Z:Q1hEyrKWuyWL6NMZCkpcJ2zhRkVf4=
R:protocols
Z:Q11fllRTkIm5bxsZVoSNeDUn2m+0c=
R:services
Z:Q1oNeiKb8En3/hfoRFImI25AJFNdA=
R:shadow
a:0:42:640
Z:Q1ltrPIAW2zHeDiajsex2Bdmq3uqA=
R:shells
Z:Q1ojm2YdpCJ6B/apGDaZ/Sdb2xJkA=
R:sysctl.conf
Z:Q14upz3tfnNxZkIEsUhWn7Xoiw96g=

C:Q1ZMq6MGv+JSXQAOE76jyCe5dWyRc=
P:libssl3
V:3.1.2-r0
A:aarch64
S:238859
I:622592
T:SSL shared libraries
U:https://www.openssl.org/
L:Apache-2.0
o:openssl
m:Ariadne Conill <ariadne@dereferenced.org>
t:1691129235
c:b68a32f25ba44f406e02c2ca8f323a76f167d924
D:so:libc.musl-aarch64.so.1 so:libcrypto.so.3
p:so:libssl.so.3=3
r:openssl
F:lib
R:libssl.so.3
a:0:0:755
Z:Q1B528ud2lKcVyzet238jBUOSlOjc=
F:usr
F:usr/lib
R:libssl.so.3
a:0:0:777
Z:Q1oMqe3ENWHl40WtquaRE6llAmBfU=

C:Q1VciVehQwKlHzJI0nfN7QbxBneOU=
P:musl
V:1.2.4-r1
A:aarch64
S:398854
I:675840
T:the musl c library (libc) implementation
U:https://musl.libc.org/
L:MIT
o:musl
m:Timo Teräs <timo.teras@iki.fi>
t:1690477896
c:a6e14d1837131339f85ff337fbd4ecb8886945ae
p:so:libc.musl-aarch64.so.1=1
F:lib
R:ld-musl-aarch64.so.1
a:0:0:755
Z:Q1Vd4Lpar6qryZj5BP4riPzeSuc/c=
R:libc.musl-aarch64.so.1
a:0:0:777
Z:Q14RpiCEfZIqcg1XDcVqp8QEpc9ks=

C:Q1IKHbsxlRheYEiOxSpa6D7WErOC0=
P:musl-utils
V:1.2.4-r1
A:aarch64
S:38564
I:286720
T:the musl c library (libc) implementation
U:https://musl.libc.org/
L:MIT AND BSD-2-Clause AND GPL-2.0-or-later
o:musl
m:Timo Teräs <timo.teras@iki.fi>
t:1690477896
c:a6e14d1837131339f85ff337fbd4ecb8886945ae
D:scanelf so:libc.musl-aarch64.so.1
p:cmd:getconf=1.2.4-r1 cmd:getent=1.2.4-r1 cmd:iconv=1.2.4-r1 cmd:ldconfig=1.2.4-r1 cmd:ldd=1.2.4-r1
r:libiconv
F:sbin
R:ldconfig
a:0:0:755
Z:Q1Kja2+POZKxEkUOZqwSjC6kmaED4=
F:usr
F:usr/bin
R:getconf
a:0:0:755
Z:Q1izZWoldOAqeTW7ToYdYlsVXav0E=
R:getent
a:0:0:755
Z:Q1PWAxA6Bsnovo+RCeGj7PGHTXc/c=
R:iconv
a:0:0:755
Z:Q1hBbOQk69kSna5AjGE3ScLcMidgU=
R:ldd
a:0:0:755
Z:Q1r+KYty/HCLl4p4dvPt8kCb1mhB0=

C:Q1dVmESVpTp/lY86UlgkSbXcef/PI=
P:scanelf
V:1.3.7-r1
A:aarch64
S:37193
I:147456
T:Scan ELF binaries for stuff
U:https://wiki.gentoo.org/wiki/Hardened/PaX_Utilities
L:GPL-2.0-only
o:pax-utils
m:Natanael Copa <ncopa@alpinelinux.org>
t:1681228881
c:84a227baf001b6e0208e3352b294e4d7a40e93de
D:so:libc.musl-aarch64.so.1
p:cmd:scanelf=1.3.7-r1
r:pax-utils
F:usr
F:usr/bin
R:scanelf
a:0:0:755
Z:Q1CBhN1OLadjUn0UEJntZnCnN+gW0=

C:Q111WrBWbdlc225kJe0GVgH9uPKkE=
P:ssl_client
V:1.36.1-r2
A:aarch64
S:4955
I:81920
T:EXternal ssl_client for busybox wget
U:https://busybox.net/
L:GPL-2.0-only
o:busybox
m:Sören Tempel <soeren+alpine@soeren-tempel.net>
t:1690477944
c:2684a6593b10051f8f9fcb01e4734e2d9533b0ea
D:so:libc.musl-aarch64.so.1 so:libcrypto.so.3 so:libssl.so.3
p:cmd:ssl_client=1.36.1-r2
i:busybox=1.36.1-r2 libssl3
r:busybox-initscripts
F:usr
F:usr/bin
R:ssl_client
a:0:0:755
Z:Q1mFoJCS0Rnhi0QrFlX4mSf1hH3hA=

C:Q1W9bNRsvWY9M6bK1xczGEXAHIID4=
P:zlib
V:1.2.13-r1
A:aarch64
S:52659
I:143360
T:A compression/decompression Library
U:https://zlib.net/
L:Zlib
o:zlib
m:Natanael Copa <ncopa@alpinelinux.org>
t:1681228881
c:84a227baf001b6e0208e3352b294e4d7a40e93de
D:so:libc.musl-aarch64.so.1
p:so:libz.so.1=1.2.13
F:lib
R:libz.so.1
a:0:0:777
Z:Q16A/yKXYR0EF3avf+wJzXcNLZHgU=
R:libz.so.1.2.13
a:0:0:755
Z:Q1xUsJjsEavwD6+3qb6onGWpseXnQ=

`
