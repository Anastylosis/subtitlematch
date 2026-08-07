package subtitlematch

import "strings"

// Vocab names the creators in a library — performers and studios, plus
// aliases — so their handles can be told apart from title words.
//
// It holds only squashed names (separators removed, as scrapers write them:
// "riverstone", "thehousenextdoor"), never individual words. Indexing every
// word of every alias sweeps up ordinary vocabulary and gutted title matching
// when measured; see docs/design.md.
type Vocab struct {
	squashed map[string]bool
}

// NewVocab builds a vocabulary from performer and studio names (and aliases).
func NewVocab(names []string) *Vocab {
	v := &Vocab{squashed: map[string]bool{}}
	for _, n := range names {
		words := strings.Fields(nonAlnumRe.ReplaceAllString(strings.ToLower(n), " "))
		if len(words) == 0 {
			continue
		}
		s := strings.Join(words, "")
		// A single-word name must be longer, or a short common word
		// ("molly", "vanity") captures every token starting with it.
		min := 6
		if len(words) == 1 {
			min = 8
		}
		if len(s) >= min {
			v.squashed[s] = true
		}
	}
	return v
}

// Len reports how many distinct creator handles are known.
func (v *Vocab) Len() int {
	if v == nil {
		return 0
	}
	return len(v.squashed)
}

// IsCreator reports whether a token names a creator. Scrapers bolt suffixes
// on ("riverstonexxx"), so a token also counts when it starts with a known
// handle; trailing digits are ignored for the same reason.
func (v *Vocab) IsCreator(tok string) bool {
	if v == nil || tok == "" {
		return false
	}
	if v.squashed[tok] {
		return true
	}
	if bare := strings.TrimRight(tok, "0123456789"); bare != tok && v.squashed[bare] {
		return true
	}
	// Prefixes only, so a handle cannot match mid-word.
	for n := len(tok) - 1; n >= 6; n-- {
		if v.squashed[tok[:n]] {
			return true
		}
	}
	return false
}

// Split partitions tokens into content and creator sets.
func (v *Vocab) Split(toks map[string]bool) (content, creator map[string]bool) {
	content = map[string]bool{}
	creator = map[string]bool{}
	for t := range toks {
		if v.IsCreator(t) {
			creator[t] = true
		} else {
			content[t] = true
		}
	}
	return content, creator
}
