# summarize.sh: extracts errors from test results
# (C) 2015-2025 magicant
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

set -Ceu

export LC_ALL=C

uname -a
date
printf '=============\n\n'

ok=0 error=0 skipped=0

for result_file do
    # The "grep" command is generally faster than repeated "read" built-in.
    if [ "$(grep -cE '^%%% (ERROR\[|SKIPPED:)' "$result_file")" -eq 0 ]; then
        ok="$((ok + $(grep -c '^%%% OK\[' "$result_file" || true)))"
        continue
    fi

    log=''
    while IFS= read -r line; do
        log="$log
$line"
        case $line in
            ('%%% START:'*)
                log="$line"
                ;;
            ('%%% OK['*)
                ok="$((ok + 1))"
                ;;
            ('%%% ERROR['*)
                printf '%s\n\n' "$log"
                error="$((error + 1))"
                ;;
            ('%%% SKIPPED:'*)
                printf '%s\n\n' "$line"
                skipped="$((skipped + 1))"
                ;;
        esac
    done <"$result_file"
done

printf '==============\n'
printf 'TOTAL:   %5d\n' "$((ok + error + skipped))"
printf 'OK:      %5d\n' "$ok"
printf 'ERROR:   %5d\n' "$error"
printf 'SKIPPED: %5d\n' "$skipped"
printf '==============\n'

# vim: set ts=8 sts=4 sw=4 et:
