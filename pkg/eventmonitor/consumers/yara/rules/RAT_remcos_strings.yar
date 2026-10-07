rule RAT_remcos_strings {
   meta:
      description = "This rule detects the remcos through your specific strings."
      author = "0x0d4y"
      reference = "Internal Research"
      date = "2024-06-26"
      score = 100
      yarahub_uuid = "33e2fa3c-67d1-43dd-9d62-50efa02aa9b2"
      yarahub_reference_md5 = "69c95c878aa933bc20078fab85281fd5"
      yarahub_license = "CC BY 4.0"
      yarahub_rule_matching_tlp = "TLP:WHITE"
      yarahub_rule_sharing_tlp = "TLP:WHITE"
      malpedia_family = "win.remcos"
   strings:
      $string1 = "Remcos_Mutex_Inj" wide ascii 
      $string2 = "autopswdata" wide ascii
      $string3 = "startcamcap" wide ascii
      $string4 = "1.7 Pro" wide ascii
      $string5 = "\\AppData\\Roaming\\Mozilla\\Firefox\\Profiles\\" wide ascii
      $string6 = "Connected to C&C!\n" wide ascii
      $string7 = "\\AppData\\Local\\Google\\Chrome\\User Data\\Default\\Cookies" wide ascii
      $string8 = "initfun" wide ascii
      $string9 = "keyinput" wide ascii
      $string10 = "deletefile" wide ascii
      $string11 = "getcamlib" wide ascii
      $string12 = "screenshotdata" wide ascii
      $string13 = "prockill" wide ascii
      $string14 = "proclist" wide ascii
      $string15 = "upload" wide ascii
      $string16 = "download" wide ascii
      $string17 = "getdrives" wide ascii
      $string18 = "uploadprogress" wide ascii
      $string19 = "remscriptsuccess" wide ascii
      $string20 = "Breaking-Security" wide ascii
      $string21 = "\\AppData\\Local\\Google\\Chrome\\User Data\\Default\\Login Data" wide ascii
   condition:
      uint16(0) == 0x5a4d and
      15 of ($string*)
}