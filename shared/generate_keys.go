package main

import (
    "crypto/rand"
    "crypto/rsa"
    "crypto/x509"
    "encoding/pem"
    "os"
)

func generateKeys() {
    // Generate private key
    privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
    if err != nil {
        panic(err)
    }

    // Save private key
    privateKeyFile, _ := os.Create("private_key.pem")
    defer privateKeyFile.Close()
    privateKeyBytes := x509.MarshalPKCS1PrivateKey(privateKey)
    pem.Encode(privateKeyFile, &pem.Block{Type: "RSA PRIVATE KEY", Bytes: privateKeyBytes})

    // Generate public key
    publicKey := privateKey.PublicKey
    publicKeyBytes, _ := x509.MarshalPKIXPublicKey(&publicKey)

    // Save public key
    publicKeyFile, _ := os.Create("public_key.pem")
    defer publicKeyFile.Close()
    pem.Encode(publicKeyFile, &pem.Block{Type: "RSA PUBLIC KEY", Bytes: publicKeyBytes})
}

func main() {
    generateKeys()
}
