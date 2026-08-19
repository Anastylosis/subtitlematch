package subtitlematch

import (
	"encoding/json"
	"flag"
	"os"
	"path"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"
)

// updateGolden regenerates the golden verdict file from today's replay
// instead of checking against it: go test ./internal/subs/ -run
// TestCorpusReplay -update
var updateGolden = flag.Bool("update", false, "regenerate the corpus replay golden file")

// Replays a match report generated against a real library and compares the
// verdicts to a golden file, so a scoring change that quietly reshuffles
// thousands of real matches shows up as a diff.
//
// The corpus is never committed: subtitle stems are live library filenames.
// Point SUBS_CORPUS at a report to run this; the test skips without it.
//
//	SUBS_CORPUS=/path/to/subtitle-match-report.md go test ./...
//	SUBS_CORPUS=/path/to/report.json go test ./...
//
// Two report shapes are accepted, dispatched on the SUBS_CORPUS extension.
// The legacy Markdown format (Custodian's old human-readable report, which
// Custodian no longer produces but old corpora still use) carries no scene
// date/studio/performers, so it can never exercise the date signal — its
// "why=" strings are ignored regardless, since the scorer has moved on since
// any given report was generated; only the structure is parsed: subtitle,
// candidates, verdict section. A `.json` path is parsed as the shape
// Custodian's `subs scan --json` writes today (see jsonCorpusReport), which
// does carry scene dates and is what makes a date-mismatch replay possible.

// corpusPath is the report to replay, and corpusGoldenPath the expected
// verdicts beside it. Both live outside the repo.
var (
	corpusPath       = os.Getenv("SUBS_CORPUS")
	corpusGoldenPath = corpusPath + ".golden.json"
)

// corpusRoot is the scan root the report's "in `<dir>`" lines are relative to;
// everything below it becomes Subtitle.Dirs. Defaults to the report's own
// convention when SUBS_CORPUS_ROOT is unset.
var corpusRoot = os.Getenv("SUBS_CORPUS_ROOT")

var (
	corpusHeaderRe    = regexp.MustCompile(`^\d+ orphaned subtitle files\.\s+CONFIRMED=(\d+)\s+LIKELY=(\d+)\s+AMBIGUOUS=(\d+)\s*$`)
	corpusSectionRe   = regexp.MustCompile(`^## (CONFIRMED|LIKELY|AMBIGUOUS)\b`)
	corpusSubtitleRe  = regexp.MustCompile("^\\*\\*`(.+)`\\*\\*\\s+_\\(runtime (.+)\\)_\\s*$")
	corpusDirRe       = regexp.MustCompile("^  in `(.+)`$")
	corpusCandidateRe = regexp.MustCompile(`^\s*(?:->\s*)?#(\d+)\s+score=([-0-9.]+)\s+\[([^\]]+)\](\s*\[already has captions\])?\s*$`)
	corpusPathLineRe  = regexp.MustCompile("^\\s+`(.+)`$")
	corpusHMSRe       = regexp.MustCompile(`^(\d+):(\d{2}):(\d{2})$`)
)

// corpusEntry is one subtitle from the report, paired with the verdict
// recorded for it (the section it was filed under) and the candidate scenes
// the original run listed under it. scenes is per-entry, not pooled: the
// original tool scored this subtitle against its full library and the
// recorded candidates are the top rivals that competed, including whichever
// runner-up decide()'s score-gap thresholds need. Pooling every scene from
// every entry into one shared index (an earlier version of this harness did
// that) introduces rivals that never competed for this subtitle in the
// original run and is a harness artifact, not a scorer behavior.
type corpusEntry struct {
	sub     Subtitle
	verdict Verdict
	scenes  []SceneRef
}

// corpusCounts tallies subtitles by verdict.
type corpusCounts struct {
	Confirmed, Likely, Ambiguous, Unmatched int
}

func (c *corpusCounts) add(v Verdict) {
	switch v {
	case Confirmed:
		c.Confirmed++
	case Likely:
		c.Likely++
	case Ambiguous:
		c.Ambiguous++
	default:
		c.Unmatched++
	}
}

// parseCorpusHMS parses "H:MM:SS", or "?" as unknown (zero).
func parseCorpusHMS(s string) time.Duration {
	if s == "?" {
		return 0
	}
	m := corpusHMSRe.FindStringSubmatch(s)
	if m == nil {
		return 0
	}
	h, _ := strconv.Atoi(m[1])
	mi, _ := strconv.Atoi(m[2])
	sec, _ := strconv.Atoi(m[3])
	return time.Duration(h)*time.Hour + time.Duration(mi)*time.Minute + time.Duration(sec)*time.Second
}

// stemOf strips a filename's extension.
func stemOf(filename string) string {
	if i := strings.LastIndex(filename, "."); i > 0 {
		return filename[:i]
	}
	return filename
}

// parseCorpus reads the report and reconstructs the ordered list of
// subtitles, each with its own recorded verdict and its own candidate
// scenes (only the scenes the report listed under that specific subtitle —
// see corpusEntry).
func parseCorpus(data string) (entries []corpusEntry, header corpusCounts) {
	var section Verdict
	var cur *corpusEntry
	var curFilename string

	var pendingDur string
	var pendingHasCaption, havePending bool
	var pendingIdx int // index into cur.scenes of the candidate awaiting its path line

	for _, raw := range strings.Split(data, "\n") {
		line := strings.TrimRight(raw, "\r")

		// The copy plan section reuses "#<id>" and backtick-path shapes for a
		// different purpose (from/to renames); stop before it.
		if strings.HasPrefix(line, "## Copy plan") {
			break
		}
		if m := corpusHeaderRe.FindStringSubmatch(line); m != nil {
			header.Confirmed, _ = strconv.Atoi(m[1])
			header.Likely, _ = strconv.Atoi(m[2])
			header.Ambiguous, _ = strconv.Atoi(m[3])
			continue
		}
		if m := corpusSectionRe.FindStringSubmatch(line); m != nil {
			switch m[1] {
			case "CONFIRMED":
				section = Confirmed
			case "LIKELY":
				section = Likely
			case "AMBIGUOUS":
				section = Ambiguous
			}
			cur = nil
			continue
		}
		if m := corpusSubtitleRe.FindStringSubmatch(line); m != nil {
			curFilename = m[1]
			entries = append(entries, corpusEntry{
				sub: Subtitle{
					Stem:    stemOf(curFilename),
					Runtime: parseCorpusHMS(m[2]),
				},
				verdict: section,
			})
			cur = &entries[len(entries)-1]
			havePending = false
			continue
		}
		if cur == nil {
			continue
		}
		if m := corpusDirRe.FindStringSubmatch(line); m != nil {
			dir := m[1]
			rel := strings.TrimPrefix(dir, corpusRoot)
			rel = strings.Trim(rel, "/")
			if rel != "" {
				cur.sub.Dirs = strings.Split(rel, "/")
			}
			cur.sub.Path = dir + "/" + curFilename
			continue
		}
		if m := corpusCandidateRe.FindStringSubmatch(line); m != nil {
			cur.scenes = append(cur.scenes, SceneRef{
				ID:         m[1],
				Duration:   parseCorpusHMS(m[3]),
				HasCaption: m[4] != "",
			})
			pendingDur = m[3]
			pendingHasCaption = m[4] != ""
			pendingIdx = len(cur.scenes) - 1
			havePending = true
			continue
		}
		if havePending {
			if m := corpusPathLineRe.FindStringSubmatch(line); m != nil {
				p := m[1]
				sc := &cur.scenes[pendingIdx]
				sc.Path = p
				sc.Stem = stemOf(path.Base(p))
				sc.Duration = parseCorpusHMS(pendingDur)
				sc.HasCaption = pendingHasCaption
				havePending = false
				continue
			}
		}
		// "why=" lines and blank lines carry no structure we need.
	}

	return entries, header
}

// jsonCorpusReport is the subset of Custodian's `subs scan --json` shape this
// replay understands. Defined locally rather than imported from Custodian —
// this package has no business depending on a caller, and the corpus is
// anchored on the wire shape, not whatever Go type happens to produce it.
//
// Unlike the markdown report, the JSON shape carries scene date, studio and
// performers on every candidate (SceneRef has always had them), which is
// what lets this replay exercise the date signal at all. It does NOT carry a
// subtitle-side date: Custodian's currently-vendored subtitlematch is v0.1.0,
// which predates Subtitle.Date, so its JSON has nothing to put there. A
// subtitle stem with an embedded YYYY-MM-DD still drives the date signal via
// the Stem fallback in Index.Match.
type jsonCorpusReport struct {
	Summary struct {
		Confirmed int `json:"confirmed"`
		Likely    int `json:"likely"`
		Ambiguous int `json:"ambiguous"`
	} `json:"summary"`
	Matches []jsonCorpusMatch `json:"matches"`
}

type jsonCorpusMatch struct {
	Subtitle string                `json:"subtitle"`
	Stem     string                `json:"stem"`
	Runtime  string                `json:"runtime"`
	Lang     string                `json:"lang"`
	Dirs     []string              `json:"dirs"`
	Verdict  string                `json:"verdict"`
	Cands    []jsonCorpusCandidate `json:"candidates"`
}

type jsonCorpusCandidate struct {
	SceneID    string   `json:"scene_id"`
	Path       string   `json:"path"`
	Stem       string   `json:"stem"`
	Title      string   `json:"title"`
	Date       string   `json:"date"`
	Studio     string   `json:"studio"`
	Performers []string `json:"performers"`
	Duration   string   `json:"duration"`
}

// parseCorpusJSON builds the same []corpusEntry the markdown parser builds
// (parseCorpus), from a JSON report shaped like Custodian's `subs scan
// --json` output. Like the markdown path, UNMATCHED entries are dropped:
// the markdown corpus never recorded them either (its sections are only
// CONFIRMED/LIKELY/AMBIGUOUS), and the header sanity check in
// TestCorpusReplay assumes their absence.
func parseCorpusJSON(data []byte) (entries []corpusEntry, header corpusCounts, err error) {
	var report jsonCorpusReport
	if err := json.Unmarshal(data, &report); err != nil {
		return nil, corpusCounts{}, err
	}
	header.Confirmed = report.Summary.Confirmed
	header.Likely = report.Summary.Likely
	header.Ambiguous = report.Summary.Ambiguous

	for _, m := range report.Matches {
		verdict := Verdict(m.Verdict)
		if verdict == Unmatched {
			continue
		}
		entry := corpusEntry{
			sub: Subtitle{
				Path:    m.Subtitle,
				Stem:    m.Stem,
				Runtime: parseCorpusHMS(m.Runtime),
				Lang:    m.Lang,
				Dirs:    m.Dirs,
			},
			verdict: verdict,
		}
		for _, c := range m.Cands {
			entry.scenes = append(entry.scenes, SceneRef{
				ID:         c.SceneID,
				Title:      c.Title,
				Date:       c.Date,
				Studio:     c.Studio,
				Performers: c.Performers,
				Path:       c.Path,
				Stem:       c.Stem,
				Duration:   parseCorpusHMS(c.Duration),
			})
		}
		entries = append(entries, entry)
	}
	return entries, header, nil
}

// TestCorpusReplayJSONParsesDatesAndTriggersDateMismatch is a small,
// self-contained fixture (no real library data) proving two things about the
// JSON corpus path that the golden-file replay above cannot: that scene
// dates survive the JSON round-trip into SceneRef, and that a same-title,
// same-runtime decoy differing only in date earns a "date mismatch" reason
// once replayed through the real scorer — the entire reason this format
// exists, since the markdown report never carried a date to begin with.
func TestCorpusReplayJSONParsesDatesAndTriggersDateMismatch(t *testing.T) {
	const fixture = `{
	  "summary": {"confirmed": 1, "likely": 0, "ambiguous": 1},
	  "matches": [
	    {
	      "subtitle": "/loose/subs/some.clip.srt",
	      "stem": "some.clip",
	      "runtime": "0:41:15",
	      "lang": "en",
	      "verdict": "CONFIRMED",
	      "candidates": [
	        {
	          "scene_id": "101",
	          "path": "/videos/some.clip.mp4",
	          "stem": "some.clip",
	          "title": "Some Clip",
	          "date": "2024-07-04",
	          "studio": "Test Studio",
	          "performers": ["Alex Rivers"],
	          "duration": "0:41:16"
	        }
	      ]
	    },
	    {
	      "subtitle": "/loose/subs/2024-03-10_TwoSisters-SharePrize.srt",
	      "stem": "2024-03-10_TwoSisters-SharePrize",
	      "runtime": "0:20:01",
	      "lang": "en",
	      "verdict": "AMBIGUOUS",
	      "candidates": [
	        {
	          "scene_id": "1",
	          "path": "/videos/two-sisters-a.mp4",
	          "stem": "TwoSisters-SharePrize",
	          "title": "Two Sisters Share The Prize",
	          "date": "2024-03-10",
	          "duration": "0:20:00"
	        },
	        {
	          "scene_id": "2",
	          "path": "/videos/two-sisters-b.mp4",
	          "stem": "TwoSisters-SharePrize",
	          "title": "Two Sisters Share The Prize",
	          "date": "2024-06-01",
	          "duration": "0:20:00"
	        }
	      ]
	    }
	  ]
	}`

	entries, header, err := parseCorpusJSON([]byte(fixture))
	if err != nil {
		t.Fatalf("parseCorpusJSON: %v", err)
	}
	if got, want := len(entries), header.Confirmed+header.Likely+header.Ambiguous; got != want {
		t.Fatalf("parsed %d entries, header claims %d", got, want)
	}
	if len(entries) != 2 {
		t.Fatalf("entries = %d, want 2", len(entries))
	}

	// First proof: scene dates survive the round-trip.
	if got := entries[0].scenes[0].Date; got != "2024-07-04" {
		t.Errorf("scene date = %q, want 2024-07-04", got)
	}
	if got := entries[0].scenes[0].Studio; got != "Test Studio" {
		t.Errorf("scene studio = %q, want %q", got, "Test Studio")
	}
	if got := entries[0].scenes[0].Performers; len(got) != 1 || got[0] != "Alex Rivers" {
		t.Errorf("scene performers = %v, want [Alex Rivers]", got)
	}

	// Second proof: replaying the decoy entry through the real scorer
	// produces a "date mismatch" reason. The JSON carries no subtitle-side
	// date (see jsonCorpusReport's doc comment), so this only fires because
	// Index.Match falls back to the YYYY-MM-DD embedded in the stem.
	decoy := entries[1]
	if decoy.sub.Date != "" {
		t.Fatalf("subtitle date = %q, want empty (JSON has no subtitle date field)", decoy.sub.Date)
	}
	ix := NewIndex(decoy.scenes, nil)
	m := ix.Match(decoy.sub, 5)

	var sceneTwo *Candidate
	for i := range m.Candidates {
		if m.Candidates[i].Scene.ID == "2" {
			sceneTwo = &m.Candidates[i]
		}
	}
	if sceneTwo == nil {
		t.Fatalf("scene 2 not scored as a candidate: %+v", m.Candidates)
	}
	found := false
	for _, r := range sceneTwo.Reasons {
		if strings.HasPrefix(r, "date mismatch") {
			found = true
		}
	}
	if !found {
		t.Errorf("scene 2 reasons = %v, want one starting with %q", sceneTwo.Reasons, "date mismatch")
	}
}

// TestCorpusReplay replays the real-library regression corpus
// (../../../subtitle-match-report.md, gitignored, not present in CI) through
// the ported matcher, one subtitle at a time.
//
// Each subtitle is matched against a mini-Index built ONLY from that
// subtitle's own recorded candidates (see corpusEntry) — the actual rivals
// that competed for it in the original full-library run — rather than a
// library pooled from every entry in the report, which would let scenes that
// never competed for this subtitle collide with it by coincidence of shared
// tokens (a harness artifact an earlier version of this test had).
//
// Even so, this replay cannot reproduce the original run bit-for-bit: the
// report records no scene titles/studios/performers (Title is always empty
// here) and no performer/studio vocabulary (Vocab is nil), and it was
// generated by an older, different version of the scorer than what's ported
// here (its "why=" strings don't match current Reasons text, which is why
// they're ignored rather than parsed). So the corpus's job is to anchor a
// realistic INPUT — real filenames, real runtimes, real candidate sets from a
// live library — while a golden file pins what THIS ported scorer verifiably
// does with that input today, so a future change to match.go/normalize.go/
// vocab.go that silently shifts verdicts on real data gets caught.
//
// If the fix ever legitimately changes verdicts here (an intentional scoring
// change, not a regression), regenerate the golden file with -update; do not
// hand-edit it.
func TestCorpusReplay(t *testing.T) {
	if corpusPath == "" {
		t.Skip("set SUBS_CORPUS to a match report to replay it")
	}
	data, err := os.ReadFile(corpusPath)
	if err != nil {
		t.Skipf("corpus not readable at %s: %v", corpusPath, err)
	}

	var entries []corpusEntry
	var header corpusCounts
	if strings.HasSuffix(corpusPath, ".json") {
		entries, header, err = parseCorpusJSON(data)
		if err != nil {
			t.Fatalf("parse JSON corpus %s: %v", corpusPath, err)
		}
	} else {
		entries, header = parseCorpus(string(data))
	}
	if got, want := len(entries), header.Confirmed+header.Likely+header.Ambiguous; got != want {
		t.Fatalf("parsed %d subtitle entries, but the report header claims CONFIRMED=%d LIKELY=%d AMBIGUOUS=%d (%d total)",
			got, header.Confirmed, header.Likely, header.Ambiguous, want)
	}

	replayed := make(map[string]Verdict, len(entries))
	var recordedTally, replayedTally corpusCounts
	var diffs []string
	for _, e := range entries {
		ix := NewIndex(e.scenes, nil)
		m := ix.Match(e.sub, 5)
		replayed[e.sub.Stem] = m.Verdict
		recordedTally.add(e.verdict)
		replayedTally.add(m.Verdict)
		if m.Verdict != e.verdict {
			best := "no candidate"
			if b := m.Best(); b != nil {
				best = b.Scene.ID + " score=" + strconv.FormatFloat(b.Score, 'f', 1, 64)
			}
			diffs = append(diffs, "  "+e.sub.Stem+": recorded="+string(e.verdict)+" replayed="+string(m.Verdict)+" ("+best+", "+strconv.Itoa(len(e.scenes))+" candidates)")
		}
		t.Logf("%-90s recorded=%-10s replayed=%-10s", e.sub.Stem, e.verdict, m.Verdict)
	}

	t.Logf("recorded distribution:  CONFIRMED=%d LIKELY=%d AMBIGUOUS=%d",
		recordedTally.Confirmed, recordedTally.Likely, recordedTally.Ambiguous)
	t.Logf("replayed distribution:  CONFIRMED=%d LIKELY=%d AMBIGUOUS=%d UNMATCHED=%d",
		replayedTally.Confirmed, replayedTally.Likely, replayedTally.Ambiguous, replayedTally.Unmatched)
	if len(diffs) > 0 {
		t.Logf("%d/%d entries differ from the recorded verdict:\n%s", len(diffs), len(entries), strings.Join(diffs, "\n"))
	} else {
		t.Logf("all %d entries match the recorded verdict", len(entries))
	}

	if *updateGolden {
		golden, err := json.MarshalIndent(replayed, "", "  ")
		if err != nil {
			t.Fatalf("marshal golden: %v", err)
		}
		if err := os.WriteFile(corpusGoldenPath, golden, 0o644); err != nil {
			t.Fatalf("write golden file %s: %v", corpusGoldenPath, err)
		}
		t.Logf("wrote golden file %s (%d entries)", corpusGoldenPath, len(replayed))
		return
	}

	goldenData, err := os.ReadFile(corpusGoldenPath)
	if err != nil {
		t.Skipf("golden file not present at %s; regenerate with: go test ./internal/subs/ -run TestCorpusReplay -update", corpusGoldenPath)
	}
	var golden map[string]Verdict
	if err := json.Unmarshal(goldenData, &golden); err != nil {
		t.Fatalf("parse golden file %s: %v", corpusGoldenPath, err)
	}

	if len(golden) != len(replayed) {
		t.Errorf("golden file has %d entries, replay produced %d — regenerate with -update if this is an intentional corpus/scorer change",
			len(golden), len(replayed))
	}
	for stem, want := range golden {
		got, ok := replayed[stem]
		if !ok {
			t.Errorf("golden entry %q not present in this replay (corpus changed?)", stem)
			continue
		}
		if got != want {
			t.Errorf("regression: %q replayed %s, golden pinned %s — this is either a scorer behavior change "+
				"(regenerate the golden with -update if intentional) or an actual regression", stem, got, want)
		}
	}
}
