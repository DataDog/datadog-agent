rule koi_loader {
    meta:
        author = "@luc4m"
        date = "2023-03-26"
        link = "https://medium.com/@lcam/updates-from-the-maas-new-threats-delivered-through-nullmixer-d45defc260d1"
        hash_md5 = "9725ec075e92e25ea5b6e99c35c7aa74"
        tlp = "WHITE"
	yarahub_uuid = "d0872aaf-306d-4068-b246-86d12a6e56f7"
        yarahub_license =  "CC0 1.0"
        yarahub_rule_matching_tlp =  "TLP:WHITE"
        yarahub_rule_sharing_tlp =  "TLP:WHITE"
        yarahub_reference_md5= "9725ec075e92e25ea5b6e99c35c7aa74" 
    strings:

 $tm_0 = /debug[0-9]{1,3}\.ps1/i wide
 $tm_1 = "First stage size: {0}" wide
 $tm_2 = "Second stage size: {0}" wide
 $tm_3 = "Telegram Desktop\\tdata" wide
 $tm_4 = "Executed " wide
 $tm_5 = " or downloading " wide
 $tm_6 = "LDR" wide

 $curve_0 = "key must be 32 bytes long (but was {0} bytes long)" wide
 $curve_1 = "rawKey must be 32 bytes long (but was {0} bytes long)" wide
 $curve_2 = "rawKey" wide 
 $curve_3 = "key" wide 

    condition:
         (5 of ($tm_*)) and (1 of ($curve_*))
}

