rule BrainCipher {
    meta:
        author = "NDA0E"
	yarahub_author_twitter = "@NDA0E"
        date = "2024-10-17"
	description = "Detects BrainCipher Ransomware"
        yarahub_uuid = "b73e7c42-18de-4824-9537-6f9b36f7be71"
        yarahub_license = "CC0 1.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
        yarahub_reference_md5 = "71c109f3bf4da2fc0173b9bcff07e979"
    
    strings:
        $str0 = "Welcome to Brain Cipher Ransomware!" ascii
		
    condition:
        (uint16(0) == 0x5a4d or
	uint16(0) == 0x457f) and
	all of them
}