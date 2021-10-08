/* Test for access to file, relative to open directory.  Linux version.
   Copyright (C) 2006-2021 Free Software Foundation, Inc.
   This file is part of the GNU C Library.

   The GNU C Library is free software; you can redistribute it and/or
   modify it under the terms of the GNU Lesser General Public
   License as published by the Free Software Foundation; either
   version 2.1 of the License, or (at your option) any later version.

   The GNU C Library is distributed in the hope that it will be useful,
   but WITHOUT ANY WARRANTY; without even the implied warranty of
   MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the GNU
   Lesser General Public License for more details.

   You should have received a copy of the GNU Lesser General Public
   License along with the GNU C Library; if not, see
   <https://www.gnu.org/licenses/>.  */

#include <fcntl.h>
#include <unistd.h>
#include <errno.h>
#include <alloca.h>
#include <sys/types.h>
#include <sys/stat.h>
#include <syscall.h>

#ifndef NGROUPS_MAX
#define NGROUPS_MAX     16      /* First guess.  */
#endif

int
group_member (gid_t gid)
{
  int n, size;
  gid_t *groups;

  size = NGROUPS_MAX;
  do
    {
      groups = alloca (size * sizeof *groups);
      n = getgroups (size, groups);
      size *= 2;
    }
  while (n == size / 2);

  while (n-- > 0)
    if (groups[n] == gid)
      return 1;

  return 0;
}

int
faccessat (int fd, const char *file, int mode, int flag)
{
  if (flag & ~(AT_SYMLINK_NOFOLLOW | AT_EACCESS)) {
    errno = EINVAL;
    return -1l;
  }

  int __libc_enable_secure = geteuid() != getuid() || getegid() != getgid();

  if ((flag == 0 || ((flag & ~AT_EACCESS) == 0 && ! __libc_enable_secure)))
    return syscall(SYS_faccessat, fd, file, mode);

  struct stat stats;
  if (fstatat(fd, file, &stats, flag & AT_SYMLINK_NOFOLLOW))
    return -1;

  mode &= (X_OK | W_OK | R_OK);	/* Clear any bogus bits. */
# if R_OK != S_IROTH || W_OK != S_IWOTH || X_OK != S_IXOTH
#  error Oops, portability assumptions incorrect.
# endif

  if (mode == F_OK)
    return 0;			/* The file exists. */

  uid_t uid = (flag & AT_EACCESS) ? geteuid () : getuid ();

  /* The super-user can read and write any file, and execute any file
     that anyone can execute. */
  if (uid == 0 && ((mode & X_OK) == 0
		   || (stats.st_mode & (S_IXUSR | S_IXGRP | S_IXOTH))))
    return 0;

  int granted = (uid == stats.st_uid
		 ? (unsigned int) (stats.st_mode & (mode << 6)) >> 6
		 : (stats.st_gid == ((flag & AT_EACCESS)
				     ? getegid () : getgid ())
		    || group_member (stats.st_gid))
		 ? (unsigned int) (stats.st_mode & (mode << 3)) >> 3
		 : (stats.st_mode & mode));

  if (granted == mode)
    return 0;

  errno = EACCES;
  return -1l;
}
