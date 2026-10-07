rule RANSOMWARE {
	meta:
		author = "ToroGuitar"
		Description = "This rule is meant to catch different types of ransomware."
		date = "2024-09-02"
		yarahub_reference_md5 = "b0fd45162c2219e14bdccab76f33946e"
		yarahub_uuid = "960a3047-a95b-44b2-acf3-307196a680c2"
		yarahub_license = "CC0 1.0"
		yarahub_rule_matching_tlp = "TLP:WHITE"
		yarahub_rule_sharing_tlp = "TLP:WHITE"
	strings:
		$a = ".onion"
		$b = "torproject.org"
		$c = "PartialFileCrypter"
		$d = "ransomware"
		$e = "infected"
		$f = "encrypted"
	condition:
		any of them
}