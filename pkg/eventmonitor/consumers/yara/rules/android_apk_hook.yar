rule android_apk_hook
      {
        meta:
          date = "2023-04-12"
          yarahub_reference_md5 = "bd00ea0d160476fc35403a954714db46"
          yarahub_uuid = "1cf204a2-7d44-4114-8c74-d8987a299626"
          yarahub_license = "CC BY 4.0"
          yarahub_rule_matching_tlp = "TLP:WHITE"
          yarahub_rule_sharing_tlp = "TLP:WHITE"
          malwaretype = "Hook - https://malpedia.caad.fkie.fraunhofer.de/details/apk.hook"
          filetype = "apk"

        strings:
          $aes_key = "1A1zP1eP5QGefi2DMPTfTL5SLmv7Divf"
          $hook_wa_cmd = "openwhatsapp"

        condition:
          all of them
      }
