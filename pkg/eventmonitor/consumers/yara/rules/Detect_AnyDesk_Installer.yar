rule Detect_AnyDesk_Installer {
    meta:
        description = "Detects malicious Python scripts that install AnyDesk"
        author = "Sn0wFr0$t"
        reference = "Custom rule for detecting InvisibleFerret AnyDesk-related malicious scripts"
        severity = "high"
		date = "2024-11-16"
		yarahub_uuid = "8fdfae1c-6926-4e55-b977-1e98098431f5"
		yarahub_license = "CC0 1.0"
		yarahub_rule_matching_tlp = "TLP:WHITE"
		yarahub_rule_sharing_tlp = "TLP:WHITE"
		yarahub_reference_md5 = "3e09edd4d8f998330c7c99062df1e5d7"

    strings:
        $pwd_hash = "ad.anynet.pwd_hash=" nocase
        $pwd_salt = "ad.anynet.pwd_salt=" nocase
        $token_salt = "ad.anynet.token_salt=" nocase
        $pip_install = "sys.executable,'-m','pip','install','psutil'" nocase

    condition:
        all of them
}