// Package detector identifies the type of an East African document
// by scanning for structural fingerprints unique to each document class.
//
// Detection is deterministic and stateless: the same input always
// produces the same output. There is no ML model — East African
// regulatory documents have stable structural conventions that do not
// require probabilistic classification.
package detector

import (
	"strings"

	"github.com/the-veez/hekima/internal/models"
)

// Detect reads raw text and returns a Document with its type identified.
// It scores each document type by counting how many of its signature
// phrases appear in the text. The type with the highest score wins.
//
// A minimum score of 2 is required — this prevents a single accidental
// word match from mislabeling a document. Ties are broken in favour of
// the type with more total signatures matched; if scores are equal,
// TypeUnknown is returned to avoid a false-confident assignment.
func Detect(filename, text string) models.Document {
	doc := models.Document{
		RawText:  text,
		Filename: filename,
		Type:     models.TypeUnknown,
	}

	// Signature phrases are lexical fingerprints for each document type.
	// Chosen because they appear in these document classes and almost
	// nowhere else in Kenyan document corpora.
	signatures := map[models.DocumentType][]string{
		models.TypeCBKCircular: {
			"Central Bank of Kenya",
			"Governor",
			"Ref. No. CBK",
			"pursuant to",
			"Banking Act",
			"all institutions",
		},
		models.TypeSACCOPolicy: {
			"SACCO",
			"loan policy",
			"repayment period",
			"guarantor",
			"share capital",
			"member",
		},
		models.TypeLandTitle: {
			"Land Registration Act",
			"parcel number",
			"Grant No",
			"registered proprietor",
			"freehold",
			"leasehold",
		},
		models.TypeCourtJudgment: {
			"REPUBLIC OF KENYA",
			"PETITIONER",
			"RESPONDENT",
			"JUDGMENT",
			"RULING",
			"CORAM",
		},
	}

	bestMatch := models.TypeUnknown
	bestScore := 0
	tiedScore := false

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
			tiedScore = false
		} else if score == bestScore && score > 0 {
			// Two types matched with equal confidence — not safe to assign either.
			tiedScore = true
		}
	}

	// Require at least 2 signature matches and no tie between types.
	if bestScore >= 2 && !tiedScore {
		doc.Type = bestMatch
	}

	return doc
}

// containsPhrase reports whether phrase exists anywhere in text.
// Case-insensitive so "Central Bank of Kenya" matches "central bank of kenya".
func containsPhrase(text, phrase string) bool {
	return strings.Contains(
		strings.ToLower(text),
		strings.ToLower(phrase),
	)
}
