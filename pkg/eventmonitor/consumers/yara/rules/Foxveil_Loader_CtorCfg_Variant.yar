rule Foxveil_Loader_CtorCfg_Variant
{
    meta:
        yarahub_uuid = "9267bdb4-03d1-4513-a84b-5754f7763802"
        yarahub_license = "CC0 1.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
        yarahub_reference_md5 = "e49128e81aa8cb1430bbe48697448f67"
        description = "Foxveil macOS loader (ClickFix -> AMOS), quill generation without the padding segment: fat x86_64+arm64 linking CoreFoundation+libSystem+libc++ but importing nothing from CoreFoundation, with the loader's fixed import core (task_info/mach_task_self_ resolver, open/write/unlink/setenv/unsetenv run-time config file) in both slices inside a per-build random libc decoy import set"
        actor = "Foxveil"
        family = "AMOS loader"
        reference_sha256 = "dba7da5c0b29cb80fa942894a52032e319538ed390f63398308199084f51fb9b"
        reference_sha256_sibling = "e5bcbe70bb0b4a19371259befc48de05f18001630d9ea803b17088cd7b1e292f"
        date = "2026-09-27"
        tlp = "CLEAR"

    strings:
        // Fat header as a literal (strings+filesize only, no uint*() or modules, as YARAify wants).
        $macho_fat = { CA FE BA BE 00 00 00 02 01 00 00 07 }
        $fw1 = "/System/Library/Frameworks/CoreFoundation.framework/Versions/A/CoreFoundation\x00"
        $dylib1 = "/usr/lib/libSystem.B.dylib\x00"
        $dylib2 = "/usr/lib/libc++.1.dylib\x00"

        // The import core the loader really uses, NUL-bounded so _fopen/_fwrite do not count.
        // task_info(mach_task_self(), TASK_DYLD_INFO) replaced the dyld trio on 2026-09-22; the
        // constructor writes its run-time settings to a 0600 /tmp file and passes the path in an
        // env var (quill builds since 2026-09-25). Everything else in the import table is decoy
        // libc that changes every build.
        $i_ti  = "\x00_task_info\x00"
        $i_mts = "\x00_mach_task_self_\x00"
        $i_op  = "\x00_open\x00"
        $i_wr  = "\x00_write\x00"
        $i_ul  = "\x00_unlink\x00"
        $i_se  = "\x00_setenv\x00"
        $i_use = "\x00_unsetenv\x00"

        // CoreFoundation is linked but never imported from: the loader resolves into it at run time.
        $cf_imp   = "\x00_CF"
        $objc_imp = "\x00_objc_"

    // WHY THIS RULE EXISTS (2026-09-27, quill build 29 dba7da5c / md5 e49128e8, and build 28
    // e5bcbe70 built three hours earlier): both dropped the section-less zero-size padding
    // LC_SEGMENT_64 that Foxveil_Loader_PadSeg_Variant needs twice (#padseg >= 2), so no loader
    // rule matched. The load-command count stayed at 21/22 because a Foundation LC_LOAD_DYLIB took
    // its place. The padding segment, segment names, second/third framework, signing identity and
    // the decoy libc imports all rotate cheaply; the import core below is what the loader calls.

    condition:
        $macho_fat at 0 and filesize > 204800 and filesize < 4194304 and
        $fw1 and $dylib1 and $dylib2 and
        #i_ti >= 2 and #i_mts >= 2 and #i_op >= 2 and #i_wr >= 2 and
        #i_ul >= 2 and #i_se >= 2 and #i_use >= 2 and
        not $cf_imp and not $objc_imp
}
