rule MAL_NET_NixImports_Loader_Jan24 {
    meta:
        description = "Detects open-source NixImports .NET malware loader. A stealthy loader using dynamic import resolving to evade static detection"
        author = "Jonathan Peters"
        date = "2024-01-12"
        reference = "https://github.com/dr4k0nia/NixImports/tree/master"
        hash = "dd3f22871879b0bc4990c96d1de957848c7ed0714635bb036c73d8a989fb0b39"
        score = 80
        yarahub_uuid = "eb2afac0-581d-4707-8fb1-0a326fd994d0"
        yarahub_license = "CC BY 4.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
        yarahub_reference_md5 = "36ea9201bf6a207a1a6cc7393c604b06"
    strings:
        $op1 = "{ 1F 0A 64 06 1F 11 62 60 }"
        $op2 = "{ 03 20 4D 5A 90 00 94 4B 2A }"
        $op3 = "{ 20 DE 7A 1F F3 20 F7 1B 18 BC }"
        $op4 = "{ 20 CE 1F BE 70 20 DF 1F 3E F8 14 }"
        $sa1 = "OffsetToStringData" ascii
        $sa2 = "GetRuntimeMethods" ascii
        $sa3 = "netstandard" ascii
    condition:
        uint16 ( 0 ) == 0x5a4d and all of ( $sa* ) and 2 of ( $op* )
}