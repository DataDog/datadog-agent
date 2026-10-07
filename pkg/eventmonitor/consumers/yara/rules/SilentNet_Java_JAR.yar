rule SilentNet_Java_JAR
{
    meta:
        author = "Farish"
        description = "Detects SilentNet Java malware JAR"
        family = "SilentNet"
        confidence = "Medium"
        date = "2026-06-28"
        yarahub_reference_md5 = "a1856636846948b488f2172028084294"
        yarahub_uuid = "1ff52932-f17b-440e-baa4-e0d501ecc938"
        yarahub_license = "CC0 1.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
    

    strings:

        $c1 = "com/github/ltPFJTIjJ.class" ascii
        $c2 = "com/github/HYCxXNej.class" ascii
        $c3 = "com/github/qiez_unbed_vowo.class" ascii
        $c4 = "com/github/aEjghhhsj.class" ascii

    condition:

        uint32(0) == 0x04034B50 and

        all of them
}
