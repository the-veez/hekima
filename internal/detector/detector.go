// Package detector identifies the type of an East African document
// by scanning for structural fingerprints unique to each document type.
//
// This is the first step in Hekima's pipeline — before we can chunk
// intelligently, we need to know what kind of document we're dealing with.
// A CBK circular and a SACCO policy are both text documents, but they
// have completely different structural grammars.
package detector

import "strings"

// DocumentType is a named string type representing a known East African
// document category. Using a named type (instead of plain string) means
// the compiler will catch mistakes like passing the wrong kind of string.
type DocumentType string

const (
	TypeSACCOPolicy   DocumentType = "sacco_policy"
	TypeCBKCircular   DocumentType = "cbk_circular"
	TypeLandTitle     DocumentType = "land_title"
	TypeCourtJudgment DocumentType = "court_judgment"
	TypeUnknown       DocumentType = "unknown"
)

// Document holds a raw document's content and its detected identity.
// Every field is exported (capitalized) so other packages can read them.
type Document struct {
	RawText  string
	Type     DocumentType
	Filename string
}

// Detect reads raw text and returns a Document with its type identified.
// It scores each document type by counting how many of its signature
// phrases appear in the text. The type with the highest score wins.
//
// A minimum score of 2 is required to avoid false positives on partial matches.
func Detect(filename, text string) Document {
	doc := Document{
		RawText:  text,
		Filename: filename,
		Type:     TypeUnknown,
	}

	signatures := map[DocumentType][]string{
		TypeCBKCircular: {
			"Central Bank of Kenya",
			"Governor",
			"Ref. No. CBK",
			"pursuant to",
			"Banking Act",
			"all institutions",
		},
		TypeSACCOPolicy: {
			"SACCO",
			"loan policy",
			"repayment period",
			"guarantor",
			"share capital",
			"member",
		},
		TypeLandTitle: {
			"Land Registration Act",
			"parcel number",
			"Grant No",
			"registered proprietor",
			"freehold",
			"leasehold",
		},
		TypeCourtJudgment: {
			"REPUBLIC OF KENYA",
			"PETITIONER",
			"RESPONDENT",
			"JUDGMENT",
			"RULING",
			"CORAM",
		},
	}

	bestMatch := TypeUnknown
	bestScore := 0

	for docType, phrases := range signatures {
		score := 0
		for _, phrase := range phrases {
			if containsPhrase(text, phrase) {
				score++
			}
		}
		if score > bestScore {
			bestScore = score
			bestMatch = docType
		}
	}

	if bestScore >= 2 {
		doc.Type = bestMatch
	}

	return doc
}

// containsPhrase checks whether a phrase exists anywhere in the text.
// Case-insensitive so "Central Bank of Kenya" matches "central bank of kenya".
func containsPhrase(text, phrase string) bool {
	return strings.Contains(
		strings.ToLower(text),
		strings.ToLower(phrase),
	)
}