// +build !nacl,!arm

package stingle

import (
	"encoding/base64"
	"encoding/json"
	"errors"

	"github.com/jamesruan/sodium"
)

// MakeSecretKey returns a new SecretKey.
func MakeSecretKey() SecretKey {
	kp := sodium.MakeBoxKP()
	return SecretKey(kp.SecretKey)
}

type SecretKey sodium.BoxSecretKey
type PublicKey sodium.BoxPublicKey

func PublicKeyFromBytes(b []byte) PublicKey {
	return PublicKey(sodium.BoxPublicKey{sodium.Bytes(b)})
}

func (k PublicKey) ToBytes() []byte {
	return []byte(sodium.BoxPublicKey(k).Bytes)
}

func (k SecretKey) Empty() bool {
	return sodium.BoxSecretKey(k).Bytes == nil
}

func (k SecretKey) PublicKey() PublicKey {
	return PublicKey(sodium.BoxSecretKey(k).PublicKey())
}

func (k *SecretKey) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}
	b, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		return err
	}
	k.Bytes = sodium.Bytes(b)
	return nil
}

func (k SecretKey) MarshalJSON() ([]byte, error) {
	return json.Marshal(base64.RawURLEncoding.EncodeToString(k.Bytes))
}

func (k *PublicKey) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}
	b, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		return err
	}
	k.Bytes = sodium.Bytes(b)
	return nil
}

func (k PublicKey) MarshalJSON() ([]byte, error) {
	return json.Marshal(base64.RawURLEncoding.EncodeToString(k.Bytes))
}

// MakeSignSecretKey returns a new SignSecretKey.
func MakeSignSecretKey() SignSecretKey {
	kp := sodium.MakeSignKP()
	return SignSecretKey(kp.SecretKey)
}

type SignSecretKey sodium.SignSecretKey
type SignPublicKey sodium.SignPublicKey

func (k SignSecretKey) Empty() bool {
	return sodium.SignSecretKey(k).Bytes == nil
}

func (k *SignSecretKey) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}
	b, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		return err
	}
	k.Bytes = sodium.Bytes(b)
	return nil
}

func (k SignSecretKey) MarshalJSON() ([]byte, error) {
	return json.Marshal(base64.RawURLEncoding.EncodeToString(k.Bytes))
}

func (k SignSecretKey) PublicKey() SignPublicKey {
	return SignPublicKey(sodium.SignSecretKey(k).PublicKey())
}

func (k SignSecretKey) Sign(msg []byte) []byte {
	return sodium.Bytes(msg).SignDetached(sodium.SignSecretKey(k)).Bytes
}

// EncryptMessage encrypts a message using Authenticated Public Key Encryption.
// https://pkg.go.dev/github.com/jamesruan/sodium#hdr-Authenticated_Public_Key_Encryption
func EncryptMessage(msg []byte, pk PublicKey, sk SecretKey) string {
	var n sodium.BoxNonce
	sodium.Randomize(&n)

	m := []byte(n.Bytes)
	m = append(m, []byte(sodium.Bytes(msg).Box(n, sodium.BoxPublicKey(pk), sodium.BoxSecretKey(sk)))...)
	return base64.StdEncoding.EncodeToString(m)
}

// DecryptMessage decrypts a message using Authenticated Public Key Encryption.
// https://pkg.go.dev/github.com/jamesruan/sodium#hdr-Authenticated_Public_Key_Encryption
func DecryptMessage(msg string, pk PublicKey, sk SecretKey) ([]byte, error) {
	b, err := base64.StdEncoding.DecodeString(msg)
	if err != nil {
		return nil, err
	}
	var n sodium.BoxNonce
	if len(b) < n.Size() {
		return nil, errors.New("msg too short")
	}
	n.Bytes = make([]byte, n.Size())
	copy(n.Bytes, b[:n.Size()])
	b = b[n.Size():]
	m, err := sodium.Bytes(b).BoxOpen(n, sodium.BoxPublicKey(pk), sodium.BoxSecretKey(sk))
	if err != nil {
		return nil, err
	}
	return []byte(m), nil
}

// SealBox encrypts a message using Anonymous Public Key Encryption.
// https://pkg.go.dev/github.com/jamesruan/sodium#hdr-Anonymous_Public_Key_Encryption
func SealBox(msg []byte, pk PublicKey) string {
	b := sodium.Bytes(msg)
	return base64.StdEncoding.EncodeToString([]byte(b.SealedBox(sodium.BoxPublicKey(pk))))
}

// SealBoxOpen decrypts a message encrypted by SealBox.
func SealBoxOpen(msg string, sk SecretKey) ([]byte, error) {
	b, err := base64.StdEncoding.DecodeString(msg)
	if err != nil {
		return nil, err
	}
	kp := sodium.BoxKP{sodium.BoxPublicKey(sk.PublicKey()), sodium.BoxSecretKey(sk)}
	d, err := sodium.Bytes(b).SealedBoxOpen(kp)
	if err != nil {
		return nil, err
	}
	return []byte(d), nil
}