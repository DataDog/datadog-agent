rule SUSP_OBF_NET_Eazfuscator_String_Encryption_Jan24 {
    meta:
        description = "Detects .NET images obfuscated with Eazfuscator string encryption. Eazfuscator is a widely used commercial obfuscation solution used by both legitimate software and malware."
        author = "Jonathan Peters"
        date = "2024-01-01"
        reference = "https://www.gapotchenko.com/eazfuscator.net"
        hash = "3a9ee09ed965e3aee677043ba42c7fdbece0150ef9d1382c518b4b96bbd0e442"
        score = 60
        yarahub_uuid = "d4e42e6a-8794-455d-a6a6-3a2d045425e1"
        yarahub_license = "CC BY 4.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
        yarahub_reference_md5 = "e53b462f9fd0e50d98a9d49e84c6a2b3"
    strings:
        $sa1 = "StackFrame" ascii
        $sa2 = "StackTrace" ascii
        $sa3 = "Enter" ascii
        $sa4 = "Exit" ascii
        $op1 = "{ 11 ?? 18 91 11 ?? 1? 91 1F 10 62 60 11 ?? 1? 91 1E 62 60 11 ?? 17 91 1F 18 62 60 }"
        $op2 = "{ D1 28 ?? 00 00 0A 0? 1F 10 63 D1 }"
        $op3 = "{ 1F 10 63 D1 28 [3] 0A }"
        $op4 = "{ 7B ?? 00 00 04 16 91 02 7B ?? 00 00 04 17 91 1E 62 60 02 7B ?? 00 00 04 18 91 1F 10 62 60 02 7B ?? 00 00 04 19 91 1F 18 62 60 }"
    condition:
        uint16 ( 0 ) == 0x5a4d and all of ( $sa* ) and ( 2 of ( $op* ) or #op1 == 2 )
}