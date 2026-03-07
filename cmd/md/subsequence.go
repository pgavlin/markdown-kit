package main

// subsequenceMatch returns true if all characters in needle appear in haystack
// in order (case-insensitive matching should be done by caller).
func subsequenceMatch(haystack, needle string) bool {
	hi := 0
	for _, c := range needle {
		found := false
		for hi < len(haystack) {
			r := rune(haystack[hi])
			hi++
			if r == c {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}
