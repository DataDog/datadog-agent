# random-y.tst: test of the $RANDOM special variable

test_x 'RANDOM yields random numbers'
until [ "$RANDOM" -ne "$RANDOM" ]; do :; done
until [ "$((RANDOM % 7))" -eq 0 ]; do :; done
__IN__

test_x -e 0 'assigning seed to RANDOM' -e
print() {
    RANDOM=123
    echo $RANDOM $RANDOM $RANDOM $RANDOM $RANDOM
    RANDOM=456
    echo $RANDOM $RANDOM $RANDOM $RANDOM $RANDOM
}
print > seeded1
print > seeded2
diff seeded1 seeded2
__IN__

test_o 'assigning non-seed to RANDOM'
(RANDOM=; echo [$RANDOM])
(RANDOM=X; echo [$RANDOM])
__IN__
[]
[X]
__OUT__

test_o 'unset RANDOM'
unset RANDOM
echo ${RANDOM-unset}
RANDOM=123
echo $RANDOM $RANDOM $RANDOM $RANDOM $RANDOM
__IN__
unset
123 123 123 123 123
__OUT__

test_x 'read-only RANDOM'
readonly RANDOM
until [ "$RANDOM" -ne "$RANDOM" ]; do :; done
__IN__

# vim: set ft=sh ts=8 sts=4 sw=4 et:
