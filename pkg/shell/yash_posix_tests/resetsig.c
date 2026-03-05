/* resetsig.c: invokes command with all signal handlers reset */
/* (C) 2009-2011 magicant */

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
#define _XOPEN_SOURCE 600
#include <signal.h>
#include <stdio.h>
#include <unistd.h>
#include "../siglist.h"

int main(int argc, char **argv)
{
    if (argc < 2) {
        fprintf(stderr, "resetsig: too few arguments\n");
        return 2;
    }

    struct sigaction action;
    action.sa_handler = SIG_DFL;
    action.sa_flags = 0;
    sigemptyset(&action.sa_mask);
    sigprocmask(SIG_SETMASK, &action.sa_mask, NULL);
    for (const signal_T *s = signals; s->no != 0; s++)
        if (s->no != SIGKILL && s->no != SIGSTOP)
            sigaction(s->no, &action, NULL);

    execvp(argv[1], &argv[1]);
    perror("invoke: exec failed");
    return 126;
}

/* vim: set ts=8 sts=4 sw=4 et tw=80: */
