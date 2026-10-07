rule SelfExtractingRAR {
  meta:
    author = "Xavier Mertens"
    description = "Detects an SFX archive with automatic script execution"
    date = "2023-05-17"
    yarahub_author_twitter = "@xme"
    yarahub_author_email = "xmertens@isc.sans.edu"
    yarahub_reference_link = "https://isc.sans.edu/diary/rss/29852"
    yarahub_uuid = "bcc4ceab-0249-43af-8d2a-8a04d5c65c70"
    yarahub_license =  "CC0 1.0"
    yarahub_rule_matching_tlp =  "TLP:WHITE"
    yarahub_rule_sharing_tlp =  "TLP:WHITE"
    yarahub_reference_md5= "7792250c87624329163817277531a5ef" 

    strings:
        $exeHeader = "MZ"
        $rarHeader = "Rar!" wide ascii
        $sfxSignature = "SFX" wide ascii
        $sfxSetup = "Setup=" wide ascii

    condition:
       $exeHeader at 0 and $rarHeader and $sfxSignature and $sfxSetup
}
