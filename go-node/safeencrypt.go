// safeencrypt.go (
package main

// FileEnvelope holds what we store at each node for a file backup
type FileEnvelope struct {
	Name             string `json:"name"`
	EncryptedFileB64 string `json:"encrypted_file_b64"` // nonce + ct base64
	EncryptedKeyB64  string `json:"encrypted_key_b64"`  // RSA-OAEP encrypted fileKey (base64)
	Created          int64  `json:"created_unix"`
	OwnerNodeID      string `json:"owner_node_id"`
}

// encryptFileForBackup: accepted inputs: file bytes, operatorPubPEM []byte (PEM-encoded RSA public key)
