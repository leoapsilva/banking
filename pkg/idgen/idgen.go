// Package idgen generates compact alphanumeric identifiers that fit the C6
// constraint on external_reference_id (max 10 characters, [a-zA-Z0-9]).
package idgen

import (
	"crypto/sha1"
	"encoding/binary"
	"math/big"
)

const alphabet = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZ"

// ExternalReferenceID derives a deterministic 10-character base36 ID from
// arbitrary input parts (e.g. subscription ID + cycle date). Determinism is
// what makes it useful as an idempotency key: the same inputs always
// produce the same C6 reference, so retries don't create duplicate charges.
func ExternalReferenceID(parts ...string) string {
	h := sha1.New()
	for _, p := range parts {
		h.Write([]byte(p))
		h.Write([]byte{0})
	}
	sum := h.Sum(nil)

	// Use the first 8 bytes of the hash as a uint64 and encode it base36,
	// truncated/padded to exactly 10 characters.
	n := new(big.Int).SetUint64(binary.BigEndian.Uint64(sum[:8]))

	buf := make([]byte, 0, 13)
	base := big.NewInt(int64(len(alphabet)))
	zero := big.NewInt(0)
	mod := new(big.Int)
	for n.Cmp(zero) > 0 {
		n.DivMod(n, base, mod)
		buf = append(buf, alphabet[mod.Int64()])
	}
	for len(buf) < 10 {
		buf = append(buf, alphabet[0])
	}
	if len(buf) > 10 {
		buf = buf[:10]
	}
	return string(buf)
}
