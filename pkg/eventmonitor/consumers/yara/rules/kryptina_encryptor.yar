rule kryptina_encryptor
{
	meta:
		author = "Mario De Tore, Corelight Labs"
		description = "Matches on kryptina encryptor binary"
		date = "2024-04-06"
		yarahub_reference_md5 = "a06a6010b3383e3ff2f9f1c427427f06"
		yarahub_uuid = "69ea0a0d-c0b3-429a-a51f-6066752c9775"
		yarahub_license = "CC BY-NC-ND 4.0"
		yarahub_rule_matching_tlp = "TLP:WHITE"
		yarahub_rule_sharing_tlp = "TLP:WHITE"
	strings:
		$elf           = "\x7fELF"
		$rodata        = "\x00.rodata\x00"
		$cipher        = "EVP_CIPHER_CTX"
		$b64_string    = /\x00[-A-Za-z0-9+\/]{22,}={0,2}\x00/
		$poss_enc_note = /\x00[-A-Za-z0-9+\/]{1500,2500}={0,2}\x00/
		$openssh       = "OpenSSH"
	condition:
		$elf at 0 and
		$rodata and
		#b64_string >= 4 and
		all of ($cipher, $poss_enc_note) and not
		$openssh
}
