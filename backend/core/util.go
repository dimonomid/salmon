package core

import "fmt"

// CheckID checks if the provided string is a valid ID (it must only contain
// lowercase English letters, numbers, hyphens and underscores, and it must
// start with a letter), and returns nil if it is, or an error explaining the
// problem.
func CheckID(id string) error {
	if len(id) == 0 {
		return fmt.Errorf("empty string is not a valid id")
	}

	for i, r := range id {
		isLetter := r >= 'a' && r <= 'z'

		// If it's a letter, we're done.
		if isLetter {
			continue
		}

		if i == 0 {
			return fmt.Errorf("%q is not a valid id, it must start with a lowercased letter")
		}

		if r >= '0' && r <= '9' || r == '-' || r == '_' {
			continue
		}

		return fmt.Errorf("%q is not a valid id, it must only contain lowercase English letters, numbers, hyphens and underscores")
	}

	return nil
}
