#ifndef _PATH_ID_TEST_H_
#define _PATH_ID_TEST_H_

SEC("test/path_id_mount_bump")
int test_path_id_mount_bump() {
    u32 mount_id1 = 123;
    u32 mount_id2 = 456;

    u32 initial_mount_1_path_id = get_path_id(0, mount_id1, 1, 0);
    u32 initial_mount_2_path_id = get_path_id(0, mount_id2, 1, 0);

    // get path id mount 1 and invalidate the path
    u32 mount_1_path_id_after_invalidation = get_path_id(0, mount_id1, 1, 1);
    assert_equals(mount_1_path_id_after_invalidation, initial_mount_1_path_id, "path id should be the same");
    
    u32 mount_1_next_path_id = get_path_id(0, mount_id1, 1, 0);
    assert_equals(mount_1_next_path_id, mount_1_path_id_after_invalidation + 1, "path id should be incremented");

    u32 mount_2_path_id_after_mount_1_invalidation = get_path_id(0, mount_id2, 1, 0);
    assert_equals(mount_2_path_id_after_mount_1_invalidation, initial_mount_2_path_id, "path id for mount 2 should be left unchanged");

    return 1;
}

SEC("test/path_id_link_bump")
int test_path_id_link_bump() {
    const u32 mount_id = 32;

    u64 inode_1 = 1;
    u64 inode_2 = 2;

    u32 initial_inode1_path_id = get_path_id(inode_1, mount_id, 1, 0);
    u32 initial_inode2_path_id = get_path_id(inode_2, mount_id, 1, 0);

    u32 inode1_path_id_no_link = get_path_id(inode_1, mount_id, 1, 0);
    assert_equals(inode1_path_id_no_link, initial_inode1_path_id, "path id should be the same");

    u32 inode1_path_id_with_link = get_path_id(inode_1, mount_id, 2, 0);
    assert_equals(inode1_path_id_with_link, PATH_ID(1, 0), "path id should be incremented");

    u32 inode2_path_id_with_link = get_path_id(inode_2, mount_id, 1, 0);
    assert_equals(inode2_path_id_with_link, initial_inode2_path_id, "path id for inode 2 should be left unchanged");

    return 1;
}

#endif /* _PATH_ID_TEST_H_ */