rule OneNote_EmbeddedFiles_NoPictures
{
    meta:
        author = "Nicholas Dhaeyer - @DhaeyerWolf"
        date_created = "2023-02-14 - <3"
        date_last_modified = "2023-02-17"
        description = "OneNote files that contain embedded files that are not pictures."
        reference = "https://blog.didierstevens.com/2023/01/22/analyzing-malicious-onenote-documents/"
		yarahub_uuid = "d0c4f0e6-adbe-4953-a2df-91427a561e97"
		date = "2023-02-14"
		yarahub_license = "CC0 1.0"
		yarahub_rule_matching_tlp = "TLP:WHITE"
		yarahub_rule_sharing_tlp = "TLP:WHITE"
		yarahub_reference_md5 = "52486a446dd4fc5842a47b57d3febec7"

    strings:
        $EmbeddedFileGUID =  { E7 16 E3 BD 65 26 11 45 A4 C4 8D 4D 0B 7A 9E AC }
        $PNG = { E7 16 E3 BD 65 26 11 45 A4 C4 8D 4D 0B 7A 9E AC ?? ?? ?? ?? ?? ?? ?? ?? 00 00 00 00 00 00 00 00 00 00 00 00 89 50 4E 47 0D 0A 1A 0A }
        $JPG = { E7 16 E3 BD 65 26 11 45 A4 C4 8D 4D 0B 7A 9E AC ?? ?? ?? ?? ?? ?? ?? ?? 00 00 00 00 00 00 00 00 00 00 00 00 FF D8 FF }
        $JPG20001 = { E7 16 E3 BD 65 26 11 45 A4 C4 8D 4D 0B 7A 9E AC ?? ?? ?? ?? ?? ?? ?? ?? 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 0C 6A 50 20 20 0D 0A 87 0A }
        $JPG20002 = { E7 16 E3 BD 65 26 11 45 A4 C4 8D 4D 0B 7A 9E AC ?? ?? ?? ?? ?? ?? ?? ?? 00 00 00 00 00 00 00 00 00 00 00 00 FF 4F FF 51 }
        $BMP = { E7 16 E3 BD 65 26 11 45 A4 C4 8D 4D 0B 7A 9E AC ?? ?? ?? ?? ?? ?? ?? ?? 00 00 00 00 00 00 00 00 00 00 00 00 42 4D }
        $GIF = { E7 16 E3 BD 65 26 11 45 A4 C4 8D 4D 0B 7A 9E AC ?? ?? ?? ?? ?? ?? ?? ?? 00 00 00 00 00 00 00 00 00 00 00 00 47 49 46 }

    condition:
        $EmbeddedFileGUID and (#EmbeddedFileGUID > #PNG + #JPG + #JPG20001 + #JPG20002 + #BMP + #GIF)
}
