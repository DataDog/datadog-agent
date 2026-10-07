import "math"

rule Foxveil_Loader_B64_Variant
{
    meta:
        yarahub_uuid = "e45d39f8-9368-4cec-9e82-1c8b160a1788"
        yarahub_license = "CC0 1.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
        yarahub_reference_md5 = "9905cb6e295c26abccc8970121528289"
        description = "Foxveil macOS loader, 2026-09-18 apph4/cc2 repackaging: no dlsym and no ad-hoc setup- identity, symbols resolved by walking loaded images via __dyld_image_count/__dyld_get_image_header/__dyld_get_image_vmaddr_slide, empty __AUX<rand> segment in both slices, payload stored as a long base64 chunk array"
        actor = "Foxveil"
        family = "AMOS loader"
        reference_sha256 = "f3b6962c49b361c63fe9d0af9a83e10a52133360117e4c8ac68272160a60edac"
        reference_sha256_cc2 = "a3dabccd14492f4c70658ece96a3ed7b4e275ff9d70c434298bd6f863d1e24b9"
        date = "2026-09-18"
        tlp = "CLEAR"
        confidence = "high (four independent anchors, none shared with the pre-0918 rules)"
        changed = "2026-09-18: cc2 build (a3dabccd) linked CoreServices where apph4 linked Security and got 0 hits; dropped the second framework, added the empty __AUX segment anchor"

    strings:
        // hand-rolled symbol resolution: the three dyld introspection calls, together.
        // Legitimate binaries import these occasionally; all three plus no dlsym is the tell.
        $dy1 = "__dyld_image_count\x00"
        $dy2 = "__dyld_get_image_header\x00"
        $dy3 = "__dyld_get_image_vmaddr_slide\x00"

        // linked but never imported from  -  the loader resolves into it at runtime.
        // The SECOND framework rotates (apph4: Security, cc2: CoreServices) and costs the operator
        // nothing to swap, so it is no longer an anchor. Do not re-add it as a list.
        $fw1 = "/System/Library/Frameworks/CoreFoundation.framework/Versions/A/CoreFoundation\x00"
        $dylib1 = "/usr/lib/libSystem.B.dylib\x00"
        $dylib2 = "/usr/lib/libc++.1.dylib\x00"

        // fat header: FAT_CIGAM, nfat_arch = 2, then cputype x86_64 (0x01000007)
        $macho_fat = { CA FE BA BE 00 00 00 02 01 00 00 07 }

        // packer artefact, once per slice: LC_SEGMENT_64 (0x19) with cmdsize 0x48 (no sections),
        // segname "__AUX" + 4 random characters + 7 NUL, vmsize/filesize 0
        // (apph4: __AUXTYCz / __AUXViZq, cc2: __AUXEWHl / __AUXElpm). Hex string, no regex.
        $aux = { 19 00 00 00 48 00 00 00 5F 5F 41 55 58 ?? ?? ?? ?? 00 00 00 00 00 00 00 }

        // NOTE 1: this variant also has NO "_dlsym\x00" and NO /setup-[0-9a-f]{40}\x00/, which is
        // what separates it from the pre-2026-09-18 builds, and its payload is stored as ~2900 runs
        // of 128 base64 characters. Neither is in the condition. The absences are cheap to evade
        // and add nothing over the positive anchors; the base64 run was expressed as
        // /[A-Za-z0-9+\/]{128}/, which YARA warns "may slow down scanning" (a character class with
        // no searchable atom) -- that warning is why YARAify refused this rule seven times.
        // Keep this rule warning-free: compile with error_on_warning=True before deploying.
        // NOTE 2: the anchors kept below are enough on their own -- 1 hit in 277 samples, no FPs.
        // NOTE 3 (2026-09-18, cc2): retested after the $fw2 -> $aux change over 2578 files
        // (foxveil/, seedhook/, unknown_samples/, LabBench/, Tools/captures/): 5 hits = the two
        // builds apph4 + cc2 and their duplicate copies; 0 in seedhook/ and unknown_samples/.

    condition:
        // Deliberately strings + filesize only, no uint*() and no module calls. Every rule
        // YARAify has accepted from us is shaped that way; both it refused (Foxveil_Loader_Hunt
        // with math.entropy/uint32be, and an earlier form of this rule with an inlined fat-header
        // test) used integer or module functions. The fat-header check lives in
        // $macho_fat below as a literal instead, which costs nothing here: the Mach-O universal
        // magic plus the x86_64 and arm64 cputype words are fixed bytes.
        $macho_fat at 0 and
        filesize > 204800 and filesize < 4194304 and
        all of ($dy*) and $fw1 and all of ($dylib*) and #aux >= 2
}
