package checks

// languageConsistency flags identifiers that are not consistently English.
var languageConsistency = Check{
	Name:        "language-consistency",
	Description: "Flags identifiers that are not English, or that mix two languages in one symbol",
	Prompt:      languageConsistencyPrompt,
}

const languageConsistencyPrompt = "flag identifiers that are not consistently English. report exactly two kinds of issues:\n" +
	"- mixed-language identifiers: a single symbol splicing two languages (e.g. `getPrénomUtilisateur`, `calculTotalPrice`)\n" +
	"- non-English identifiers: symbols written entirely in a language other than English, even if internally consistent (e.g. `nomUtilisateur`, `precioTotal`)\n" +
	"leave unflagged: string literals; comments; user-facing strings, labels, or messages in another language; proper nouns; acronyms; established loanwords (e.g. `naiveDate`, `RSVP`)\n" +
	"list each offending identifier with its file and a one-line reason, nothing else. put each finding on its own line, followed by one blank line"
