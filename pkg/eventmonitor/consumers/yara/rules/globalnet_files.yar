rule globalnet_files
{
    meta:
        description = "Detect PE files compiled with PyInstaller with AntiDecompilation string. Observed in GlobalNet botnet campaign."
		reference = "https://twitter.com/vmovupd/status/1722548036839072017"
		author = "vmovupd"
		version = "1.0"
		date = "2024-01-28"
		yarahub_uuid = "e0280e2f-3fe8-4c11-b131-148d6b89cbde"
		yarahub_license = "CC0 1.0"
		yarahub_rule_matching_tlp = "TLP:WHITE"
		yarahub_rule_sharing_tlp = "TLP:WHITE"
		yarahub_reference_md5 = "96728cdb39ea05f8c8b1d80195a2914b"

    strings:
        $pyinst = {4D 45 49 0C 0B 0A 0B 0E}
		$antidecomp = "AntiDecompilation"
    condition:
        uint16(0) == 0x5A4D and $pyinst and $antidecomp
}