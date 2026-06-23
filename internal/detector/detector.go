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
// word match from mislabeling a document. Ties return TypeUnknown to
// avoid a false-confident assignment.
//
// TypeLegislation and TypeCBKCircular share vocabulary ("Central Bank
// of Kenya", "Banking Act") because circulars cite the Act they
// implement. They are disambiguated by score, not by check order —
// signatures is a map, and Go map iteration order is randomized, so
// nothing here may rely on one type being scored before another.
// TypeLegislation's signatures ("Cap.", "Laws of Kenya", "An Act of
// Parliament", "PRELIMINARY", "Short title", "Interpretation") are
// statutory structural markers that a circular never contains, so a
// real Act scores high on TypeLegislation and low (typically 1, just
// "Central Bank of Kenya") on TypeCBKCircular — well under the
// tie-breaking threshold.
func Detect(filename, text string) models.Document {
	doc := models.Document{
		RawText:  text,
		Filename: filename,
		Type:     models.TypeUnknown,
	}

	signatures := map[models.DocumentType][]string{
		// Legislation signatures are highly specific statutory phrases that
		// appear in Acts and Statutes but never in circulars or policies.
		// "Cap." is the Kenya statute citation format (e.g. Cap. 491).
		// "Laws of Kenya" appears on the cover of every revised statute.
		// "An Act of Parliament" is the opening recital of every Kenyan Act.
		// "PRELIMINARY" appears as the title of Part I in virtually every Act.
		// "Short title" is section 1 of every Kenyan Act without exception.
		// "Interpretation" is section 2 of virtually every Kenyan Act.
		models.TypeLegislation: {
			"Laws of Kenya",
			"An Act of Parliament",
			"Cap.",
			"Short title",
			"Interpretation",
			"PRELIMINARY",
		},
		// CBK circular signatures: regulatory directives issued by CBK.
		// These phrases appear in circulars but not in the Acts themselves —
		// circulars cite the Act but do not contain its structural recitals.
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
			tiedScore = true
		}
	}

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
