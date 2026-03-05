# settty-y.tst: yash-specific test of the set built-in
../checkfg || skip="true" # %REQUIRETTY%

test_x -e 0 'monitor (short) on: $-'
set -m &&
printf '%s\n' "$-" | grep -q m
__IN__

test_x -e 0 'monitor (short) off: $-' -m
set +m &&
printf '%s\n' "$-" | grep -qv m
__IN__

test_x -e 0 'monitor (long) on: $-'
set -o monitor &&
printf '%s\n' "$-" | grep -q m
__IN__

test_x -e 0 'monitor (long) off: $-' -m
set +o monitor &&
printf '%s\n' "$-" | grep -qv m
__IN__

# vim: set ft=sh ts=8 sts=4 sw=4 et:
