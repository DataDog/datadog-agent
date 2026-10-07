rule testlumma
{
	meta:
		date = "2024-12-11"
		yarahub_uuid = "23ff8a5a-4670-4f6c-b51e-f220cd229b9c"
		yarahub_license = "CC0 1.0"
		yarahub_rule_matching_tlp = "TLP:WHITE"
		yarahub_rule_sharing_tlp = "TLP:WHITE"
		yarahub_reference_md5 = "37363b24a0f1a339cf2e9a3dba0e12e2"

	strings:
		$string1 = "lumma" wide
        $string2 = "POST /login" ascii
        $string3 = "C2 server" ascii
        $string4 = "user-agent: lumma-agent" ascii
        $hash1 = { F1 9E 8A 7D 74 B8 9C 45 }

	condition:
		1 of ($string*) or $hash1
}