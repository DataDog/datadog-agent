# declutil-p.tst: test of declaration utilities for any POSIX-compliant shell

posix="true"

# Pathname expansion may match this dummy file in incorrect implementations.
>tmpfile

test_oE 'no pathname expansion or field splitting in export A=$a'
a="1  *  2"
export A=$a
sh -c 'printf "%s\n" "$A"'
__IN__
1  *  2
__OUT__

test_oE 'tilde expansions in export A=~:~' 
HOME=/foo
export A=~:~
sh -c 'printf "%s\n" "$A"'
__IN__
/foo:/foo
__OUT__

test_oE 'pathname expansion and field splitting in export $a'
A=foo B=bar a='A B'
export $a
sh -c 'printf "%s\n" "$A" "$B"'
__IN__
foo
bar
__OUT__

test_oE 'no pathname expansion or field splitting in readonly A=$a'
a="1  *  2"
readonly A=$a
printf "%s\n" "$A"
__IN__
1  *  2
__OUT__

test_oE 'tilde expansions in readonly A=~:~'
HOME=/foo
readonly A=~:~
printf "%s\n" "$A"
__IN__
/foo:/foo
__OUT__

test_oE 'pathname expansion and field splitting in readonly $a'
A=foo B=bar a='A B'
readonly $a
printf "%s\n" "$A" "$B"
__IN__
foo
bar
__OUT__

test_oE 'command command export'
a="1  *  2"
command command export A=$a
sh -c 'printf "%s\n" "$A"'
__IN__
1  *  2
__OUT__

test_oE 'command command readonly'
a="1  *  2"
command command readonly A=$a
printf "%s\n" "$A"
__IN__
1  *  2
__OUT__

# POSIX allows any utility to be a declaration utility as an extension,
# so there are no tests to check that a utility is not a declaration utility.

# vim: set ft=sh ts=8 sts=4 sw=4 et:
