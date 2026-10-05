rule JAVA_Sample_Unique_11d23712
{
    meta:
        author = "Marjoriefort"
        description = "Detects Unknown (class, etat binaire)"
        date = "2026-09-25"
        reference_sha256 = "11d2371205e1932fa22928299ca401aa7212dfa8041b732e5fa780f8e6d6a721"
        yarahub_uuid = "b1a300d6-d7e9-4996-b96e-4d95149eca12"
        famille = "unknown"
        famille_source = "inconnue"
        classe = "class"
        etat = "binaire"
        confidence_suggeree = "low"
        source = "forge_miss M3 v1.9.44"
        yarahub_license = "CC0 1.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
        yarahub_reference_md5 = "05e6c848de1f10eb08f39bbc0ffc4c0e"
        yarahub_reference_link = "https://github.com/Marjoriefort/yara-rules"
    strings:
        $s0 = "com/foure/client/module/modules/donut/RTPFinder" ascii
        $s1 = "8[Lcom/foure/client/module/modules/donut/RTPFinder$State;" ascii
        $s2 = ")[Lcom/foure/client/module/modules/donut/RTPFinder$State;" ascii
    condition:
        uint32(0) == 0xBEBAFECA and filesize < 50MB and 3 of them
}
