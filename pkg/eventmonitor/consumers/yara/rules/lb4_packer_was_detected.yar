rule lb4_packer_was_detected
{
    meta:
        author = "0x0d4y"
        description = "Detect the packer used by Lockbit4.0"
        date = "2024-02-16"
        score = 100
        yarahub_reference_md5 = "15796971D60F9D71AD162060F0F76A02"
        yarahub_uuid = "3c2b2806-9dce-4dce-a7ca-89ebc9005695"
        yarahub_license = "CC BY 4.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
        malpedia_family = "win.lockbit"
    strings:
        $unpacking_loop_64b = { 8b 1e 48 83 ee fc 11 db 8a 16 72 e5 8d 41 01 41 ff d3 11 c0 01 db 75 0a }
        $jump_to_unpacked_code_64b = { 48 8b 2d 16 0f ?? ?? 48 8d be 00 f0 ?? ?? bb 00 ?? ?? ?? 50 49 89 e1 41 b8 04 ?? ?? ?? 53 5a 90 57 59 90 48 83 ec ?? ff d5 48 8d 87 ?? ?? ?? ?? 80 20 ?? 80 60 ?? ?? 4c 8d 4c 24 ?? 4d 8b 01 53 90 5a 90 57 59 ff d5 48 83 c4 ?? 5d 5f 5e 5b 48 8d 44 24 ?? 6a ?? 48 39 c4 75 f9 48 83 ec ?? e9 ?? ?? ?? ?? }
        $unpacking_loop_32b = { 8A 06 46 88 07 47 01 DB 75 ?? 8B 1E 83 EE ?? 11 DB 72 ?? 9C 29 C0 40 9D 01 DB 75 ?? 8B 1E 83 EE ?? 11 DB 11 C0 01 DB 73 ?? 75 ?? 8B 1E 83 EE ?? 11 DB 73 ?? }        
        $jump_to_unpacked_code_32b = { 8b ae ?? ?? ?? ?? 8d be 00 f0 ?? ?? bb 00 ?? ?? ?? 50 54 6a 04 53 57 ff d5 8d 87 ?? ?? ?? ?? 80 20 ?? 80 60 ?? ?? 58 50 54 50 53 57 ff d5 58 8d 9e 00 f0 ?? ?? 8d bb ?? ?? ?? ?? 57 31 c0 aa 59 49 50 6a 01 53 ff d1 61 8d 44 24 ?? 6a ?? 39 c4 75 fa 83 ec ?? e9 ?? ?? ?? ??}

    condition:
        uint16(0) == 0x5a4d and
        1 of ($jump_to_unpacked_code_*) and
        1 of ($unpacking_loop_*)
}