#ifndef _HOOKS_ALL_H_
#define _HOOKS_ALL_H_

#include "bpf.h"
#include "cgroup.h"
#include "chmod.h"
#include "chown.h"
#include "commit_creds.h"
#include "dentry_resolver.h"
#include "exec.h"
#include "filename.h"
#include "ioctl.h"
#include "iouring.h"
#include "link.h"
#include "mkdir.h"
#include "mmap.h"
#include "module.h"
#include "mount.h"
#include "mprotect.h"
#include "namespaces.h"
#include "open.h"
#include "procfs.h"
#include "ptrace.h"
#include "raw_syscalls.h"
#include "rename.h"
#include "rmdir.h"
#include "selinux.h"
#include "setattr.h"
#include "setxattr.h"
#include "signal.h"
#include "splice.h"
#include "umount.h"
#include "unlink.h"
#include "utimes.h"
#include "chdir.h"

#include "network/bind.h"

#ifndef DO_NOT_USE_TC
#include "network/dns.h"
#include "network/flow.h"
#include "network/net_device.h"
#include "network/router.h"
#include "network/tc.h"
#endif

#endif
