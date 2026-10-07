rule MAL_NET_LimeCrypter_RunPE_Jan24 {
    meta:
        description = "Detects LimeCrypter RunPE module. LimeCrypter is an open source .NET based crypter and loader commonly used by threat actors"
        author = "Jonathan Peters"
        date = "2024-01-16"
        reference = "https://github.com/NYAN-x-CAT/Lime-Crypter/tree/master"
        hash = "bcc8c679acfc3aabf22ebdb2349b1fabd351a89fd23a716d85154049d352dd12"
        score = 80
        yarahub_uuid = "d2810063-3314-465c-afb2-18a42df79436"
        yarahub_license = "CC BY 4.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
        yarahub_reference_md5 = "7bc7e32b32539a9cfa698ee7caea0a6c"
    strings:
        $op1 = "{ 1F 1A 58 1F 1A 58 28 }"
        $op2 = "{ 20 B3 00 00 00 8D ?? 00 00 01 13 ?? 11 ?? 16 20 02 00 01 00 }"
        $op3 = "{ 11 0? 11 0? 20 00 30 00 00 1F 40 28 ?? 00 00 06 }"
        $op4 = "{ 6E 20 FF 7F 00 00 6A FE 02 }"
        $s1 = "RawSecurityDescriptor" ascii
        $s2 = "CommonAce" ascii
    condition:
        uint16 ( 0 ) == 0x5a4d and all of ( $s* ) and 2 of ( $op* )
}