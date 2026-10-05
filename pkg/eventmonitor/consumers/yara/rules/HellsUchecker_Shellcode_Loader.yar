rule HellsUchecker_Shellcode_Loader
{
    meta:
        id = "ssI6NI2m8XL6s7Bj_50cli"
        fingerprint = "bb14f18efb80ed58ab1387945573d9d587f58e69e33431f3ff96f0034744569f"
        version = "1.0"
        date = "2026-03-11"
        modified = "2026-03-11"
        status = "RELEASED"
        sharing = "TLP:CLEAR"
        source = "HTTPS://GITHUB.COM/KIRKDERP/YARA"
        author = "kirkderp"
        description = "HellsUchecker x64 shellcode loader - position-independent code with SipHash-variant CTR cipher and aPLib decompression. Targets raw shellcode as injected via Hell's Gate NtCreateSection."
        category = "MALWARE"
        malware = "HELLSUCHECKER"
        mitre_att = "T1055.012"
        reference = "https://derp.ca/blog/hellsuchecker-clickfix-etherhiding"
        triage_score = 10
        triage_description = "HellsUchecker shellcode loader detected in memory. SipHash-CTR cipher + aPLib PE loader."
        yarahub_uuid = "16ef277b-f30e-4710-93b3-b73d575427d8"
        yarahub_license = "CC0 1.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
        yarahub_reference_md5 = "465d5847030acbbe1b6aaad39088d2f1"

    strings:
        // Config token in unencrypted config area (offset 0x002E)
        $config_token = "ebAZZQH7n" ascii

        // Shellcode entry point: POP RCX; PUSH RBP; MOV RBP,RSP; AND RSP,-16; SUB RSP,0x20; CALL +5
        $entry_point = { 59 55 48 89 E5 48 83 E4 F0 48 83 EC 20 E8 05 00 00 00 }

        // SipHash-CTR round: ROL5, SHR27, ROL8, ROR16 rotation constant cluster
        $siphash_round = { C1 E0 05 C1 E9 1B ?? ?? ?? ?? ?? C1 E9 18 ?? ?? ?? ?? ?? C1 E0 08 }

        // aPLib decompressor prologue
        $aplib_prologue = { 48 89 5C 24 08 48 89 6C 24 10 48 89 74 24 18 4C 89 70 20 55 48 8B EC 48 83 EC 40 }

    condition:
        not uint16(0) == 0x5A4D
        and filesize < 100KB
        and $config_token
        and ($entry_point or $siphash_round or $aplib_prologue)
}
