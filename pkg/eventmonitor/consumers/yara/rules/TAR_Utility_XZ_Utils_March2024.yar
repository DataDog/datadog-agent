
rule TAR_Utility_XZ_Utils_March2024
{
  meta:
    author = "Yashraj Solanki - Cyber Threat Intelligence Analyst at Bridewell"
    Sescription = "Detects Malicious Backdoor found in the XZ-Utils in Versions 5.6.0 and 5.6.1"
    Reference = "https://gist.github.com/thesamesam/223949d5a074ebc3dce9ee78baad9e27"
    File_Hash = "cf46bd09ff6164747f56e46d461f3375a1ff84803090bbbb15cd64651a83bd2e"
    Note = "this rule was only tested on the 5.6.0 version, will improve the rule later to cover 5.6.1"
    date = "2024-03-30"
    yarahub_author_twitter = "@RustyNoob619"
    yarahub_reference_md5 = "42a48cea544ce30b33135466cdca80d2"
    yarahub_uuid = "fb2b4b51-dc6e-4f81-a067-b62737b8900a"
    yarahub_license = "CC0 1.0"
    yarahub_rule_matching_tlp = "TLP:WHITE"
    yarahub_rule_sharing_tlp = "TLP:WHITE" 

  strings:
    $hex1 = {78 7a 2d 35 2e 36 2e 30 2f}  //xz-5.6.0/
    $hex2 = {78 7a 2d 35 2e 36 2e 31 2f}  //xz-5.6.1/
    $lib = "liblzma" fullword 
    $arch = "x86_64"fullword
    $script = "build-to-host.m4"
     
  condition:
    any of ($hex*) at 0 
    and $lib
    and $arch
    and $script 
}











