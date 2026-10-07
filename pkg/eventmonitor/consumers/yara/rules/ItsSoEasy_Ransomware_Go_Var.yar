rule ItsSoEasy_Ransomware_Go_Var {
    meta:
		description = "Detect ItsSoEasy Ransomware (Itssoeasy-A Go.Var)"
		author = "bstnbuck"
		date = "2023-11-02"
        yarahub_author_twitter = "@bstnbuck"
        yarahub_reference_link = "https://github.com/bstnbuck/ItsSoEasy"
        yarahub_uuid = "e1115417-d183-472e-8156-6e3f070ef2e6"
        yarahub_license = "CC0 1.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
        yarahub_reference_md5 = "b4b6c316ba4285d42649026d38f9ea43"
    
	strings:
		$typ1 = "itssoeasy" nocase
		$typ1_wide = "itssoeasy" nocase wide
		$typ2 = "itssoeasy" base64
		$typ3 = "ItsSoEasy" base64

		// C2 communication message strings
		// well this sucks, ha!
		$c2m1 = "d2VsbCB0aGlzIHN1Y2tzLCBoYSE="  
		// has this idiot payed the ransom?               
		$c2m2 = "aGFzIHRoaXMgaWRpb3QgcGF5ZWQgdGhlIHJhbnNvbT8="
		// oh, you're good!
		$c2m3 = "b2gsIHlvdSdyZSBnb29kIQ=="          
		// money, money, money!           
		$c2m4 = "bW9uZXksIG1vbmV5LCBtb25leSE="        
		// i need this to fuck you up!         
		$c2m5 = "aSBuZWVkIHRoaXMgdG8gZnVjayB5b3UgdXAh"   
		// --KEY-PROCEDURE--      
		$c2m6 = "LS1LRVktUFJPQ0VEVVJFLS0="                     
		
		// Base64 encoded message strings
		// Decrypt files now?
		$tkmsgMsg = "RGVjcnlwdCBmaWxlcyBub3c/"
		// Whoops your personal data was encrypted! Read the index.html on the Desktop how to decrypt it
		$tkmsg1Msg = "V2hvb3BzIHlvdXIgcGVyc29uYWwgZGF0YSB3YXMgZW5jcnlwdGVkISBSZWFkIHRoZSBpbmRleC5odG1sIG9uIHRoZSBEZXNrdG9wIGhvdyB0byBkZWNyeXB0IGl0"
		// Now your data is lost
		$tkmsg2Msg = "Tm93IHlvdXIgZGF0YSBpcyBsb3N0" 
		// It was as easy as I said, ha?
		$tkmsg3Msg = "SXQgd2FzIGFzIGVhc3kgYXMgSSBzYWlkLCBoYT8=" 

		// file names and typical ransom filetype
		$fileFiles = "L2lmX3lvdV9jaGFuZ2VfdGhpc19maWxlX3lvdXJfZGF0YV9pc19sb3N0" // /if_you_change_this_file_your_data_is_lost
		// /identifier
		$fileident = "L2lkZW50aWZpZXI=" 
		// .itssoeasy                                        
		$filetype = "Lml0c3NvZWFzeQ==" 
		$fileransom = "itssoeasy.html"

		// CMD print messages
		$cmd1 = "Welcome to the Google connector!\nPlease wait while the installer runs..."
		$cmd2 = "Do not destroy the current process, otherwise your data will be irreversibly encrypted."
		$cmd3 = "Please use the instructions in the .html file on your Desktop or your Home-Directory to decrypt your data"
		$cmd4 = "If you payed, this window will automatically check and decrypt your data."
		$cmd5 = "Wow! You're good. Now i will recover your files!\n => Do not kill this process, otherwise your data is lost!"
		$cmd6 = "Your files has been decrypted!\nThank you and Goodbye."

    condition:
        any of ($typ*) and all of ($c2*, $tkmsg*, $file*, $cmd*) and (filesize > 2500KB and filesize < 6MB)
}