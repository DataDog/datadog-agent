rule SUSP_OBF_NET_Eazfuscator_Virtualization_Jan24 {
    meta:
        description = "Detects .NET images obfuscated with Eazfuscator virtualization protection. Eazfuscator is a widely used commercial obfuscation solution used by both legitimate software and malware."
        author = "Jonathan Peters"
        date = "2024-01-02"
        reference = "https://www.gapotchenko.com/eazfuscator.net"
        hash = "53d5c2574c7f70b7aa69243916acf6e43fe4258fbd015660032784e150b3b4fa"
        score = 60
        yarahub_uuid = "4f140b0d-540d-495e-9a0d-7c4630802f78"
        yarahub_license = "CC BY 4.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
        yarahub_reference_md5 = "0d61d50067eb93ae9ce049be387ae4e2"
    strings:
        $sa1 = "BinaryReader" ascii
        $sa2 = "GetManifestResourceStream" ascii
        $sa3 = "get_HasElementType" ascii
        $op1 = "{ 28 [2] 00 06 28 [2] 00 06 72 [2] 00 70 ?? 1? 2D 0? 26 26 26 26 2B }"
        $op2 = "{ 7E [3] 04 2D 3D D0 [3] 02 28 [3] 0A 6F [3] 0A 72 [3] 70 6F [3] 0A 20 80 00 00 00 8D ?? 00 00 01 25 D0 [3] 04 28 [3] 0A 28 [3] 06 28 [3] 06 80 [3] 04 7E [3] 04 2A }"
        $op3 = "{ 02 20 [4] 1F 09 73 [4] 7D [3] 04 }"
    condition:
        uint16 ( 0 ) == 0x5a4d and all of ( $sa* ) and 2 of ( $op* )
}