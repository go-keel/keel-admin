// Package totp provides TOTP generation/verification and AES-GCM encryption
// for OTP secrets at rest. The AES key comes from config (env/dev), never the repo.
package totp

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"errors"
	"io"

	"github.com/pquerna/otp/totp"
)

// Step is the TOTP time-step size in seconds (RFC 6238 default, matches the
// pquerna/otp default Period used by GenerateSecret/Verify below).
const Step = 30

// ErrKeyLen is returned when the AES key is not 16/24/32 bytes.
var ErrKeyLen = errors.New("otp: AES key must be 16, 24 or 32 bytes")

// Cipher encrypts/decrypts OTP secrets with AES-GCM (random nonce prepended).
type Cipher struct {
	gcm cipher.AEAD
}

// NewCipher builds an AES-GCM cipher from the raw key.
func NewCipher(key []byte) (*Cipher, error) {
	switch len(key) {
	case 16, 24, 32:
	default:
		return nil, ErrKeyLen
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &Cipher{gcm: gcm}, nil
}

// Encrypt returns nonce||ciphertext.
func (c *Cipher) Encrypt(plaintext string) ([]byte, error) {
	nonce := make([]byte, c.gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}
	return c.gcm.Seal(nonce, nonce, []byte(plaintext), nil), nil
}

// Decrypt reverses Encrypt.
func (c *Cipher) Decrypt(enc []byte) (string, error) {
	ns := c.gcm.NonceSize()
	if len(enc) < ns {
		return "", errors.New("otp: ciphertext too short")
	}
	nonce, ct := enc[:ns], enc[ns:]
	pt, err := c.gcm.Open(nil, nonce, ct, nil)
	if err != nil {
		return "", err
	}
	return string(pt), nil
}

// GenerateSecret creates a new TOTP secret and its provisioning URI (otpauth://).
func GenerateSecret(issuer, account string) (secret, uri string, err error) {
	key, err := totp.Generate(totp.GenerateOpts{Issuer: issuer, AccountName: account})
	if err != nil {
		return "", "", err
	}
	return key.Secret(), key.URL(), nil
}

// Verify checks a TOTP code against the secret.
func Verify(secret, code string) bool {
	return totp.Validate(code, secret)
}
