# enqueue.sh: runs a task sequentially
# (C) 2015-2018 magicant
#
# This program is free software: you can redistribute it and/or modify
# it under the terms of the GNU General Public License as published by
# the Free Software Foundation, either version 2 of the License, or
# (at your option) any later version.
# 
# This program is distributed in the hope that it will be useful,
# but WITHOUT ANY WARRANTY; without even the implied warranty of
# MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
# GNU General Public License for more details.
# 
# You should have received a copy of the GNU General Public License
# along with this program.  If not, see <http://www.gnu.org/licenses/>.

# This script expects any number of (but usually one or more) operands. The
# operands are considered as a command and arguments. The script enqueues the
# command so that enqueued commands are run sequentially (not in parallel).
#
# The purpose of this script is to ensure that tests of job-control are run in
# the foreground. Since at most one process group can be in the foreground,
# those tests cannot be run in parallel.
#
# This script assumes all commands in the same queue are run in the same
# working directory, with the same environment variables, etc.

set -Ceu
umask u+rwx

if [ $# -eq 0 ]; then
    exit
fi

queue_dir="tmp.queue"
created_queue_dir="false"
tmp_file="tmp.$$"
interrupted=""

cleanup() {
    if "$created_queue_dir"; then
        until ! [ -d "$queue_dir" ] || rm -fr "$queue_dir"; do sleep 1; done
    fi
    rm -fr "$tmp_file"
    if [ "$interrupted" ]; then
        trap - "$interrupted"
        kill -s "$interrupted" $$
        exit 1
    fi
}
trap cleanup EXIT

trap 'interrupted=${interrupted:-INT}'  INT
trap 'interrupted=${interrupted:-TERM}' TERM
trap 'interrupted=${interrupted:-QUIT}' QUIT
trap 'interrupted=${interrupted:-HUP}'  HUP

# Write the command in a temporary file and move it into the queue directory.
# Don't make the file in the directory directly so that other instances of this
# script don't see the file in an intermediate state.
printf '%s\n' "$@" >|"$tmp_file"

trial=0 n=0
until cmd_file="$queue_dir/$$.$n"; ln "$tmp_file" "$cmd_file" 2>/dev/null; do
    if [ -e "$cmd_file" ]; then
        n=$((n+1))
        continue
    fi

    if mkdir "$queue_dir" 2>/dev/null; then
        created_queue_dir="true"
    elif ! [ -d "$queue_dir" ]; then
        trial=$((trial+1))
        if [ "$trial" -gt 10 ]; then
            printf '%s: cannot create the queue directory.\n' "$0" >&2
            exit 69 # sysexits.h EX_UNAVAILABLE
        fi
    fi
done

if ! "$created_queue_dir"; then
    # The queue directory has been created by another shell instance running
    # this script. That instance is responsible for running all enqueued
    # commands.
    exit
fi

IFS='
'
until [ "$interrupted" ] || rmdir "$queue_dir" 2>/dev/null; do
    if ! [ -d "$queue_dir" ]; then
        printf '%s: the queue directory was unexpectedly removed.\n' "$0" >&2
        exit 69 # sysexits.h EX_UNAVAILABLE
    fi

    for file in "$queue_dir"/*; do
        $(cat "$file")
        rm "$file"
    done
done

# vim: set ts=8 sts=4 sw=4 et:
