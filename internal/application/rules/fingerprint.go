package rules

import (
	"crypto/sha256"
	"encoding/hex"
	"sort"

	"github.com/kernul-io/cloudopt/internal/domain/types"
)

// Fingerprint returns a stable identifier for a finding instance.
func Fingerprint(ruleID, ruleVersion string, resourceIDs []types.ResourceID) string {
	ids := append([]types.ResourceID(nil), resourceIDs...)
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })

	h := sha256.New()
	_, _ = h.Write([]byte(ruleID))
	_, _ = h.Write([]byte{0})
	_, _ = h.Write([]byte(ruleVersion))
	_, _ = h.Write([]byte{0})
	for _, id := range ids {
		_, _ = h.Write([]byte(id))
		_, _ = h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil))
}
