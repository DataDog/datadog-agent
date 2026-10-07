rule JAVA_Sample_Unique_5ae7601b
{
    meta:
        author = "Marjoriefort"
        description = "Detects Unknown (class, etat binaire)"
        date = "2026-09-25"
        reference_sha256 = "5ae7601b3a2d589eef612530c845eb54c6f1029fddd34ff057253a8baf4b8c19"
        yarahub_uuid = "d1303f3b-e797-4a01-a43c-64fbd9a5ab39"
        famille = "unknown"
        famille_source = "inconnue"
        classe = "class"
        etat = "binaire"
        confidence_suggeree = "low"
        source = "forge_miss M3 v1.9.44"
        yarahub_license = "CC0 1.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
        yarahub_reference_md5 = "ba37bf609c7c99551ea4be2c5b98f254"
        yarahub_reference_link = "https://github.com/Marjoriefort/yara-rules"
    strings:
        $s0 = "com/nnpg/glazed/modules/esp/SpawnerNotifier" ascii
        $s1 = "3[Lcom/nnpg/glazed/modules/esp/SpawnerNotifier$Mode;" ascii
        $s2 = "5()[Lcom/nnpg/glazed/modules/esp/SpawnerNotifier$Mode;" ascii
    condition:
        uint32(0) == 0xBEBAFECA and filesize < 50MB and 3 of them
}
