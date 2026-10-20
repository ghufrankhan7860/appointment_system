package utils

import "strings"

// NormalizeValues trims and lowercases each value and drops empty values and
// duplicates, keeping the first occurrence order.
func NormalizeValues(values []string) []string {
	result := make([]string, 0, len(values))
	seen := make(map[string]bool, len(values))
	for _, value := range values {
		value = strings.ToLower(strings.TrimSpace(value))
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		result = append(result, value)
	}

	return result
}

// EscapeLike escapes the LIKE wildcards in value so user text matches literally.
func EscapeLike(value string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(value)
}
