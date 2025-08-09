package grading

import "unicode"

// normalize does simple casefolding and trims punctuation/extra spaces.
func normalize(s string) string {
	out := make([]rune, 0, len(s))
	space := false
	for _, r := range []rune(s) {
		switch {
		case unicode.IsSpace(r):
			space = true
		case unicode.IsPunct(r):
			// skip
		default:
			if space && len(out) > 0 {
				out = append(out, ' ')
			}
			space = false
			out = append(out, unicode.ToLower(r))
		}
	}
	return string(out)
}

// levenshtein computes edit distance (insertion, deletion, substitution cost 1).
func levenshtein(a, b string) int {
	ar := []rune(a)
	br := []rune(b)
	n, m := len(ar), len(br)
	if n == 0 {
		return m
	}
	if m == 0 {
		return n
	}
	dp := make([]int, m+1)
	for j := 0; j <= m; j++ {
		dp[j] = j
	}
	for i := 1; i <= n; i++ {
		prev := dp[0]
		dp[0] = i
		for j := 1; j <= m; j++ {
			tmp := dp[j]
			cost := 0
			if ar[i-1] != br[j-1] {
				cost = 1
			}
			ins := dp[j] + 1
			del := dp[j-1] + 1
			sub := prev + cost
			dp[j] = min3(ins, del, sub)
			prev = tmp
		}
	}
	return dp[m]
}

func min3(a, b, c int) int {
	if a < b {
		if a < c {
			return a
		}
		return c
	}
	if b < c {
		return b
	}
	return c
}
