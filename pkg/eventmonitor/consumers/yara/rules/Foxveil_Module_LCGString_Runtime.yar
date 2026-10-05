rule Foxveil_Module_LCGString_Runtime
{
    meta:
        yarahub_uuid = "d540c261-0ad1-42fd-a2e8-ceaf794d33ec"
        yarahub_license = "CC0 1.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
        yarahub_reference_md5 = "9fde93ed3b267513a1f21509cda529fe"
        description = "Foxveil second-stage macOS module family (shared internal C++ codebase behind the clipper and the PTY bot modules): ad-hoc-signed fat x86_64+arm64 Mach-O carrying the packer's empty randomly-named SG_PROTECTED_VERSION_1 segment once per slice, with every literal built on the stack by a compile-time ANSI-C-LCG string obfuscator (0x41c64e6d / 0x3039) and control flow flattened through an indirect-branch table"
        actor = "Foxveil"
        family = "Foxveil module runtime"
        reference_sha256 = "a7c0d024ed24dcbc1b17cbda430a39edb52325c30ad9b7207c0267e43fbec4b7"
        date = "2026-09-22"
        tlp = "CLEAR"
        confidence = "medium-high"

    strings:
        $macho_fat = { CA FE BA BE 00 00 00 02 01 00 00 07 }
        $dylib1 = "/usr/lib/libSystem.B.dylib\x00"
        $dylib2 = "/usr/lib/libc++.1.dylib\x00"
        $lcg_x64 = { 6D 4E C6 41 }
        $lcg_a64 = { ?? CD 89 52 ?? 38 A8 72 }
        $emptyseg = { 19 00 00 00 48 00 00 00 5F 5F [14] 00 ?? ?? 08 01 00 00 00
                      00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00
                      00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00
                      00 00 00 00 08 00 00 00 }

    condition:
        $macho_fat at 0 and filesize > 131072 and filesize < 8388608 and
        $dylib1 and $dylib2 and #emptyseg >= 2 and
        #lcg_x64 >= 20 and #lcg_a64 >= 20
}
