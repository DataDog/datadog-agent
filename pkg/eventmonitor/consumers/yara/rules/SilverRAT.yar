rule SilverRAT {
    meta:
        author = "NDA0E"
        yarahub_author_twitter = "@NDA0E"
        date = "2024-10-28"
        description = "Detects SilverRAT"
        yarahub_uuid = "1ee8e50d-0059-4125-9409-d23305359383"
        yarahub_license = "CC BY 4.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
        yarahub_reference_md5 = "5ec3267acfd4ef36cbfb796016142892"
        
    strings:
        $Online = "You have a client online now" wide ascii
        $AutoRun = "SOFTWARE\\Microsoft\\Windows\\CurrentVersion\\Run" wide ascii
        $Keyloaggr = "Keyloaggr" ascii
        
    condition:
        uint16(0) == 0x5a4d and
        all of them
}