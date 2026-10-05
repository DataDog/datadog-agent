rule NitrogenRansomware
{
    meta:
      author = "AceS Cybersecurity"
      description="Detects if a file is NitrogenRansomware"
      date = "2025-08-03"
      yarahub_uuid = "1cc02d6f-7505-46b2-9297-3d510aee050b"
      yarahub_license = "CC0 1.0"
      yarahub_rule_matching_tlp = "TLP:WHITE"
      yarahub_rule_sharing_tlp = "TLP:WHITE"
      yarahub_reference_md5 = "5E31D74E575ED2931AC1B49892388A0E" 
    
    strings:
        $hex_string = { 4E  69  74  72  6F  67  65  6E} //start of ransom note Nitrogen

        $TOR_string = "http://nitrogenczslprh3xyw6lh5xyjvmsz7ciljoqxxknd7uymkfetfhgvqd.onion" //Tor negotiation URL
    
    condition:
        $hex_string and $TOR_string

}