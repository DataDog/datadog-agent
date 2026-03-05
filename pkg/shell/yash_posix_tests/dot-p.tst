# dot-p.tst: test of the dot built-in for any POSIX-compliant shell

posix="true"

cat <<\__END__ >file1
echo $?
(exit 3)
__END__

cat <<\__END__ >file2
echo in
. ./file1
echo out
__END__

cat <<\__END__ >file3
exit 11
__END__

test_OE -e 0 'empty dot script'
(exit 1)
. /dev/null
__IN__

test_oE -e 3 'non-empty dot script'
(exit 5)
. ./file1
__IN__
5
__OUT__

test_oE -e 0 'recursive dot script'
. ./file2
__IN__
in
0
out
__OUT__

test_e 'with verbose option' -v
. ./file3
__IN__
. ./file3
exit 11
__ERR__

test_oE -e 3 'option-operand separator'
(exit 5)
. -- ./file1
__IN__
5
__OUT__

(
# Ensure $PWD is safe to assign to $PATH
case $PWD in (*[:%]*)
    skip="true"
esac

setup 'savepath=$PATH; PATH=$PWD'

test_OE -e 11 'dot script in $PATH'
. file3
__IN__

test_O -d -e n 'dot script not found, in $PATH, non-interactive shell'
. _no_such_file_
PATH=$savepath
echo not reached
__IN__

test_o -d 'dot script not found, in $PATH, subshell, exiting'
(. _no_such_file_)
PATH=$savepath
echo reached
__IN__
reached
__OUT__

test_O -d -e n 'dot script not found, in $PATH, subshell, exit status'
(. _no_such_file_)
__IN__

test_o -d 'dot script not found, in $PATH, interactive shell, no exiting' -i +m
. _no_such_file_
PATH=$savepath
echo reached
__IN__
reached
__OUT__

test_O -d -e n 'dot script not found, in $PATH, interactive shell, exit status' -i +m
. _no_such_file_
__IN__

)

test_O -d -e n 'dot script not found, relative, non-interactive shell'
. ./_no_such_file_
echo not reached
__IN__

test_o -d 'dot script not found, relative, subshell, exiting'
(. ./_no_such_file_)
echo reached
__IN__
reached
__OUT__

test_O -d -e n 'dot script not found, relative, subshell, exit status'
(. ./_no_such_file_)
__IN__

test_o -d 'dot script not found, relative, interactive shell, no exiting' -i +m
. ./_no_such_file_
echo reached
__IN__
reached
__OUT__

test_O -d -e n 'dot script not found, relative, interactive shell, exit status' -i +m
. ./_no_such_file_
__IN__

# vim: set ft=sh ts=8 sts=4 sw=4 et:
