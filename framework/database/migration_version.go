package database

import (
	"crypto/sha256"
	"encoding/hex"
	"regexp"
)

func computeChecksum(m Migration) string {
	if m.Checksum != "" {
		return m.Checksum
	}
	hash := sha256.Sum256([]byte(m.Name))
	return hex.EncodeToString(hash[:])
}

var versionRegex = regexp.MustCompile(`^(?:[vV])?(\d+(?:[\._]\d+)*)`)

func compareMigrationNames(a, b string) bool {
	matchA := versionRegex.FindStringSubmatch(a)
	matchB := versionRegex.FindStringSubmatch(b)

	if len(matchA) > 1 && len(matchB) > 1 {
		partsA := parseNumericParts(matchA[1])
		partsB := parseNumericParts(matchB[1])

		for i := 0; i < len(partsA) && i < len(partsB); i++ {
			if partsA[i] != partsB[i] {
				return partsA[i] < partsB[i]
			}
		}
		if len(partsA) != len(partsB) {
			return len(partsA) < len(partsB)
		}
	}
	return a < b
}

func parseNumericParts(s string) []int64 {
	var parts []int64
	var current int64
	hasDigit := false
	for _, r := range s {
		if r >= '0' && r <= '9' {
			current = current*10 + int64(r-'0')
			hasDigit = true
		} else if r == '.' || r == '_' {
			if hasDigit {
				parts = append(parts, current)
				current = 0
				hasDigit = false
			}
		}
	}
	if hasDigit {
		parts = append(parts, current)
	}
	return parts
}

func isVersionLTE(name, version string) bool {
	return !compareMigrationNames(version, name)
}

func findLatestExecutedMigration(executedMap map[string]MigrationRecord) string {
	var latest string
	for name := range executedMap {
		if latest == "" || compareMigrationNames(latest, name) {
			latest = name
		}
	}
	return latest
}
