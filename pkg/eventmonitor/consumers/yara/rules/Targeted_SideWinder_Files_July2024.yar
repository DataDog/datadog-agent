
rule Targeted_SideWinder_Files_July2024
{
meta:
    distribution = "TLP:WHITE"
    date = "2024-07-18"
    version = "1.0"
    last_modified = "2024-07-18"
    description = "Rule detecting maldoc used for targeting Egypt and Pakistan"
    author = "The BlackBerry Threat Research and Intelligence team"
    source = "https://blogs.blackberry.com/en/2024/07/sidewinder-targets-ports-and-maritime-facilities-in-the-mediterranean-sea?utm_content=&utm_medium=social"
    hash = "b72ac58d599e6e1080251b1ac45a521b33c08d7d129828a4e82a7095e9f93e53" 
    yarahub_reference_md5 = "9345d52abd5bab4320c1273eb2c90161"
    yarahub_uuid = "2cca6413-2eff-43d1-9e76-a334142e5792"
    yarahub_license = "CC0 1.0"
    yarahub_rule_matching_tlp = "TLP:WHITE"
    yarahub_rule_sharing_tlp = "TLP:WHITE"

strings:
   $a1 = "BA36646F1D81C20659D6"
   $a2 = {30 32 30 32 30 32 30 37 37 36 38 36 39 36 43 36 35 32
          30 32 38 36 35 32 45 36 31 37 34 34 35 36 45 36 34 32
          38 32 39 32 30 33 44 33 44 32 30 36 36 36 31 36 43 37
          33 36 35 32 39 32 30 37 42 30}

   $a3 = {4D 53 52 70 CE CF 03 0A 94 84 54 16 A4 DA 2A 65 E6 26
          A6 A7 EA 57 E8 82 64 F4 ED 00 50 4B 03 04 14 00 00 00 08}   

   $a4 = {62 54 4C B1 F0 B9 E6 E0 44 33 69 8E 85 BF B5 34 27 8B
          9B DC 5F 06 58 9C 01 1E 9C B8 0C 71 DF 23}

condition:
   filesize < 5000KB and any of ($a*)
}