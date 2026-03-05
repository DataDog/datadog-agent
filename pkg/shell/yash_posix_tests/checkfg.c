/* checkfg.c: checks if the current process is in the foreground */
/* (C) 2011 magicant */

/* This program is free software: you can redistribute it and/or modify
 * it under the terms of the GNU General Public License as published by
 * the Free Software Foundation, either version 2 of the License, or
 * (at your option) any later version.
 * 
 * This program is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 * GNU General Public License for more details.
 * 
 * You should have received a copy of the GNU General Public License
 * along with this program.  If not, see <http://www.gnu.org/licenses/>.  */

#define _POSIX_C_SOURCE 200112L
#include <fcntl.h>
#include <stdlib.h>
#include <unistd.h>

int main(void)
{
    int ttyfd = open("/dev/tty", O_RDWR | O_NOCTTY | O_NONBLOCK);
    if (ttyfd < 0)
        return EXIT_FAILURE;

    pid_t tpgid = tcgetpgrp(ttyfd);
    if (tpgid < 0)
        return EXIT_FAILURE;

    pid_t pgid = getpgrp();

    return tpgid == pgid ? EXIT_SUCCESS : EXIT_FAILURE;
}

/* vim: set ts=8 sts=4 sw=4 et tw=80: */
