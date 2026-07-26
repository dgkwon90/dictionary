package knowledge

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
)

// maxInlineKeyLength is how much text we are willing to store verbatim as an index
// key before switching to a digest.
const maxInlineKeyLength = 120

// hashedKeyPrefix marks a key that is a digest rather than the text itself, so the
// two can never be confused when reading rows by hand.
const hashedKeyPrefix = "h:"

// NormalizeKey derives the identity of a learning item from its text. It is the
// server's answer to "are these two the same thing to learn?", and it must stay
// stable: it is half of the UNIQUE constraint on knowledge_items.
//
// The AI also returns a normalized_key, but we do not trust it as an identity.
// It is generated text, so nothing stops it from truncating a long sentence — and
// two different sentences that truncate to the same prefix would merge into one
// row, losing one of them with no error anywhere. Deriving the key here makes that
// impossible.
//
// Long text is hashed rather than stored inline so a whole paragraph does not end up
// inside an index key. Collisions are not a practical concern with SHA-256.
func NormalizeKey(text string) string {
	normalized := strings.ToLower(strings.Join(strings.Fields(text), " "))
	if normalized == "" {
		return ""
	}
	if len(normalized) <= maxInlineKeyLength {
		return normalized
	}
	sum := sha256.Sum256([]byte(normalized))
	return hashedKeyPrefix + hex.EncodeToString(sum[:])
}
