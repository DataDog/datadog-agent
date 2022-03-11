// Adapted from codeql-go's experimental CWE-327 Weak Key Algorithm examples

package main

import (
	"crypto/aes"
	"crypto/rsa"
	"crypto/des"
	"crypto/md5"
	"crypto/rc4"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/tls"
	"golang.org/x/crypto/bcrypt"
	"golang.org/x/crypto/hkdf"
)

func main() {
	foo := "bar"

    // DETECTED - Approved Encryption Callee
	test.Aes128(foo)

	// DETECTED - Approved Encryption Package
	rsa.GenerateKey(foo, 128)

	// DETECTED - Approved Hash Callee
	test.Sha256(foo)

	// DETECTED - Approved Hash Package
	sha256.Sum256(foo)

	// DETECTED - Approved Password Callee
	test.Bcrypt(foo)

	// DETECTED - Approved Password Package
	bcrypt.GenerateFromPassword(foo, 11)

    // DETECTED - Disallowed Encryption Callee
	test.Des(foo)

	// DETECTED - Disallowed Encryption Package
	des.NewCipher(foo)

	// DETECTED - Disallowed Hash Callee
	test.Md5(foo)

	// DETECTED - Disallowed Hash Package
	md5.Sum(foo)

	// DETECTED - Disallowed Password Callee
    test.Hkdf(foo)

	// DETECTED - Disallowed Password Package
    hkdf.New(foo, foo, foo, foo)

	// DETECTED - Misc Flags Callee
    test.tls(foo)

	// DETECTED - Misc Flags Package
	tls.X509KeyPair(foo, foo)

	// NOT DETECTED - crypto as a parameter
    test.doingsomething("AES", "MD5", "SHA256")

}

