/*
 * Copyright (c) 2025-2026. TxnLab Inc.
 * All Rights reserved.
 */

package nfd

import "testing"

func TestIsValidNFDName(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected bool
	}{
		{"valid single part", "name.algo", true},
		{"valid two parts", "name.sub.algo", true},
		{"invalid too many parts", "name.sub.extra.algo", false},
		{"invalid no extension", "name", false},
		{"invalid wrong extension", "name.txt", false},
		{"invalid empty string", "", false},
		{"invalid only extension", ".algo", false},
		{"invalid special characters", "name@!.algo", false},
		{"invalid uppercase letters", "NaMe.algo", false},
		{"invalid part too short", ".algo", false},
		{"invalid part too long", "averylongpartthatexceedstwentyseven.algo", false},
		{"valid max length single part", "apartthatexactlytwentyseven.algo", true},
		{"valid max length two parts", "apartthatexactlytwentyseven.apartthatexactlytwentyseven.algo", true},
		{"invalid too long two parts", "averylongpartthatexceedstwentyseven.averylongpartthatexceedstwentyseven.algo", false},
		{"valid alphanumeric", "name123.sub456.algo", true},
		{"invalid trailing dot", "name.sub..algo", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isValidNFDName(tt.input)
			if result != tt.expected {
				t.Errorf("input: %q, expected: %v, got: %v", tt.input, tt.expected, result)
			}
		})
	}
}
