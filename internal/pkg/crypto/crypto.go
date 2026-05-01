package crypto

import (
	"crypto/ed25519"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"os"

	"github.com/pkg/errors"
)

func LoadEd25519PrivateKey(path string) (ed25519.PrivateKey, error) {
	bytes, err := loadPEMBlock(path)
	if err != nil {
		return nil, err
	}
	key, err := x509.ParsePKCS8PrivateKey(bytes)
	if err != nil {
		return nil, errors.Wrap(err, "failed to parse PKCS#8 key")
	}
	ed25519Key, ok := key.(ed25519.PrivateKey)
	if !ok {
		return nil, fmt.Errorf("failed to convert %q to %q", "any", "ed25519.PrivateKey")
	}
	return ed25519Key, nil
}

func LoadEd25519PublicKey(path string) (ed25519.PublicKey, error) {
	bytes, err := loadPEMBlock(path)
	if err != nil {
		return nil, err
	}
	key, err := x509.ParsePKIXPublicKey(bytes)
	if err != nil {
		return nil, errors.Wrap(err, "failed to parse PKIX key")
	}
	ed25519Key, ok := key.(ed25519.PublicKey)
	if !ok {
		return nil, fmt.Errorf("failed to convert %q to %q", "any", "ed25519.PublicKey")
	}
	return ed25519Key, nil
}

func loadPEMBlock(path string) ([]byte, error) {
	bytes, err := os.ReadFile(path)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to read key file from %q", path)
	}
	block, _ := pem.Decode(bytes)
	if block == nil {
		return nil, errors.Wrap(err, "fail to decode PEM block")
	}
	return block.Bytes, nil
}
