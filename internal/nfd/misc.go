/*
 * Copyright (c) 2025-2026. TxnLab Inc.
 * All Rights reserved.
 */

package nfd

import "strings"

// isValidNFDName checks if a name matches the pattern ^([a-z0-9]{1,27}\.){0,1}[a-z0-9]{1,27}\.algo$
// without using regexp
func isValidNFDName(name string) bool {
	// Check if name ends with ".algo"
	if len(name) < 6 || name[len(name)-5:] != ".algo" {
		return false
	}

	// Split by "." to check the parts
	parts := strings.Split(name[:len(name)-5], ".")

	// Should have either 1 or 2 parts before ".algo"
	if len(parts) > 2 || len(parts) == 0 {
		return false
	}

	// Check each part is 1-27 chars of a-z0-9
	for _, part := range parts {
		if len(part) < 1 || len(part) > 27 {
			return false
		}

		// Check each character is a-z or 0-9
		for _, c := range part {
			if !((c >= 'a' && c <= 'z') || (c >= '0' && c <= '9')) {
				return false
			}
		}
	}
	return true
}
