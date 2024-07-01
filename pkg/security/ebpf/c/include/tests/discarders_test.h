#ifndef _DISCARDERS_TEST_H
#define _DISCARDERS_TEST_H

#include "helpers/discarders.h"
#include "baloum.h"

int __attribute__((always_inline)) _is_discarded_by_inode(u64 event_type, u32 mount_id, u64 inode) {
    struct is_discarded_by_inode_t params = {
        .discarder_type = event_type,
        .discarder = {
            .path_key.ino = inode,
            .path_key.mount_id = mount_id,
        }
    };

    return is_discarded_by_inode(&params);
}

SEC("test/discarders_event_mask")
int test_discarders_event_mask() {
    u32 mount_id = 123;
    u64 inode = 456;

    int ret = discard_inode(EVENT_OPEN, mount_id, inode, 0, 0);
    assert_zero(ret, "failed to discard the inode");

    struct inode_discarder_params_t *inode_params = get_inode_discarder_params(mount_id, inode, 0);
    assert_not_null(inode_params, "unable to find the inode discarder entry");

    ret = mask_has_event(inode_params->params.event_mask, EVENT_OPEN);
    assert_not_zero(ret, "event not found in mask");

    ret = _is_discarded_by_inode(EVENT_OPEN, mount_id, inode);
    assert_not_zero(ret, "inode should be discarded");

    // add another event type
    ret = discard_inode(EVENT_CHMOD, mount_id, inode, 0, 0);
    assert_zero(ret, "failed to discard the inode");

    // check that we have now both open and chmod event discarded
    inode_params = get_inode_discarder_params(mount_id, inode, 0);
    assert_not_null(inode_params, "unable to find the inode discarder entry");

    ret = mask_has_event(inode_params->params.event_mask, EVENT_OPEN);
    assert_not_zero(ret, "event not found in mask");

    ret = mask_has_event(inode_params->params.event_mask, EVENT_CHMOD);
    assert_not_zero(ret, "event not found in mask");

    ret = _is_discarded_by_inode(EVENT_OPEN, mount_id, inode);
    assert_not_zero(ret, "inode should be discarded");

    ret = _is_discarded_by_inode(EVENT_CHMOD, mount_id, inode);
    assert_not_zero(ret, "inode should be discarded");

    return 0;
}

SEC("test/discarders_retention")
int test_discarders_retention() {
    u32 mount_id = 123;
    u64 inode = 456;

    int ret = discard_inode(EVENT_OPEN, mount_id, inode, 0, 0);
    assert_zero(ret, "failed to discard the inode");

    ret = _is_discarded_by_inode(EVENT_OPEN, mount_id, inode);
    assert_not_zero(ret, "inode should be discarded");

    // expire the discarder
    expire_inode_discarders(mount_id, inode);

    // shouldn't be discarded anymore
    ret = _is_discarded_by_inode(EVENT_OPEN, mount_id, inode);
    assert_zero(ret, "inode shouldn't be discarded");

    // we shouldn't be able to add a new discarder for the same inode during the retention period
    // TODO(safchain) should return an error value
    ret = discard_inode(EVENT_OPEN, mount_id, inode, 0, 0);
    assert_zero(ret, "able to discard the inode");

    // shouldn't still be discarded
    ret = _is_discarded_by_inode(EVENT_CHMOD, mount_id, inode);
    assert_zero(ret, "inode shouldn't be discarded");

    // wait the retention period
    baloum_sleep(get_discarder_retention() + 1);

    // the retention period is now over, we should be able to add a discarder
    ret = discard_inode(EVENT_OPEN, mount_id, inode, 0, 0);
    assert_zero(ret, "failed to discard the inode");

    ret = _is_discarded_by_inode(EVENT_OPEN, mount_id, inode);
    assert_not_zero(ret, "inode should be discarded");

    return 0;
}

SEC("test/discarders_revision")
int test_discarders_revision() {
    u32 mount_id1 = 123;
    u64 inode1 = 456;

    u32 mount_id2 = 456;
    u64 inode2 = 789;

    int ret = discard_inode(EVENT_OPEN, mount_id1, inode1, 0, 0);
    assert_zero(ret, "failed to discard the inode");

    ret = _is_discarded_by_inode(EVENT_OPEN, mount_id1, inode1);
    assert_not_zero(ret, "inode should be discarded");

    ret = discard_inode(EVENT_OPEN, mount_id2, inode2, 0, 0);
    assert_zero(ret, "failed to discard the inode");

    ret = _is_discarded_by_inode(EVENT_OPEN, mount_id2, inode2);
    assert_not_zero(ret, "inode should be discarded");

    // expire the discarders
    bump_discarders_revision();

    // now all the discarders whatever their mount id should be discarded
    ret = _is_discarded_by_inode(EVENT_OPEN, mount_id1, inode1);
    assert_zero(ret, "inode shouldn't be discarded");

    ret = _is_discarded_by_inode(EVENT_OPEN, mount_id2, inode2);
    assert_zero(ret, "inode shouldn't be discarded");

    // check that we added a retention period
    ret = discard_inode(EVENT_OPEN, mount_id1, inode1, 0, 0);
    assert_zero(ret, "able to discard the inode");

    ret = _is_discarded_by_inode(EVENT_OPEN, mount_id1, inode1);
    assert_zero(ret, "inode shouldn't be discarded");

    // wait the retention period
    baloum_sleep(get_discarder_retention() + 1);

    ret = discard_inode(EVENT_OPEN, mount_id1, inode1, 0, 0);
    assert_zero(ret, "able to discard the inode");

    ret = _is_discarded_by_inode(EVENT_OPEN, mount_id1, inode1);
    assert_not_zero(ret, "inode should be discarded");

    return 0;
}

SEC("test/discarders_mount_revision")
int test_discarders_mount_revision() {
    u32 mount_id1 = 123;
    u64 inode1 = 456;

    u32 mount_id2 = 456;
    u64 inode2 = 789;

    int ret = discard_inode(EVENT_OPEN, mount_id1, inode1, 0, 0);
    assert_zero(ret, "failed to discard the inode");

    ret = _is_discarded_by_inode(EVENT_OPEN, mount_id1, inode1);
    assert_not_zero(ret, "inode should be discarded");

    ret = discard_inode(EVENT_OPEN, mount_id2, inode2, 0, 0);
    assert_zero(ret, "failed to discard the inode");

    ret = _is_discarded_by_inode(EVENT_OPEN, mount_id2, inode2);
    assert_not_zero(ret, "inode should be discarded");

    // bump the revision
    bump_mount_discarder_revision(mount_id1);

    // now the inode1 shouldn't be discarded anymore
    ret = _is_discarded_by_inode(EVENT_OPEN, mount_id1, inode1);
    assert_zero(ret, "inode shouldn't be discarded");

    // while node2 should still be
    ret = _is_discarded_by_inode(EVENT_OPEN, mount_id2, inode2);
    assert_not_zero(ret, "inode should be discarded");

    // we are allowed to re-add inode1 right away
    ret = discard_inode(EVENT_OPEN, mount_id1, inode1, 0, 0);
    assert_zero(ret, "failed to discard the inode");

    ret = _is_discarded_by_inode(EVENT_OPEN, mount_id1, inode1);
    assert_not_zero(ret, "inode should be discarded");

    return 0;
}

#endif
