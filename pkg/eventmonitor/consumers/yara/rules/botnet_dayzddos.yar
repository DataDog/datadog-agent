rule botnet_dayzddos {
    meta:
        author = "NDA0E"
        date = "2024-05-11"
        description = "dayzddos botnet"
        yarahub_uuid = "fa9ae8db-5393-4554-9fec-da031bf6cb23"
        yarahub_license = "CC0 1.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
        yarahub_reference_md5 = "7ac9673e951d038c2c10c230393b6f0a"
    strings:
        $dayzddos = "dayzddos" ascii
    condition:
        uint16(0) == 0x457f and all of them
}