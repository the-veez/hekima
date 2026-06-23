package tokenizer

// Tokenizer counts the number of tokens in a text string.
// Implementations must be safe for concurrent use.
// Count is intentionally non-fallible: token count is an annotation,
// not a correctness-critical value. On any internal error, implementations
// fall back to a best-effort estimate rather than propagating an error.
type Tokenizer interface {
	Count(text string) int
}

// WhitespaceTokenizer is the zero-dependency default tokenizer.
// It estimates token count as word count × 1.3, which approximates
// BPE token counts for English and Swahili prose without requiring
// any external vocabulary file. Suitable for low-connectivity deployments.
type WhitespaceTokenizer struct{}

// Count returns an estimated token count for text.
// The estimate is: number of whitespace-separated words × 1.3, rounded up.
func (w WhitespaceTokenizer) Count(text string) int {
	if text == "" {
		return 0
	}

	words := 0
	inWord := false
	for _, r := range text {
		isSpace := r == ' ' || r == '\t' || r == '\n' || r == '\r'
		if !isSpace && !inWord {
			words++
			inWord = true
		} else if isSpace {
			inWord = false
		}
	}

	// ×1.3 rounded up: multiply by 13, divide by 10, add 1 if remainder exists.
	tokens := (words * 13) / 10
	if (words*13)%10 != 0 {
		tokens++
	}
	return tokens
}
