// Package models defines the shared data types used across Hekima's
// internal packages.
//
// Placing shared types here — rather than inside the detector or chunker
// packages — means neither package needs to import the other. The
// dependency graph flows in one direction:
//
//	models → (nothing)
//	detector → models
//	chunker  → models
//	cli      → models, detector, chunker
//	main     → cli
package models

// DocumentType is a named string type representing a known East African
// document category. Using a named type (instead of plain string) means
// the compiler catches mistakes like passing the wrong kind of string.
type DocumentType string

const (
	TypeSACCOPolicy   DocumentType = "sacco_policy"
	TypeCBKCircular   DocumentType = "cbk_circular"
	TypeLandTitle     DocumentType = "land_title"
	TypeCourtJudgment DocumentType = "court_judgment"
	TypeLegislation   DocumentType = "legislation"
	TypeUnknown       DocumentType = "unknown"
)

// Document holds a raw document's content and its detected identity.
type Document struct {
	RawText  string
	Type     DocumentType
	Filename string
}

// Chunk is one semantically complete piece of a document.
// It carries the text and enough metadata for an AI retrieval system
// to understand where this chunk came from and what it means.
//
// TokenCount is always populated (no omitempty) so downstream consumers
// can rely on it being present without a nil/zero check meaning "unknown".
type Chunk struct {
	ID         int               `json:"id"`
	Text       string            `json:"text"`
	Section    string            `json:"section"`
	DocType    string            `json:"doc_type"`
	Filename   string            `json:"filename"`
	TokenCount int               `json:"token_count"`
	Metadata   map[string]string `json:"metadata,omitempty"`
}
