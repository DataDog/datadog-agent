rule Nymaim
{
	meta:
		author = "Chaitanya"
		description = "Nymaim Loader"
		date = "2023-01-27"
		yarahub_reference_md5 = "0e56ecfe46a100ed5be6a7ea5a43432c"
		yarahub_license = "CC BY 4.0"
		yarahub_rule_matching_tlp = "TLP:WHITE"
		yarahub_rule_sharing_tlp = "TLP:WHITE"
		malpedia_family = "win.nymaim"
		yarahub_uuid = "5c578ac7-23cd-44d3-8bf9-e5c6db8cc13d"
    strings:
  $a = {80 79 ?? 00 74 ?? 0f 10 01 b8 10 00 00 00 0f 28 0d ?? ?? ?? ?? 66 0f ef c8 0f 11 09 0f 1f 40 00 80 34 08 2e 40 83 f8 ?? 72 ??}    
  $b = {80 79 0b 00 74 ?? 33 c0 80 34 08 2e 40 83 f8 0c 72 ??}
  $c = {80 79 0e 00 74 ?? 33 c0 80 34 08 2e 40 83 f8 0f 72 ??}
  condition:
		uint16(0) == 0x5A4D and all of them
	}