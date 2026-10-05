rule mal_italianpasta_pastaloader
{
	meta:
		version = "1.0"
		author = "FatzQatz"
		description = "Detect PastaLoader, a loader used in ItalianPasta campaign targeting Travel Sector"
		date = "2025-01-31"
		last_modified = "2025-01-31"
		yarahub_reference_md5 = "93503d920e2d06748a4fab3134171625"
		yarahub_uuid = "607d7ca3-2469-44d5-b816-10b4a08fd3ed"
		yarahub_license = "CC BY 4.0"
		yarahub_rule_matching_tlp = "TLP:WHITE"
		yarahub_rule_sharing_tlp = "TLP:WHITE"
		falsepositives= "Unknown"
    strings:
        $decrypt_shell = { 01 10 8D 40 04 83 E9 01 } // Addition with the value stored in EAX
        $resolve_API_hash = { 0F B6 C9 03 CA 03 4C 24 08 03 C1 46 }
        $copy_shell = { 8B 45 08 8B 4D 0C 8A 09 88 08 8B 45 08 40 89 45 08 8B 45 0C 40 89 45 0C }
    condition:
        uint16(0) == 0x5A4D
        and all of them
}