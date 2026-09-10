// Package application holds what the use cases share: minting a key and
// sealing it.
package application

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"

	"github.com/lalternativefabrique/tornade/core/registry/infrastructure"
)

// Mint returns a fresh key and its sealed form.
func Mint(cipher *infrastructure.Cipher) (string, []byte, error) {
	key := newKey()
	sealed, err := cipher.Encrypt(key)
	if err != nil {
		return "", nil, err
	}
	return key, sealed, nil
}

func newKey() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic(fmt.Sprintf("registry: random source unavailable: %v", err))
	}
	return hex.EncodeToString(b)
}
