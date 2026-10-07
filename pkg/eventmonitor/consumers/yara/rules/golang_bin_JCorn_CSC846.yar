rule golang_bin_JCorn_CSC846 {

	meta:
		description = "CSC-846 Golang detection ruleset"
		author = "Justin Cornwell"
		date = "2024-12-09"
		yarahub_reference_md5 = "c8820b30c0eddecf1f704cb853456f37"
		yarahub_license = "CC0 1.0"
		yarahub_uuid = "b684bc3e-c106-4636-b9b7-f0a90e0b45d7"
		yarahub_rule_matching_tlp = "TLP:WHITE"
		yarahub_rule_sharing_tlp = "TLP:WHITE"
	strings:
		$string_go_build = "Go build" ascii wide
		$string_runtime = "runtime" ascii wide

	condition:
		uint16(0) == 0x5a4d // MZ header
		and any of them

}
