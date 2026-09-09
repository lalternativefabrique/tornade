// Package application holds what the use cases share: minting a pair and
// sealing it.
package application

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"

	"github.com/lalternativefabrique/tornade/core/registry/domain"
	"github.com/lalternativefabrique/tornade/core/registry/infrastructure"
)

// Mint returns a fresh pair and its sealed form.
func Mint(cipher *infrastructure.Cipher) (domain.Keys, domain.Encrypted, error) {
	keys := domain.Keys{Signing: newKey(), App: newKey()}
	signing, err := cipher.Encrypt(keys.Signing)
	if err != nil {
		return domain.Keys{}, domain.Encrypted{}, err
	}
	app, err := cipher.Encrypt(keys.App)
	if err != nil {
		return domain.Keys{}, domain.Encrypted{}, err
	}
	return keys, domain.Encrypted{Signing: signing, App: app}, nil
}

func newKey() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic(fmt.Sprintf("registry: random source unavailable: %v", err))
	}
	return hex.EncodeToString(b)
}
