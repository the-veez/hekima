package tokenizer

import "github.com/tiktoken-go/tokenizer/codec"

// bpeCodec is the subset of tiktoken-go/tokenizer/codec.Codec that
// BPETokenizer actually uses. Defining it locally — rather than
// depending on the codec package's exported Codec type directly in
// our struct field — means a future swap of the underlying library
// only touches this file, not callers of BPETokenizer.
type bpeCodec interface {
	Count(input string) (int, error)
}

// BPETokenizer counts tokens using a real OpenAI-compatible
// byte-pair-encoding tokenizer (cl100k_base, the encoding used by
// text-embedding-3-small/large and the GPT-4 family). Use this when
// accurate token counts matter — for example, packing chunks as
// tightly as possible against an embedding model's context limit.
//
// The underlying library embeds its vocabulary at compile time (no
// network access required at runtime), which matters for Hekima:
// East African deployments cannot assume reliable connectivity, so a
// tokenizer that phones home on first use is not acceptable here.
type BPETokenizer struct {
	codec bpeCodec
}

// NewBPETokenizer returns a BPETokenizer using the cl100k_base
// encoding. cl100k_base is the right default for most current
// embedding models; if Hekima later needs to support tokenizers for
// other model families, add a constructor per encoding rather than
// parameterising this one — each encoding has different correctness
// properties and silently picking the wrong one is worse than an
// explicit choice at the call site.
func NewBPETokenizer() *BPETokenizer {
	return &BPETokenizer{codec: codec.NewCl100kBase()}
}

// Count returns the exact cl100k_base token count for text. If the
// underlying codec errors — in practice this only happens on
// malformed input the codec cannot encode — Count falls back to the
// whitespace approximation rather than reporting a wrong zero or
// requiring every caller to handle an error for what is, for Hekima's
// purposes, an annotation rather than a correctness-critical value.
func (b *BPETokenizer) Count(text string) int {
	if b.codec == nil {
		return WhitespaceTokenizer{}.Count(text)
	}
	n, err := b.codec.Count(text)
	if err != nil {
		return WhitespaceTokenizer{}.Count(text)
	}
	return n
}