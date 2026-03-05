# exec-y.tst: yash-specific test of the exec built-in

test_O -d -e 127 'executing non-existing command (empty path)'
PATH=
exec _no_such_command_
echo not reached
__IN__

test_oE -e 0 'executing with specific name (-a)'
exec -a foo sh -c 'echo "$0"'
echo not reached
__IN__
foo
__OUT__

test_oE -e 0 'executing with specific name (--as)'
exec --as=foo sh -c 'echo "$0"'
echo not reached
__IN__
foo
__OUT__

# This test fails on some environments, notably Cygwin, which implicitly adds
# some environment variables on exec'ing.
#test_OE -e 0 'clearing environment variables (-c)'
#exec -c env
#__IN__

(
# We need an absolute path to the "env" command because it cannot be found with
# $PATH cleared.
export ENVCMD="$(command -v env)"

test_OE -e 0 'clearing environment variables (-c)'
"$ENVCMD" -i "$ENVCMD" | sort >1.expected
(exec -c "$ENVCMD") | sort >1.actual
diff 1.expected 1.actual
__IN__

test_OE -e 0 'clearing environment variables (--clear)'
"$ENVCMD" -i "$ENVCMD" | sort >2.expected
(exec --clear "$ENVCMD") | sort >2.actual
diff 2.expected 2.actual
__IN__

test_OE -e 0 'clearing and adding scalar environment variables'
"$ENVCMD" -i FOO=1 BAR=2 "$ENVCMD" | sort >3.expected
(FOO=1 BAR=2 exec -c "$ENVCMD") | sort >3.actual
diff 3.expected 3.actual
__IN__

test_OE -e 0 'clearing and adding array environment variables'
"$ENVCMD" -i FOO=1:2:3 BAR=abc:xyz "$ENVCMD" | sort >4.expected
(FOO=(1 2 3) BAR=(abc xyz) exec -c "$ENVCMD") | sort >4.actual
diff 4.expected 4.actual
__IN__

)

# The -f (--force) option is not tested because it cannot be tested without
# producing garbage processes.

(
posix="true"

test_Oe -e n 'invalid option'
exec -a echo echo echo
__IN__
exec: `-a' is not a valid option
__ERR__
#'
#`

test_Oe -e n 'invalid option'
exec -c echo echo echo
__IN__
exec: `-c' is not a valid option
__ERR__
#'
#`

test_Oe -e n 'invalid option'
exec -f echo echo echo
__IN__
exec: `-f' is not a valid option
__ERR__
#'
#`

)

test_Oe -e 2 'invalid option'
exec --no-such-option
__IN__
exec: `--no-such-option' is not a valid option
__ERR__
#'
#`

# vim: set ft=sh ts=8 sts=4 sw=4 et:
