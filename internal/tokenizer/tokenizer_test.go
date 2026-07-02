package tokenizer

import (
	"errors"
	"testing"
)

func TestWhitespaceTokenizer_Count(t *testing.T) {
	tests := []struct {
		name string
		text string
		want int
	}{
		{name: "empty string", text: "", want: 0},
		{name: "single word", text: "hello", want: 2},
		{name: "two words", text: "hello world", want: 3},
		{name: "five words", text: "the quick brown fox jumps", want: 7},
		{name: "whitespace only", text: "   \n\t  ", want: 0},
		{
			name: "punctuation and multiple spaces",
			text: "Hello,   world!  How are you?",
			want: 7, // 5 words ("Hello," "world!" "How" "are" "you?") * 1.3 + 0.5, truncated = 7
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := WhitespaceTokenizer{}.Count(tt.text)
			if got != tt.want {
				t.Errorf("WhitespaceTokenizer{}.Count(%q) = %d, want %d", tt.text, got, tt.want)
			}
		})
	}
}

// TestWhitespaceTokenizer_NeverZeroForNonEmptyInput verifies the
// documented guarantee: any non-empty, non-whitespace-only text
// reports at least one token. A zero count for real content would be
// a worse failure mode for a downstream pipeline than a rough
// overestimate, since "zero tokens" could be misread as "empty chunk".
func TestWhitespaceTokenizer_NeverZeroForNonEmptyInput(t *testing.T) {
	inputs := []string{"a", "I", "x", "."}
	for _, text := range inputs {
		got := WhitespaceTokenizer{}.Count(text)
		if got == 0 {
			t.Errorf("WhitespaceTokenizer{}.Count(%q) = 0, want >= 1 for non-empty input", text)
		}
	}
}

// fakeBPECodec lets us test BPETokenizer's fallback behaviour without
// depending on the real tiktoken-go vocabulary at test time.
type fakeBPECodec struct {
	count int
	err   error
}

func (f fakeBPECodec) Count(input string) (int, error) {
	return f.count, f.err
}

func TestBPETokenizer_Count_DelegatesToCodec(t *testing.T) {
	bt := &BPETokenizer{codec: fakeBPECodec{count: 42}}
	got := bt.Count("irrelevant text — the fake codec ignores its input")
	if got != 42 {
		t.Errorf("BPETokenizer.Count = %d, want 42 (the fake codec's fixed count)", got)
	}
}

func TestBPETokenizer_Count_FallsBackOnCodecError(t *testing.T) {
	bt := &BPETokenizer{codec: fakeBPECodec{count: 0, err: errors.New("simulated codec failure")}}

	text := "the quick brown fox jumps" // 5 words -> whitespace approximation is 7
	got := bt.Count(text)
	want := WhitespaceTokenizer{}.Count(text)

	if got != want {
		t.Errorf("BPETokenizer.Count on codec error = %d, want fallback value %d", got, want)
	}
	if got == 0 {
		t.Error("BPETokenizer.Count on codec error returned 0 for non-empty text — fallback did not engage correctly")
	}
}

func TestBPETokenizer_Count_NilCodecFallsBack(t *testing.T) {
	bt := &BPETokenizer{codec: nil}
	text := "hello world"
	got := bt.Count(text)
	want := WhitespaceTokenizer{}.Count(text)
	if got != want {
		t.Errorf("BPETokenizer.Count with nil codec = %d, want fallback value %d", got, want)
	}
}

// TestTokenizer_InterfaceSatisfaction is a compile-time-flavoured
// check (it will fail to build, not just fail an assertion, if either
// type stops satisfying Tokenizer) that both implementations are
// interchangeable through the shared interface.
func TestTokenizer_InterfaceSatisfaction(t *testing.T) {
	var tokenizers = []Tokenizer{
		WhitespaceTokenizer{},
		&BPETokenizer{codec: fakeBPECodec{count: 1}},
	}
	for _, tok := range tokenizers {
		if n := tok.Count("test"); n < 0 {
			t.Errorf("Count returned negative value %d", n)
		}
	}
}
