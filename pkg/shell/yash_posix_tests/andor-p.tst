# andor-p.tst: test of and-or lists for any POSIX-compliant shell

posix="true"

test_oE -e 0 '2-command list, success && success'
echo 1 && echo 2
__IN__
1
2
__OUT__

test_oE -e 0 '2-command list, success || success'
echo 1 || echo 2
__IN__
1
__OUT__

test_oE -e n '2-command list, failure && success'
false && echo 2
__IN__
__OUT__

test_oE -e 0 '2-command list, failure || success'
false || echo 2
__IN__
2
__OUT__

test_oE -e n '2-command list, success && failure'
echo 1 && false
__IN__
1
__OUT__

test_oE -e 0 '2-command list, success || failure'
echo 1 || false
__IN__
1
__OUT__

test_oE -e n '2-command list, failure && failure'
false && false
__IN__
__OUT__

test_oE -e n '2-command list, failure || failure'
false || false
__IN__
__OUT__

test_oE '3-command list'
false && echo foo || echo bar
true || echo foo && echo bar
__IN__
bar
bar
__OUT__

test_x -e 0 'exit status of list is from last-executed pipeline (success)'
false && exit 1 || true || exit 1 || exit 2
__IN__

test_x -e 13 'exit status of list is from last-executed pipeline (failure)'
true && (exit 1) || true || exit || exit && (exit 13) && exit 20 && exit 21
__IN__

test_o 'linebreak after &&'
echo 1 &&
    echo 2 &&

    echo 3
__IN__
1
2
3
__OUT__

test_o 'linebreak after ||'
false ||
    false ||

    echo foo
__IN__
foo
__OUT__

test_o 'pipelines in list'
! false && ! true | false && echo foo | cat
__IN__
foo
__OUT__

# vim: set ft=sh ts=8 sts=4 sw=4 et:
