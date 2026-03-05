# disown-y.tst: yash-specific test of the disown built-in

test_OE -e 0 'omitting % in job ID'
sleep 1&
disown sleep
__IN__

test_oE -e 0 'disown is an elective built-in'
command -V disown
__IN__
disown: an elective built-in
__OUT__

test_O -d -e 127 'disown built-in is unavailable in POSIX mode' --posix
echo echo not reached > disown
chmod a+x disown
PATH=$PWD:$PATH
disown --help
__IN__

# vim: set ft=sh ts=8 sts=4 sw=4 et:
