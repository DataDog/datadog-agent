#ifndef _DISCARDERS_TEST_H
#define _DISCARDERS_TEST_H

#include "defs.h"
#include "discarders.h"
#include "baloum.h"

SEC("test/discarders_event_mask")
int test_discarders_event_mask()
{
    u32 mount_id = 123;
    u64 inode = 456;

    int ret = discard_inode(EVENT_OPEN, mount_id, inode, 0, 0);
    assert_zero(ret, "failed to discard the inode");

    struct inode_discarder_t key = {
        .path_key = {
            .ino = inode,
            .mount_id = mount_id,
        }
    };

    struct inode_discarder_params_t *inode_params = bpf_map_lookup_elem(&inode_discarders, &key);
    assert_not_null(inode_params, "unable to find the inode discarder entry");

    ret = mask_has_event(inode_params->params.event_mask, EVENT_OPEN);
    assert_not_zero(ret, "event not found in mask");

    struct is_discarded_by_inode_t params = {
        .discarder_type = EVENT_OPEN,
        .discarder = {
            .path_key.ino = inode,
            .path_key.mount_id = mount_id,
        }
    };

    ret = is_discarded_by_inode(&params);
    assert_not_zero(ret, "inode should be discarded");

    // add another event type
    ret = discard_inode(EVENT_CHMOD, mount_id, inode, 0, 0);
    assert_zero(ret, "failed to discard the inode");

    // check that we have now both open and chmod event discarded
    inode_params = bpf_map_lookup_elem(&inode_discarders, &key);
    assert_not_null(inode_params, "unable to find the inode discarder entry");

    ret = mask_has_event(inode_params->params.event_mask, EVENT_OPEN);
    assert_not_zero(ret, "event not found in mask");

    ret = mask_has_event(inode_params->params.event_mask, EVENT_CHMOD);
    assert_not_zero(ret, "event not found in mask");

    ret = is_discarded_by_inode(&params);
    assert_not_zero(ret, "inode should be discarded");

    params.discarder_type = EVENT_CHMOD;

    ret = is_discarded_by_inode(&params);
    assert_not_zero(ret, "inode should be discarded");

    return 0;
}

SEC("test/discarders_retention")
int test_discarders_retention()
{
    u32 mount_id = 123;
    u64 inode = 456;

    int ret = discard_inode(EVENT_OPEN, mount_id, inode, 0, 0);
    assert_zero(ret, "failed to discard the inode");

    struct inode_discarder_t key = {
        .path_key = {
            .ino = inode,
            .mount_id = mount_id,
        }
    };

    struct inode_discarder_params_t *inode_params = bpf_map_lookup_elem(&inode_discarders, &key);
    assert_not_null(inode_params, "unable to find the inode discarder entry");

    struct is_discarded_by_inode_t params = {
        .discarder_type = EVENT_OPEN,
        .discarder = {
            .path_key.ino = inode,
            .path_key.mount_id = mount_id,
        }
    };

    ret = is_discarded_by_inode(&params);
    assert_not_zero(ret, "inode should be discarded");

    // expire the discarder
    expire_inode_discarders(mount_id, inode);

    // the entry should still be there
    inode_params = bpf_map_lookup_elem(&inode_discarders, &key);
    assert_not_null(inode_params, "unable to find the inode discarder entry");

    // but should be discarded anymore
    ret = is_discarded_by_inode(&params);
    assert_zero(ret, "inode shouldn't be discarded");

    // we shouldn't be able to add a new discarder for the same inode during the retention period
    // TODO(safchain) should return an error value
    ret = discard_inode(EVENT_OPEN, mount_id, inode, 0, 0);
    assert_zero(ret, "failed to discard the inode");

    // shouldn't still be discarded
    ret = is_discarded_by_inode(&params);
    assert_zero(ret, "inode shouldn't be discarded");

    // wait the retention period
    baloum_sleep(get_discarder_retention() + 1);

    // the retention period is now over, we should be able to add a discarder
    ret = discard_inode(EVENT_OPEN, mount_id, inode, 0, 0);
    assert_zero(ret, "failed to discard the inode");

    ret = is_discarded_by_inode(&params);
    assert_not_zero(ret, "inode should be discarded");

    return 0;
}

#endif