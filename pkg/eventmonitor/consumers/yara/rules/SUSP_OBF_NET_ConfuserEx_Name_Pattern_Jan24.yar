rule SUSP_OBF_NET_ConfuserEx_Name_Pattern_Jan24 {
    meta:
        description = "Detects Naming Pattern used by ConfuserEx. ConfuserEx is a widely used open source obfuscator often found in malware"
        author = "Jonathan Peters"
        date = "2024-01-03"
        reference = "https://github.com/yck1509/ConfuserEx/tree/master"
        hash = "2f67f590cabb9c79257d27b578d8bf9d1a278afa96b205ad2b4704e7b9a87ca7"
        score = 60
        yarahub_uuid = "cc8ed082-1927-409c-b560-4df800e46f90"
        yarahub_license = "CC BY 4.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
        yarahub_reference_md5 = "c7692b713225ebf0138f3e93ea1e6fa6"
    strings:
        $s1 = "mscoree.dll" ascii
        $s2 = "mscorlib" ascii
        $s3 = "System.Private.Corlib" ascii
        $s4 = "#Strings" ascii
        $s5 = "{ 5F 43 6F 72 [3] 4D 61 69 6E }"
        $name_pattern = "{ E2 ( 80 8? | 81 AA ) E2 [2] E2 [2] E2 [2] E2 [2] E2 [2] E2 [2] E2 [2] E2 [2] E2 [2] E2 [2] E2 [2] E2 [2] E2 [2] E2 [2] E2 [2] E2 [2] E2 [2] E2 [2] E2 [2] E2 [2] E2 [2] E2 [2] E2 [2] E2 [2] E2 [2] E2 [2] E2 [2] E2 [2] E2 [2] E2 [2] E2 [2] E2 [2] E2 [2] E2 [2] E2 [2] E2 [2] E2 [2] E2 [2] E2 [2] E2 80 AE}"
    condition:
        uint16 ( 0 ) == 0x5a4d and 2 of ( $s* ) and #name_pattern > 5
}