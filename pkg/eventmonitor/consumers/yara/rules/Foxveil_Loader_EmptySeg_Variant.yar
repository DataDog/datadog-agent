rule Foxveil_Loader_EmptySeg_Variant
{
    meta:
        yarahub_uuid = "797529d6-0074-49c3-a547-c313769ae2fd"
        yarahub_license = "CC0 1.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
        yarahub_reference_md5 = "9c7f01a0f2098f7d6f6636647ab04949"
        description = "Foxveil macOS loader (ClickFix -> AMOS), apph4/cc2 packer generation, resolver-agnostic: fat x86_64+arm64 linking only CoreFoundation+libSystem+libc++, plus one empty randomly-named LC_SEGMENT_64 per slice flagged SG_PROTECTED_VERSION_1 with zero vmsize/fileoff/filesize/prot/nsects"
        actor = "Foxveil"
        family = "AMOS loader"
        reference_sha256 = "b9aba591f9ea7faccc80e23b5821e14a0fd208979737019c6d61c86e4bb813e9"
        reference_sha256_apph4 = "f3b6962c49b361c63fe9d0af9a83e10a52133360117e4c8ac68272160a60edac"
        date = "2026-09-22"
        tlp = "CLEAR"
        confidence = "high (92 hits over 4264 files, all Foxveil apph4/cc2 builds and their copies; 0 in seedhook/, gibberdrop/, debugtrail/, unknown_samples/)"

    strings:
        // Fat header as a literal (no uint*() and no module calls: every rule YARAify has accepted
        // from us is strings+filesize only). FAT_CIGAM, nfat_arch = 2, first cputype x86_64.
        $macho_fat = { CA FE BA BE 00 00 00 02 01 00 00 07 }

        // Linked but nothing is imported from it; the loader resolves into it at runtime.
        $fw1 = "/System/Library/Frameworks/CoreFoundation.framework/Versions/A/CoreFoundation\x00"
        $dylib1 = "/usr/lib/libSystem.B.dylib\x00"
        $dylib2 = "/usr/lib/libc++.1.dylib\x00"

        // The packer's padding segment, once per slice. LC_SEGMENT_64 (0x19) with cmdsize 0x48
        // (no sections), segname "__" + junk, vmaddr placed at 0x1_08xx_xxxx (above __LINKEDIT),
        // then vmsize / fileoff / filesize / maxprot / initprot / nsects all zero and
        // flags = SG_PROTECTED_VERSION_1 (8). A zero-length, zero-protection, section-less segment
        // marked "protected" is the artefact; __PAGEZERO fails on vmsize, __LINKEDIT on
        // fileoff/filesize/prot. Structure only, so it survives the segname rotation that broke
        // $aux in Foxveil_Loader_B64_Variant on 2026-09-22 (__AUXlmMk -> __OPSSnXnTNPS).
        $emptyseg = { 19 00 00 00 48 00 00 00 5F 5F [14] 00 ?? ?? 08 01 00 00 00
                      00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00
                      00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00
                      00 00 00 00 08 00 00 00 }

    // WHY THIS RULE EXISTS (2026-09-22, build 28 b9aba591 / md5 9c7f01a0):
    // that build broke two of Foxveil_Loader_B64_Variant's three positive anchor groups at once.
    //   - all of ($dy*): the loader dropped __dyld_image_count / __dyld_get_image_header /
    //     __dyld_get_image_vmaddr_slide and now reaches dyld_all_image_infos through
    //     task_info(mach_task_self(), TASK_DYLD_INFO=17, ...) instead; _task_info and
    //     _mach_task_self_ enter the import table for the first time in the family.
    //   - #aux >= 2: the padding segment kept its exact byte shape but lost the "__AUX" prefix.
    // Both anchors are cheap for the operator to rotate again, so neither is used here. Keep
    // Foxveil_Loader_B64_Variant deployed as well: it is tighter and still covers the
    // 2026-09-18 .. 2026-09-22 dyld-walking builds.

    condition:
        $macho_fat at 0 and filesize > 204800 and filesize < 4194304 and
        $fw1 and $dylib1 and $dylib2 and #emptyseg >= 2
}
