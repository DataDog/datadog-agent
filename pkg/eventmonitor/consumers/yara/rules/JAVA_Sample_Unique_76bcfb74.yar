rule JAVA_Sample_Unique_76bcfb74
{
    meta:
        author = "Marjoriefort"
        description = "Detects Unknown (class, etat binaire)"
        date = "2026-09-25"
        reference_sha256 = "76bcfb7426f6205f207a2974ce49059bca617c563aba7f2f93ade758f1406355"
        yarahub_uuid = "23d01a0a-5dac-40be-ad95-c06621259349"
        famille = "unknown"
        famille_source = "inconnue"
        classe = "class"
        etat = "binaire"
        confidence_suggeree = "low"
        source = "forge_miss M3 v1.9.44"
        yarahub_license = "CC0 1.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
        yarahub_reference_md5 = "c810474aafdfd5581bcd17f0b36dcd84"
        yarahub_reference_link = "https://github.com/Marjoriefort/yara-rules"
    strings:
        $s0 = "client/xenon/imixin/IRaycastContext" ascii
        $s1 = "net/minecraft/class_3959$class_3960" ascii
        $s2 = "net/minecraft/class_3959" ascii
        $s3 = "net/minecraft/class_3959$class_242" ascii
        $s4 = "Lnet/minecraft/class_243;Lnet/minecraft/class_243;Lnet/minecraft/class_3959$class_3960;Lnet/minecraft/class_3959$class_242;Lnet/minecraft/class_1297;)V" ascii
        $s5 = "class_3960" ascii
        $s6 = "class_242" ascii
    condition:
        uint32(0) == 0xBEBAFECA and filesize < 50MB and 3 of them and 1 of ($s0, $s1, $s2, $s3, $s4)
}
