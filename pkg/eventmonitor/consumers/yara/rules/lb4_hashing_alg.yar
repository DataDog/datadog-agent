rule lb4_hashing_alg
{
    meta:
        author = "0x0d4y"
        description = "This rule detects the custom hashing algorithm of Lockbit4.0 unpacked"
        date = "2024-02-16"
        score = 100
        yarahub_reference_md5 = "062311F136D83F64497FD81297360CD4"
        yarahub_uuid = "e91aedba-6f70-4ca2-9217-2991cbbc6e8d"
        yarahub_license = "CC BY 4.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
        malpedia_family = "win.lockbit"
    strings:
        $hashing_alg = { 41 89 d0 46 0f be 04 00 45 09 c0 74 ?? 45 8d 48 ?? 45 8d 50 ?? 41 80 f9 ?? 45 0f 43 d0 44 31 d1 44 8d 04 3a 45 0f af c2 41 01 c8 89 d1 31 f9 09 d2 0f 44 ca 41 0f af c8 44 01 d1 ff c2 eb ?? 49 ff c6 }
        
    condition:
        uint16(0) == 0x5a4d and
        $hashing_alg
}