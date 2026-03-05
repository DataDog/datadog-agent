# umask-p.tst: test of the umask built-in for any POSIX-compliant shell

posix="true"

# $1 = $LINENO, $2 = umask
test_restore_non_symbolic() {
    testcase "$1" -e 0 \
        "restoring umask using previous output, non-symbolic, $2" \
        3<<\__IN__ 4</dev/null 5<&4
mask=$(umask)
umask 777
umask "$mask"
test "$mask" = "$(umask)"
__IN__
}

test_restore_non_symbolic "$LINENO" 000
test_restore_non_symbolic "$LINENO" 001
test_restore_non_symbolic "$LINENO" 002
test_restore_non_symbolic "$LINENO" 004
test_restore_non_symbolic "$LINENO" 010
test_restore_non_symbolic "$LINENO" 020
test_restore_non_symbolic "$LINENO" 040
test_restore_non_symbolic "$LINENO" 100
test_restore_non_symbolic "$LINENO" 200
test_restore_non_symbolic "$LINENO" 400
test_restore_non_symbolic "$LINENO" 653
test_restore_non_symbolic "$LINENO" 017

# $1 = $LINENO, $2 = umask
test_restore_symbolic() {
    testcase "$1" -e 0 \
        "restoring umask using previous output, symbolic, $2" \
        3<<\__IN__ 4</dev/null 5<&4
mask=$(umask -S)
umask 777
umask "$mask"
test "$mask" = "$(umask -S)"
__IN__
}

test_restore_symbolic "$LINENO" 000
test_restore_symbolic "$LINENO" 001
test_restore_symbolic "$LINENO" 002
test_restore_symbolic "$LINENO" 004
test_restore_symbolic "$LINENO" 010
test_restore_symbolic "$LINENO" 020
test_restore_symbolic "$LINENO" 040
test_restore_symbolic "$LINENO" 100
test_restore_symbolic "$LINENO" 200
test_restore_symbolic "$LINENO" 400
test_restore_symbolic "$LINENO" 653
test_restore_symbolic "$LINENO" 017

(
# $1 = $LINENO, $2 = expected permission, $3 = umask
test_symbolic_operand() {
    testcase "$1" "symbolic operand $3" 3<<__IN__ 4<<__OUT__ 5</dev/null
umask "$3"
mkdir "dir.$1"
ls -dl "dir.$1" | cut -c 1-10
__IN__
$2
__OUT__
}

umask 777

test_symbolic_operand "$LINENO" d--------- u+
test_symbolic_operand "$LINENO" dr-------- u+r
test_symbolic_operand "$LINENO" d-w------- u+w
test_symbolic_operand "$LINENO" d--x------ u+x
test_symbolic_operand "$LINENO" drw------- u+rw
test_symbolic_operand "$LINENO" dr-x------ u+xr
test_symbolic_operand "$LINENO" d-wx------ u+wx
test_symbolic_operand "$LINENO" drwx------ u+xwr

test_symbolic_operand "$LINENO" d--------- g+
test_symbolic_operand "$LINENO" d---r----- g+r
test_symbolic_operand "$LINENO" d----w---- g+w
test_symbolic_operand "$LINENO" d-----x--- g+x
test_symbolic_operand "$LINENO" d---rw---- g+rw
test_symbolic_operand "$LINENO" d---r-x--- g+xr
test_symbolic_operand "$LINENO" d----wx--- g+wx
test_symbolic_operand "$LINENO" d---rwx--- g+xwr

test_symbolic_operand "$LINENO" d--------- o+
test_symbolic_operand "$LINENO" d------r-- o+r
test_symbolic_operand "$LINENO" d-------w- o+w
test_symbolic_operand "$LINENO" d--------x o+x
test_symbolic_operand "$LINENO" d------rw- o+rw
test_symbolic_operand "$LINENO" d------r-x o+xr
test_symbolic_operand "$LINENO" d-------wx o+wx
test_symbolic_operand "$LINENO" d------rwx o+xwr

test_symbolic_operand "$LINENO" d--------- a+
test_symbolic_operand "$LINENO" dr--r--r-- a+r
test_symbolic_operand "$LINENO" d-w--w--w- a+w
test_symbolic_operand "$LINENO" d--x--x--x a+x
test_symbolic_operand "$LINENO" drw-rw-rw- a+rw
test_symbolic_operand "$LINENO" dr-xr-xr-x a+xr
test_symbolic_operand "$LINENO" d-wx-wx-wx a+wx
test_symbolic_operand "$LINENO" drwxrwxrwx a+xwr

test_symbolic_operand "$LINENO" d--------- +
test_symbolic_operand "$LINENO" dr--r--r-- +r
test_symbolic_operand "$LINENO" d-w--w--w- +w
test_symbolic_operand "$LINENO" d--x--x--x +x
test_symbolic_operand "$LINENO" drw-rw-rw- +rw
test_symbolic_operand "$LINENO" dr-xr-xr-x +xr
test_symbolic_operand "$LINENO" d-wx-wx-wx +wx
test_symbolic_operand "$LINENO" drwxrwxrwx +xwr

test_symbolic_operand "$LINENO" d--------- u=
test_symbolic_operand "$LINENO" dr-------- u=r
test_symbolic_operand "$LINENO" d-w------- u=w
test_symbolic_operand "$LINENO" d--x------ u=x
test_symbolic_operand "$LINENO" drw------- u=rw
test_symbolic_operand "$LINENO" dr-x------ u=xr
test_symbolic_operand "$LINENO" d-wx------ u=wx
test_symbolic_operand "$LINENO" drwx------ u=xwr

test_symbolic_operand "$LINENO" drw------- u=r+w
test_symbolic_operand "$LINENO" dr-------- u+w=r
test_symbolic_operand "$LINENO" dr-x------ u+w=r+x

test_symbolic_operand "$LINENO" drw--wxr-x u=r+w,g=wx,o+xr
test_symbolic_operand "$LINENO" dr-x------ u=rwx,u-w

umask 177

test_symbolic_operand "$LINENO" drw-rw---- g=u
test_symbolic_operand "$LINENO" drw----rw- o=u
test_symbolic_operand "$LINENO" drw-rw-rw- og=u
test_symbolic_operand "$LINENO" drw-rw-rw- og=u
test_symbolic_operand "$LINENO" drw-rw---x g+u,o+rwx-u

)

test_OE -e 0 'with operand, -S option is ignored'
umask -S 000
__IN__

# vim: set ft=sh ts=8 sts=4 sw=4 et:
