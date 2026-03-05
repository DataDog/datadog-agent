# export-y.tst: yash-specific test of the export built-in

# XXX: missing test 'printing all exported variables'

test_oE -e 0 'printing specific exported variables'
export a=A f=FOO
export -p a f
__IN__
export a=A
export f=FOO
__OUT__

test_OE -e 0 'without argument, -p is assumed'
export >withoutp.out
export -p >withp.out
diff withoutp.out withp.out
__IN__

test_oE -e 0 'assigning empty value'
export a=
export -p a
__IN__
export a=''
__OUT__

test_oE 'exporting with -p'
export -p a=A
export -p a
__IN__
export a=A
__OUT__

test_oE 'un-exporting'
export a=A
export -X a
sh -c 'echo ${a-unset}'
__IN__
unset
__OUT__

test_Oe -e 1 'assigning to read-only variable'
readonly a=A
export a=X
__IN__
export: $a is read-only
__ERR__

test_oE 'exporting before separate assignment'
export a
a=A
sh -c 'echo $a'
__IN__
A
__OUT__

test_oE -e 0 'separator preceding scalar variable name starting with -' -e
export -- -a=1
export -p -- -a
__IN__
export -- -a=1
__OUT__

test_O -d -e 1 'assigning to ill-named variable'
export =A
__IN__

(
posix="true"

test_Oe -e 2 'invalid option -r (POSIX)'
export -r
__IN__
export: `-r' is not a valid option
__ERR__
#'
#`

test_Oe -e 2 'invalid option -X (POSIX)'
export -X
__IN__
export: `-X' is not a valid option
__ERR__
#'
#`

)

test_Oe -e 2 'invalid option -z'
export -z
__IN__
export: `-z' is not a valid option
__ERR__
#'
#`

test_Oe -e 2 'invalid option --xxx'
export --no-such=option
__IN__
export: `--no-such=option' is not a valid option
__ERR__
#'
#`

test_O -d -e 1 'printing to closed stream'
export >&-
__IN__

test_Oe -e 1 'printing non-existing variable'
unset a
export -p a
__IN__
export: no such variable $a
__ERR__

# vim: set ft=sh ts=8 sts=4 sw=4 et:
