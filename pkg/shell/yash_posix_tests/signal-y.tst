# signal-y.tst: yash-specific test of signal handling, part 1
../checkfg || skip="true" # %REQUIRETTY%

cat >eraseps <<\__END__
PS1= PS2=
__END__

test_oe 'SIGINT interrupts interactive shell (+m)' -i +m --rcfile=./eraseps
for i in 1 2 3; do
    echo $i
    "$TESTEE" -c 'kill -s INT $$'
    echo not reached
done
echo - >&2
for i in 4 5 6; do
    echo $i
    kill -s INT $$
    echo not reached
done
echo done
__IN__
1
4
done
__OUT__

-
__ERR__

test_oe 'SIGINT interrupts interactive shell (-m)' -im --rcfile=./eraseps
for i in 1 2 3; do
    echo $i
    "$TESTEE" -c 'kill -s INT $$'
    echo not reached
done
echo - >&2
for i in 4 5 6; do
    echo $i
    kill -s INT $$
    echo not reached
done
echo done
__IN__
1
4
done
__OUT__

-
__ERR__

# vim: set ft=sh ts=8 sts=4 sw=4 et:
