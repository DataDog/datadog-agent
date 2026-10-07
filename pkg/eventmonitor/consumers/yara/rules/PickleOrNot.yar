private rule PickleFile {
	meta:
		author = "Eoin Wickens - Eoin@HiddenLayer.com"
		description = "Detects Pickle files"
		date = "2022-09-16"
	strings:
		$header_cos = "cos"
		$header_runpy = "runpy"
		$header_builtins = "builtins"
		$header_ccommands = "ccommands"
		$header_subprocess = "subprocess"
		$header_cposix = "cposix\nsystem"
		$header_c_builtin = "c__builtin__"
		$header_pickle_proto_1 = {80 01}
		$header_pickle_proto_2 = {80 02}
		$header_pickle_proto_3 = {80 03}
		$header_pickle_proto_4 = {80 04}
		$header_pickle_proto_5 = {80 05}
	condition:
		(
			for any of them: ($ at 0) or $header_runpy at 1 or
			$header_subprocess at 1
		)

}
private rule Pickle_LegacyPyTorch {
	meta:
		author = "Eoin Wickens - Eoin@HiddenLayer.com"
		description = "Detects Legacy PyTorch Pickle files"
		date = "2022-09-16"
	strings:
		$pytorch_legacy_magic_big = {19 50 a8 6a 20 f9 46 9c fc 6c}
		$pytorch_legacy_magic_little = {50 19 6a a8 f9 20 9c 46 6c fc}
	condition:
		uint8(0) == 0x80 and ($pytorch_legacy_magic_little or
			$pytorch_legacy_magic_big in (0..20))
}
rule PickleOrNot {
	meta:
		author = "Eoin Wickens - Eoin@HiddenLayer.com"
		description = "Detects Pickle files with dangerous c_builtins or non standard module imports. These are indicators of possible malicious intent"
		date = "2022-09-16"
		yarahub_reference_md5 = "81e132b5437b902040dd72fb0c62f7dc"
		yarahub_uuid = "dd704c93-e88d-4327-9e79-a20c7260bb3a"
		yarahub_license = "CC0 1.0"
		yarahub_rule_matching_tlp = "TLP:WHITE"
		yarahub_rule_sharing_tlp = "TLP:WHITE"
	condition:
		(PickleFile or Pickle_LegacyPyTorch)

}