rule PE_Packer_XtremeProtector {
    meta:
        description = "Detects Windows executables packed with Xtreme-Protector (XProtector)"
        author = "Serhii Kocherhan"
        date = "2026-08-28"
        yarahub_author_twitter = "@skocherhan"
        yarahub_uuid = "278c3786-a058-406c-a5cb-76a7cf417574"
        yarahub_license = "CC0 1.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
        yarahub_reference_md5 = "43349dd33578dd9dc7388894670e0d07"

    strings:
        // Characteristic section names used by Xtreme-Protector variants
        $sec_xp0 = ".XP0" ascii fullword
        $sec_xp1 = ".XP1" ascii fullword
        $sec_xp2 = ".XP2" ascii fullword
        $sec_xtr = ".Xtreme" ascii fullword

        // Unique string artifact embedded in unstripped runtime/stub blocks
        $xp_stub_str = "Xtreme-Protector" ascii wide

        // Decryption/unpacking entry point loop pattern (x86/x64 decryption stub)
        $xp_stub_code = { 60 E8 00 00 00 00 5D 81 ED ?? ?? ?? ?? 8B ?? ?? ?? ?? ?? 8B ?? ?? ?? ?? ?? 8A ?? ?? 30 ?? 40 }

    condition:
        // Must be a Windows PE binary (MZ header + PE header offset check)
        uint16(0) == 0x5A4D and uint32(uint32(0x3C)) == 0x00004550 and
        filesize < 10MB and
        (
            // Primary signature: Matching Xtreme-Protector section header names
            2 of ($sec_*) or
            // Secondary signature: Matching stub strings or decryption code routines
            ($xp_stub_str or $xp_stub_code)
        )
}