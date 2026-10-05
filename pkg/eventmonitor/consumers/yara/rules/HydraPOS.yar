rule HydraPOS {
    meta:
        author = "NDA0E"
        yarahub_author_twitter = "@NDA0E"
	date = "2024-10-21"
        description = "Detects HydraPOS"
	yarahub_uuid = "98daac26-ce94-4fd5-bbdc-8d9068102501"
        yarahub_license = "CC BY 4.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
        yarahub_reference_md5 = "d942b8a239f9e76f5813ce43cf3ec8eb"
    
    strings:
        $str01 = "adicional.txt" wide ascii
        $str02 = "{0:dd_mm_yyyy hhh}.txt" wide ascii
        $str03 = "-connect" wide ascii
        $str04 = "-controlapp" wide ascii
        
	$regex = "(?<Trilha>[3-6][0-9]\\d{14}=(?<Ano>[0-9]\\d{1})(?<Mes>[0-9]\\d{1})(?<Codigo>[0-9]\\d{2})[0-9]\\d{3,12}|37\\d{13}=(?<Ano2>[0-9]\\d{1})(?<Mes2>[0-9]\\d{1})(?<Codigo2>[0-9]\\d{2})[0-9]\\d{3,12})" wide ascii
        
    condition:
        (3 of ($str*) or ($regex)) and
        uint16(0) == 0x5a4d
}