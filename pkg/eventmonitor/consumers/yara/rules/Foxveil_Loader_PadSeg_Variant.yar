rule Foxveil_Loader_PadSeg_Variant
{
    meta:
        yarahub_uuid = "1811de0f-6657-4acc-9655-4263f724178d"
        yarahub_license = "CC0 1.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
        yarahub_reference_md5 = "e987075e068bfc3b2e0cc5dd05d8826e"
        description = "Foxveil macOS loader (ClickFix -> AMOS), apph4/cc2 packer generation, 2026-09-23 wrapper: fat x86_64+arm64 linking CoreFoundation+libSystem+libc++, plus one randomly-named section-less LC_SEGMENT_64 per slice with zero vmsize and zero filesize whose vmaddr, small fileoff, prot (0/1) and flags (0/8) are randomised per slice"
        actor = "Foxveil"
        family = "AMOS loader"
        reference_sha256 = "4043c95c58dfa8dedb4f485ea59f849b150d5882f4be8ba36cb98ff9dd67fc4e"
        reference_sha256_emptyseg = "b9aba591f9ea7faccc80e23b5821e14a0fd208979737019c6d61c86e4bb813e9"
        date = "2026-09-23"
        tlp = "CLEAR"

    strings:
        // FAT_CIGAM, nfat_arch = 2, first cputype x86_64 (strings+filesize only, no uint*()).
        $macho_fat = { CA FE BA BE 00 00 00 02 01 00 00 07 }
        $fw1 = "/System/Library/Frameworks/CoreFoundation.framework/Versions/A/CoreFoundation\x00"
        $dylib1 = "/usr/lib/libSystem.B.dylib\x00"
        $dylib2 = "/usr/lib/libc++.1.dylib\x00"

        // The packer's padding segment, once per slice. LC_SEGMENT_64, cmdsize 0x48 (no sections),
        // segname "__" + junk, vmaddr page-aligned in 0x1_xxxx_x000, vmsize 0, fileoff < 0x10000,
        // filesize 0, maxprot/initprot 0 or 1, nsects 0, flags 0 or 8 (SG_PROTECTED_VERSION_1).
        // Up to build 46 every field after vmaddr was fixed (fileoff/prot 0, flags 8, vmaddr
        // 0x1_08xx_xxxx) and Foxveil_Loader_EmptySeg_Variant pinned them; from cc2 build 47 and
        // DANTE build 4 (2026-09-23) vmaddr, fileoff, prot and flags vary per slice. Zero vmsize
        // and filesize with no sections is the part a loader cannot map and so the artefact.
        $padseg = { 19 00 00 00 48 00 00 00 5F 5F [14]
                     00 ?0 ?? ?? 01 00 00 00
                     00 00 00 00 00 00 00 00
                     ?? ?? 00 00 00 00 00 00
                     00 00 00 00 00 00 00 00
                     0? 00 00 00 0? 00 00 00
                     00 00 00 00 0? 00 00 00 }

    // WHY THIS RULE EXISTS (2026-09-23, cc2 build 47 4043c95c / md5 e987075e): the operator
    // randomised the padding segment's vmaddr/fileoff/prot/flags per slice, so
    // Foxveil_Loader_EmptySeg_Variant's $emptyseg (fixed zeros + flags 8) matches at most one
    // slice. The same build newly links CoreGraphics (DANTE build 4: CoreServices); neither is
    // required or forbidden here. Keep EmptySeg_Variant deployed: tighter for builds 28..46.

    condition:
        $macho_fat at 0 and filesize > 204800 and filesize < 4194304 and
        $fw1 and $dylib1 and $dylib2 and #padseg >= 2
}
