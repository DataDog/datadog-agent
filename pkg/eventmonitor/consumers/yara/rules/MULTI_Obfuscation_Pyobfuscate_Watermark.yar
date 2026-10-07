rule MULTI_Obfuscation_Pyobfuscate_Watermark {
    meta:
        description = "Payload Python obfusque par pyobfuscate.com : filigrane __obfuscated_by__ en base64 dans le CLIENT_CODE"
        author      = "Marjoriefort"
        yarahub_reference_md5 = "7af59b0d647e83954bd4e7d5e0bcef32"
        date        = "2026-09-19"
        reference   = "Veille glissante-2026-09-19_08 / b9df548d.py"
        confidence  = "medium"
        yarahub_uuid = "357deea3-10ec-4d40-9324-735ed5b44798"
        yarahub_license = "CC0 1.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
        yarahub_reference_link = "https://github.com/Marjoriefort/yara-rules"
    strings:
        $a = "X19vYmZ1c2NhdGVkX2J5X18gPSAnaHR0cHM6Ly9weW9iZnVzY2F0ZS5jb20n" ascii
        $b = "CLIENT_CODE = base64.b64decode(" ascii
    condition:
        $a and $b
}
