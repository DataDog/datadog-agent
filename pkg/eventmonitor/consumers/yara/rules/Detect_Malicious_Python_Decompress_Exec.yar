rule Detect_Malicious_Python_Decompress_Exec {
    meta:
        description = "Detects malicious Python scripts with obfuscated zlib decompression and execution logic"
        author = "Sn0wFr0$t"
        reference = "Custom rule for obfuscated Python script detection"
        severity = "high"
		date = "2024-11-16"
		yarahub_uuid = "30413a55-c9cd-4b51-8944-1aec8eb95e66"
		yarahub_license = "CC0 1.0"
		yarahub_rule_matching_tlp = "TLP:WHITE"
		yarahub_rule_sharing_tlp = "TLP:WHITE"
		yarahub_reference_md5 = "b4916289881a8d13ad5230738bad3a6a"

    strings:
        $obfuscated_code = "_ = lambda __ : __import__('zlib').decompress(__import__('base64').b64decode(__[::-1]));exec((_)("

    condition:
        $obfuscated_code
}