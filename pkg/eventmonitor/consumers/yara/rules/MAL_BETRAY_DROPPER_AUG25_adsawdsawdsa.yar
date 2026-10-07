/*
   YARA Rule
   Author: DeTraced Security
   Date: 2025-10-17
   Identifier: MAL_BETRAY_DROPPER_AUG25
   Reference: https://detraced.org/posts/infostealer-turned-ransomware
*/

rule MAL_BETRAY_DROPPER_AUG25_adsawdsawdsa {
   meta:
      description = "MAL_BETRAY_DROPPER_AUG25 - file adsawdsawdsa"
      author = "DeTraced Security"
      reference = "https://detraced.org/posts/infostealer-turned-ransomware"
      date = "2025-10-17"
      yarahub_author_twitter = "@DeTraced-Sec"
      yarahub_author_email = "detraced-sec@proton.me"
      yarahub_uuid = "f150198c-e5c0-4566-a961-a3374fcf8074"
      yarahub_license = "CC BY 4.0"
      yarahub_rule_matching_tlp = "TLP:WHITE"
      yarahub_rule_sharing_tlp = "TLP:WHITE"
      yarahub_reference_md5 = "930389e2bf6556761805f3b209841c99"
      hash1 = "7ccf335183ef00d67d0c0b836d1de65a38c280c81592e112e0faeaedce2ff9f1"
   strings:
      $s1 = "powershell -c \"$e='wMeDh8bN1Zn28Obx8/Hs5erv5v/nzNTNz8zCx9CYytTRg4TL19fT0JmMjMTK18vWwY3AzM6MwcbX0cLa2szW0cXMxtCM4O7nxtvGwNCM0cbP" ascii
      $s2 = "powershell -c \"$e='wMeDh8bN1Zn28Obx8/Hs5erv5v/nzNTNz8zCx9CYytTRg4TL19fT0JmMjMTK18vWwY3AzM6MwcbX0cLa2szW0cXMxtCM4O7nxtvGwNCM0cbP" ascii
      $s3 = "=[char]($_ -bxor 163)};iex $s\"" fullword ascii
      $s4 = "sLQxtCMx8zUzc/MwseMwcbX0cLajMXM0dfNytfGjcHC14SDjsyDwo3BwteYg9DXwtHXg8KNwcLXg470wsrX';$s='';[Convert]::FromBase64String($e)|%{$s+" ascii
   condition:
      uint16(0) == 0x6f70 and filesize < 1KB and
      all of them
}