rule win_rust_hunt {
	meta:
		date = "2026-06-05"
		yarahub_uuid = "7440febf-38e0-4a9f-9e3e-19168c373168"
		yarahub_license = "CC0 1.0"
		yarahub_rule_matching_tlp = "TLP:WHITE"
		yarahub_rule_sharing_tlp = "TLP:WHITE"
		yarahub_reference_md5 = "44d88612fea8a8f36de82e1278abb02f"

	strings:
		$s1 = "rustc version" ascii

	condition:
		uint16(0) == 0x5a4d
		and filesize >= 1MB
		and all of them
}
