#ifndef _PATH_ID_TEST_H_
#define _PATH_ID_TEST_H_

SEC("test/path_id_mount_and_invalidation")
int test_path_id_mount_and_invalidation() {
    const u32 mount_id1 = 123;
    const u32 mount_id2 = 456;

    // get path id mount 1 and invalidate the path
    u32 mount_1_path_id_after_invalidation = get_path_id(0, mount_id1, 1, PATH_ID_INVALIDATE_TYPE_LOCAL);
    assert_equals(mount_1_path_id_after_invalidation, PATH_ID(0, 0), "path id should be the same");

    u32 mount_1_next_path_id = get_path_id(0, mount_id1, 1, PATH_ID_INVALIDATE_TYPE_NONE);
    assert_equals(mount_1_next_path_id, PATH_ID(0, 1), "path id should have only high id incremented");

    u32 mount_2_path_id_after_mount_1_invalidation = get_path_id(0, mount_id2, 1, PATH_ID_INVALIDATE_TYPE_NONE);
    assert_equals(mount_2_path_id_after_mount_1_invalidation, PATH_ID(0, 1), "path id should have only high id incremented");

    // simulate a mount release
    bump_high_path_id(mount_id1);

    u32 mount_1_path_id_after_release = get_path_id(0, mount_id1, 1, PATH_ID_INVALIDATE_TYPE_NONE);
    assert_equals(mount_1_path_id_after_release, PATH_ID(1, 1), "path id should have low and high id incremented");

    return 1;
}

SEC("test/path_id_link_and_invalidation")
int test_path_id_link_and_invalidation() {
    const u32 mount_id = 32;
    const u64 inode_1 = 1;
    const u64 inode_2 = 2;

    u32 inode1_path_id_no_link = get_path_id(inode_1, mount_id, 1, PATH_ID_INVALIDATE_TYPE_NONE);
    assert_equals(inode1_path_id_no_link, PATH_ID(0, 0), "path id should be the same");

    u32 inode1_path_id_with_link = get_path_id(inode_1, mount_id, 2, PATH_ID_INVALIDATE_TYPE_NONE);
    assert_equals(inode1_path_id_with_link, PATH_ID(0, 1), "path id should be incremented");

    u32 inode2_path_id_with_link = get_path_id(inode_2, mount_id, 1, PATH_ID_INVALIDATE_TYPE_NONE);
    assert_equals(inode2_path_id_with_link, PATH_ID(0, 0), "path id for inode 2 should be left unchanged");

    u32 inode2_path_id_with_link_and_invalidation = get_path_id(inode_2, mount_id, 2, PATH_ID_INVALIDATE_TYPE_LOCAL);
    assert_equals(inode2_path_id_with_link_and_invalidation, PATH_ID(0, 1), "path id for inode 2 should have high id incremented");

    return 1;
}

SEC("test/path_id_rename_and_invalidation")
int test_path_id_rename_and_invalidation() {
    const u32 mount_id = 32;
    const u64 src_inode = 1;

    // invalidate local path id for rename, next get_path_id should a new path id
    u32 src_inode_path_id_with_rename = get_path_id(src_inode, mount_id, 1, PATH_ID_INVALIDATE_TYPE_LOCAL);
    assert_equals(src_inode_path_id_with_rename, PATH_ID(0, 0), "path id for src inode should be the same");

    // get path id again should have high id incremented
    src_inode_path_id_with_rename = get_path_id(src_inode, mount_id, 1, PATH_ID_INVALIDATE_TYPE_NONE);
    assert_equals(src_inode_path_id_with_rename, PATH_ID(0, 1), "path id for src inode should have high id incremented");

    // simulate a rename directory
    u32 src_inode_path_id_with_rename_directory = get_path_id(src_inode, mount_id, 1, PATH_ID_INVALIDATE_TYPE_GLOBAL);
    assert_equals(src_inode_path_id_with_rename_directory, PATH_ID(0, 1), "path for src inode should be the same");

    // simulate a rename directory and invalidate local path id
    src_inode_path_id_with_rename_directory = get_path_id(src_inode, mount_id, 1, PATH_ID_INVALIDATE_TYPE_NONE);
    assert_equals(src_inode_path_id_with_rename_directory, PATH_ID(1, 1), "path for src inode should have low and high id incremented");

    return 1;
}

#endif /* _PATH_ID_TEST_H_ */