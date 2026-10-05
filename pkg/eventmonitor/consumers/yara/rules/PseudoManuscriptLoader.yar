rule PseudoManuscriptLoader{
  meta:
    author="@luc4m"
    date="2023-03-26"
    hash="e299ac0fd27e67160225400bdd27366f"
    tlp="CLEAR"
    yarahub_uuid = "b5613b13-99a6-4aa7-95a2-44ca02429965"
    yarahub_license =  "CC0 1.0"
    yarahub_rule_matching_tlp =  "TLP:WHITE"
    yarahub_rule_sharing_tlp =  "TLP:WHITE"
    yarahub_reference_md5= "53f9c2f2f1a755fc04130fd5e9fcaff4" 

  strings:
          $trait_0 = {57 8b ce 8b d8 e8 7b ff ff ff 8b 0b 89 08 33 ed 45 8b c5 5d 5b 5f 5e c2 04 00}
        $trait_1 = {57 8b ce 8b d8 e8 7b ff ff ff 8b 0b 89 08 33 ed 45 8b c5 5d 5b 5f 5e c2 04 00}
        $trait_2 = {ff 15 ?? ?? ?? ?? 85 c0 75 05 e8 6c f1 ff ff c2 04 00}
        $trait_3 = {ff 74 b5 ?? 8b 4d ?? e8 e7 fa ff ff 3b c7 59 75 07}
        $trait_4 = {b7 c0 0b c3 50 ff d6 53 89 45 ?? ff d6 89 45 ?? c7 45 ?? ?? ?? ?? ?? e9 9b fe ff ff}
        $trait_5 = {ff 74 b5 ?? 8b 4d ?? e8 e7 fa ff ff 3b c7 59 75 07}
        $trait_6 = {45 fc 56 8b c1 be 04 01 00 00 56 8d 8d ?? ?? ?? ?? 51 ff 70 ?? ff 15 ?? ?? ?? ?? 85 c0 74 56}
        $trait_7 = {8d 75 ?? 56 2b d1 52 50 e8 bd f9 ff ff 83 c4 0c 8d 85 ?? ?? ?? ?? 50 e8 97 fc ff ff eb 02}
        $trait_8 = {8d 45 ?? 50 8d 4d ?? 89 7d ?? e8 51 f5 ff ff 84 c0 74 08}
        $trait_9 = {ff 74 b5 ?? 8b 4d ?? e8 e7 fa ff ff 3b c7 59 75 07}


     $u1 = "https://%s.com/%d.html"

  condition:
     (uint16(0) == 0x5A4D) and filesize < 5MB and (1 of ($u*) and 5 of ($trait_*))


}
