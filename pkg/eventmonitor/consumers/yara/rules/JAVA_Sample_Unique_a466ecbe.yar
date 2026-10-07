rule JAVA_Sample_Unique_a466ecbe
{
    meta:
        author = "Marjoriefort"
        description = "Detects Unknown (class, etat binaire)"
        date = "2026-09-25"
        reference_sha256 = "a466ecbe5a1e034279c7f7c4d2fecdbfd69e27f4f77ef315402ab90e8fc1869c"
        yarahub_uuid = "965d9484-0522-4503-81a3-3648209c6570"
        famille = "unknown"
        famille_source = "inconnue"
        classe = "class"
        etat = "binaire"
        confidence_suggeree = "low"
        source = "forge_miss M3 v1.9.44"
        yarahub_license = "CC0 1.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
        yarahub_reference_md5 = "b85367588c651bfb31100e350e58a50c"
        yarahub_reference_link = "https://github.com/Marjoriefort/yara-rules"
    strings:
        $s0 = "client/xenon/imixin/IExplosion" ascii
        $s1 = "Lnet/minecraft/class_243;FZ)V" ascii
    condition:
        uint32(0) == 0xBEBAFECA and filesize < 50MB and 2 of them
}
