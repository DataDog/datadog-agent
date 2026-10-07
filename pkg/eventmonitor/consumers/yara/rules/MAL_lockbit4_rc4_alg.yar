rule MAL_lockbit4_rc4_alg
{
    meta:
        author = "0x0d4y"
        description = "Detect the implementation of RC4 Algorithm by Lockbit4.0"
        date = "2024-02-13"
        score = 100
        yarahub_reference_md5 = "062311F136D83F64497FD81297360CD4"
        yarahub_uuid = "4de48ced-b9fa-4286-aac4-c263ad20d67d"
        yarahub_license = "CC BY 4.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
        malpedia_family = "win.lockbit"
    strings:
        $rc4_alg = { 48 3d 00 01 00 00 74 0c 88 84 04 ?? ?? ?? ?? 48 ff c0 eb ec 29 c9 41 b8 ?? ?? ?? ?? 4c 8d 0d 15 7b 00 00 45 31 d2 48 81 f9 00 01 00 00 74 34 44 8a 9c 0c ?? ?? ?? ?? 45 00 da 89 c8 99 41 f7 f8 46 02 14 0a 41 0f b6 c2 8a 94 04 ?? ?? ?? ?? 88 94 0c ?? ?? ?? ?? 44 88 9c 04 ?? ?? ?? ?? 48 ff c1 eb c3 29 c0 48 8b 0d 14 9e 00 00 31 d2 45 29 c0 48 3d ?? ?? ?? ?? 74 4b 41 ff c0 45 0f b6 c0 46 8a 8c 04 ?? ?? ?? ?? 44 00 ca 44 0f b6 d2 46 8a 9c 14 ?? ?? ?? ?? 46 88 9c 04 ?? ?? ?? ?? 46 88 8c 14 ?? ?? ?? ?? 46 02 8c 04 ?? ?? ?? ?? 45 0f b6 c9 46 8a 8c 0c ?? ?? ?? ?? 44 30 0c 01 48 ff c0 eb ad }
        
    condition:
        uint16(0) == 0x5a4d and
        $rc4_alg
}