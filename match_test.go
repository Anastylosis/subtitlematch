package subtitlematch

import (
	"strings"
	"testing"
	"time"
)

// testVocab mimics what CreatorNames returns from a live library: performer
// and studio names plus aliases, including an alias that is also an ordinary
// English word — the case that makes blanket-dropping creator words unsafe.
func testVocab() *Vocab {
	return NewVocab([]string{
		"River Stone", "RiverStone", "Quinn Marsh", "Hana Kirsawa",
		"Wren Ashby", "Sable Vance", "thehousenextdoor",
		"Molly", // a real alias that is also a common given name
	})
}

func scene(id, stem, title string, d time.Duration, performers ...string) SceneRef {
	return SceneRef{
		ID: id, Stem: stem, Title: title, Duration: d,
		Performers: performers, Path: "/data/x/" + stem + ".mp4",
	}
}

func TestCodes(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want string
	}{
		{"venu00765", "VENU-765"},
		{"VENU-765 Chinese", "VENU-765"},
		{"RCTD-590-en", "RCTD-590"},
		{"jul-256 Toyosaki Misato saliva", "JUL-256"},
		{"a1080hd.com@13gvg00367hhb", "GVG-367"},
	} {
		got := Codes(tc.in)
		if !got[tc.want] {
			t.Errorf("Codes(%q) = %v, want it to contain %q", tc.in, keys(got), tc.want)
		}
	}
}

func keys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

// "stepMommy" must fold to "mommy" -> "mom". Missing this caused two real
// matches to be reported as unmatched.
func TestTokens_FoldsStepPrefix(t *testing.T) {
	a := Tokens("RiverStoneXXX_Mommy-and-Son-Nesting-Season")
	b := Tokens("2023-05-23_River.Stone-stepMommy.and.stepSon.Nesting.Season_1080p")
	if !a["mom"] || !b["mom"] {
		t.Fatalf("step prefix not folded: a=%v b=%v", keys(a), keys(b))
	}
	if sim := F1(a, b); sim < 0.6 {
		t.Errorf("similarity = %.2f, want >= 0.60 (a=%v b=%v)", sim, keys(a), keys(b))
	}
}

// The regression that started this: names barely overlap, runtime decides.
func TestMatch_BreedingSeason(t *testing.T) {
	ix := NewIndex([]SceneRef{
		scene("54548", "2023-05-23_River.Stone-stepMommy.and.stepSon.Nesting.Season_1080p",
			"stepMommy and stepSon Nesting Season", mins(23, 52), "River Stone"),
		scene("73661", "Quinn Marsh - Mommy & Son Go Live (Extreme Taboo Nesting) (x265)",
			"", mins(31, 51), "Quinn Marsh"),
		scene("73662", "Quinn Marsh - Mommy & Son Storm Nesting (x265)",
			"", mins(20, 59), "Quinn Marsh"),
	}, testVocab())
	m := ix.Match(Subtitle{
		Stem:    "RiverStoneXXX_Mommy-and-Son-Nesting-Season+(transcribed+on+25-Jul-2024+00-23-01)",
		Runtime: mins(23, 45),
	}, 5)

	best := m.Best()
	if best == nil || best.Scene.ID != "54548" {
		t.Fatalf("best = %+v, want scene 54548 (all: %+v)", best, m.Candidates)
	}
	if m.Verdict != Confirmed {
		t.Errorf("verdict = %s, want CONFIRMED (reasons: %v)", m.Verdict, best.Reasons)
	}
}

// A same-titled decoy 23 minutes away must lose to the one 2 seconds away.
func TestMatch_RuntimeSeparatesIdenticalTitles(t *testing.T) {
	ix := NewIndex([]SceneRef{
		scene("55739", "2022-10-13_River.Stone-Nurse.River.Soothes.The.Aching.Patient_1080p",
			"Nurse River Soothes The Aching Patient", mins(10, 54), "River Stone"),
		scene("75175", "2022-10-13_River.Stone-Nurse.River.Soothes.The.Aching.Patient_1080p_1",
			"Nurse River Soothes The Aching Patient", mins(33, 49), "River Stone"),
	}, testVocab())
	m := ix.Match(Subtitle{
		Stem:    "Nurse+River+Soothes+the+Aching+Patient+WM+(transcribed+on+24-Jul-2024+20-55-00)",
		Runtime: mins(10, 52),
	}, 5)
	if b := m.Best(); b == nil || b.Scene.ID != "55739" {
		t.Fatalf("best = %+v, want 55739", b)
	}
	if m.Verdict == Ambiguous || m.Verdict == Unmatched {
		t.Errorf("verdict = %s, want a decisive verdict", m.Verdict)
	}
}

// Dog -> Pet is a real rename in this library; runtime carries the match.
func TestMatch_DogPetSitterRename(t *testing.T) {
	ix := NewIndex([]SceneRef{
		scene("76466", "The-Reluctant-Pet-Sitter-Part-1", "", mins(11, 40)),
		scene("6278", "Emmas Secret Life in The Dog Sitter", "", mins(14, 54)),
	}, testVocab())
	m := ix.Match(Subtitle{
		Stem:    "thehousenextdoor2024 - The Reluctant Dog Sitter Part 1 - Compressed",
		Runtime: mins(11, 38),
	}, 5)
	if b := m.Best(); b == nil || b.Scene.ID != "76466" {
		t.Fatalf("best = %+v, want 76466", b)
	}
}

// A JAV code should dominate even when the titles share nothing.
func TestMatch_JAVCode(t *testing.T) {
	ix := NewIndex([]SceneRef{
		scene("60771", "2024-06-20_Hana.Kirsawa_Yui.Tomita_Mei.Arano-RCTD-590_1080p",
			"RCTD-590", mins(132, 1), "Hana Kirsawa"),
		scene("99999", "some other clip", "Unrelated", mins(20, 0)),
	}, testVocab())
	m := ix.Match(Subtitle{Stem: "RCTD-590-en"}, 5)
	if b := m.Best(); b == nil || b.Scene.ID != "60771" {
		t.Fatalf("best = %+v, want 60771", b)
	}
}

// Two identical copies of the same film must not be auto-assigned.
func TestMatch_TrueTieIsAmbiguous(t *testing.T) {
	ix := NewIndex([]SceneRef{
		scene("52322", "La novia celosa", "", mins(36, 46)),
		scene("69705", "La Novia Celosa", "", mins(36, 50)),
	}, testVocab())
	m := ix.Match(Subtitle{Stem: "La_novia_celosa_1", Runtime: mins(36, 36)}, 5)
	if m.Verdict != Ambiguous {
		t.Errorf("verdict = %s, want AMBIGUOUS (candidates %+v)", m.Verdict, m.Candidates)
	}
}

// Nothing in the library resembles it: say so rather than guessing.
func TestMatch_Unmatched(t *testing.T) {
	ix := NewIndex([]SceneRef{
		scene("1", "completely different content", "Nothing Alike", mins(9, 0)),
	}, testVocab())
	m := ix.Match(Subtitle{
		Stem:    "thehousenextdoor1 - Your Sinful Aunt Bringing Out Mommys Secret Side",
		Runtime: mins(18, 3),
	}, 5)
	if m.Verdict != Unmatched && m.Verdict != Ambiguous {
		t.Errorf("verdict = %s, want UNMATCHED/AMBIGUOUS", m.Verdict)
	}
}

// A subtitle that clearly outlives its supposed video cannot belong to it.
//
// A few seconds over is normal noise — Runtime takes the maximum timestamp,
// which includes cue END times — so the tolerance is deliberately generous and
// only a substantial overrun counts as evidence against.
func TestMatch_SubtitleClearlyLongerThanVideoIsRejected(t *testing.T) {
	ix := NewIndex([]SceneRef{
		scene("77855", "Wren Ashby - Mommy Has a Little Obsession", "", mins(20, 0)),
	}, testVocab())
	m := ix.Match(Subtitle{Stem: "Mommy Has a Little Obsession", Runtime: mins(30, 0)}, 5)
	if m.Verdict == Confirmed || m.Verdict == Likely {
		t.Errorf("verdict = %s for a subtitle 10min longer than the video; want AMBIGUOUS/UNMATCHED", m.Verdict)
	}
}

// A small overrun must NOT disqualify an otherwise good match.
func TestMatch_SmallOverrunIsTolerated(t *testing.T) {
	ix := NewIndex([]SceneRef{
		scene("1", "2021-06-18_Sable.Vance-Look.into.My.Eyes_1080p",
			"Look into My Eyes", mins(24, 59), "Sable Vance"),
	}, testVocab())
	m := ix.Match(Subtitle{
		Stem:    "2021-06-18_Sable.Vance-Look.into.My.Eyes_1080p",
		Runtime: mins(25, 6),
	}, 5)
	if m.Verdict != Confirmed {
		t.Errorf("verdict = %s, want CONFIRMED (a 7s overrun is within tolerance)", m.Verdict)
	}
}

// An exact filename match is the easy case and must be confirmed outright.
func TestMatch_ExactFilename(t *testing.T) {
	ix := NewIndex([]SceneRef{
		scene("70051", "2021-06-18_Sable.Vance-Look.into.My.Eyes_1080p",
			"Look into My Eyes", mins(11, 50), "Sable Vance"),
	}, testVocab())
	m := ix.Match(Subtitle{
		Stem:    "2021-06-18_Sable.Vance-Look.into.My.Eyes_1080p",
		Runtime: mins(11, 49),
	}, 5)
	if m.Verdict != Confirmed {
		t.Errorf("verdict = %s, want CONFIRMED", m.Verdict)
	}
}

// Libraries put the identifying information in different places. These two
// tests pin both conventions, because scoring a merged blob served neither:
// a junk filename diluted a good title, and a long filename dragged similarity
// down through sheer token count.

// Convention A: Stash-Box tagged. Good title, meaningless filename.
func TestMatch_TitleOnlyLibrary(t *testing.T) {
	ix := NewIndex([]SceneRef{
		{ID: "1", Stem: "13gvg00367hhb", Title: "The Reluctant Pet Sitter",
			Path: "/data/x/13gvg00367hhb.mp4", Duration: mins(11, 40)},
		{ID: "2", Stem: "1080p", Title: "Something Else Entirely",
			Path: "/data/x/1080p.wmv", Duration: mins(11, 38)},
	}, testVocab())

	m := ix.Match(Subtitle{Stem: "The Reluctant Pet Sitter", Runtime: mins(11, 38)}, 5)
	best := m.Best()
	if best == nil || best.Scene.ID != "1" {
		t.Fatalf("best = %+v, want scene 1 matched on its title", best)
	}
	if m.Verdict != Confirmed {
		t.Errorf("verdict = %s, want CONFIRMED (reasons %v)", m.Verdict, best.Reasons)
	}
}

// Convention B: no titles at all, everything encoded in the filename.
func TestMatch_FilenameOnlyLibrary(t *testing.T) {
	ix := NewIndex([]SceneRef{
		{ID: "1", Stem: "2023-05-23_Some.Performer-The.Reluctant.Pet.Sitter_1080p", Title: "",
			Path: "/data/x/a.mp4", Duration: mins(11, 40)},
		{ID: "2", Stem: "2023-05-23_Some.Performer-Something.Else.Entirely_1080p", Title: "",
			Path: "/data/x/b.mp4", Duration: mins(11, 38)},
	}, testVocab())

	m := ix.Match(Subtitle{Stem: "The Reluctant Pet Sitter", Runtime: mins(11, 38)}, 5)
	best := m.Best()
	if best == nil || best.Scene.ID != "1" {
		t.Fatalf("best = %+v, want scene 1 matched on its filename", best)
	}
}

// A junk filename must not drag down a scene whose title matches well.
func TestMatch_JunkFilenameDoesNotDiluteGoodTitle(t *testing.T) {
	ix := NewIndex([]SceneRef{
		{ID: "1", Stem: "video_2023_final_v2_reencoded_x265_aac_2ch",
			Title: "Look into My Eyes", Path: "/data/x/a.mp4", Duration: mins(11, 50)},
	}, testVocab())

	m := ix.Match(Subtitle{Stem: "Look into My Eyes", Runtime: mins(11, 49)}, 5)
	best := m.Best()
	if best == nil {
		t.Fatal("no candidate found")
	}
	// Scored on the title, the match is near-perfect; merged with that
	// filename it would be badly diluted.
	if best.Score < 90 {
		t.Errorf("score %.0f is too low; the title should carry it (reasons %v)",
			best.Score, best.Reasons)
	}
}

// People file subtitles under a performer folder, which names the creator even
// when the filename does not. Ignoring that discards the strongest hint
// available for a generically-named file.
func TestMatch_FolderNamesGiveCreatorHint(t *testing.T) {
	scenes := []SceneRef{
		{ID: "1", Stem: "clip_a", Title: "Prom Night", Path: "/data/a/clip_a.mp4",
			Duration: mins(10, 0), Performers: []string{"River Stone"}},
		{ID: "2", Stem: "clip_b", Title: "Prom Night", Path: "/data/b/clip_b.mp4",
			Duration: mins(10, 0), Performers: []string{"Quinn Marsh"}},
	}
	ix := NewIndex(scenes, testVocab())

	// Identical title and runtime: only the folder can separate them.
	m := ix.Match(Subtitle{
		Stem:    "Prom Night",
		Runtime: mins(9, 58),
		Dirs:    []string{"Quinn Marsh", "Subs"},
	}, 5)

	best := m.Best()
	if best == nil {
		t.Fatal("no candidate")
	}
	if best.Scene.ID != "2" {
		t.Errorf("best = scene %s, want 2 (the folder names its performer)", best.Scene.ID)
	}
}

// The folder must inform who, never what: a folder called "Subs" or "Downloads"
// must not contribute title evidence.
func TestMatch_FolderNamesAreNotTitleEvidence(t *testing.T) {
	ix := NewIndex([]SceneRef{
		{ID: "1", Stem: "x", Title: "Subs Downloads Complete", Path: "/data/a/x.mp4",
			Duration: mins(10, 0)},
	}, testVocab())

	m := ix.Match(Subtitle{
		Stem:    "Totally Different Name",
		Runtime: mins(9, 58),
		Dirs:    []string{"Subs", "Downloads", "Complete"},
	}, 5)
	if b := m.Best(); b != nil && b.NameSim > 0 {
		t.Errorf("folder words leaked into title similarity: %.2f (%v)", b.NameSim, b.Reasons)
	}
}

// dateReason returns the candidate's date-related reason, if any. Exactly one
// reason string is expected to start with "date" — this pins that invariant
// while reading the reason out.
func dateReason(t *testing.T, c *Candidate) string {
	t.Helper()
	var found string
	for _, r := range c.Reasons {
		if strings.HasPrefix(r, "date") {
			if found != "" {
				t.Fatalf("more than one date reason: %q and %q", found, r)
			}
			found = r
		}
	}
	return found
}

// The date rule itself: agreement within 2 days scores +25, a wider gap
// scores -40, and anything unparseable or missing on either side is neutral
// rather than penalised — garbage input must never count against a
// candidate. See docs/design.md.
func TestMatch_DateSignal(t *testing.T) {
	newIndex := func(sceneDate string) *Index {
		ix := NewIndex([]SceneRef{
			scene("1", "some-stem", "Some Unique Title Words", mins(20, 0)),
		}, testVocab())
		ix.scenes[0].Date = sceneDate
		return ix
	}

	// Baseline: no date evidence on either side.
	base := newIndex("")
	baseBest := base.Match(Subtitle{Stem: "Some Unique Title Words", Runtime: mins(20, 0)}, 5).Best()
	if baseBest == nil {
		t.Fatal("no baseline candidate")
	}
	if r := dateReason(t, baseBest); r != "" {
		t.Fatalf("baseline carries a date reason: %q", r)
	}
	baseScore := baseBest.Score

	for _, tc := range []struct {
		name       string
		subDate    string
		sceneDate  string
		wantReason string
		wantDelta  float64
	}{
		{"exact match", "2024-05-10", "2024-05-10", "date 2024-05-10", 25},
		{"1 day skew tolerated", "2024-05-11", "2024-05-10", "date 2024-05-11", 25},
		{"2 day skew tolerated", "2024-05-12", "2024-05-10", "date 2024-05-12", 25},
		{"3 day skew is a mismatch", "2024-05-13", "2024-05-10", "date mismatch 2024-05-13 vs 2024-05-10", -40},
		{"scene has no date", "2024-05-10", "", "", 0},
		{"subtitle has no date", "", "2024-05-10", "", 0},
		{"scene date unparseable is treated as absent", "2024-05-10", "not-a-date", "", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ix := newIndex(tc.sceneDate)
			m := ix.Match(Subtitle{
				Stem:    "Some Unique Title Words",
				Runtime: mins(20, 0),
				Date:    tc.subDate,
			}, 5)
			best := m.Best()
			if best == nil {
				t.Fatal("no candidate")
			}
			if r := dateReason(t, best); r != tc.wantReason {
				t.Errorf("reason = %q, want %q", r, tc.wantReason)
			}
			if delta := best.Score - baseScore; delta != tc.wantDelta {
				t.Errorf("score delta = %v, want %v (score=%v base=%v)", delta, tc.wantDelta, best.Score, baseScore)
			}
		})
	}
}

// Same title, same runtime, two candidates differing only by date: the one
// agreeing with the caller's date must win decisively rather than leaving the
// pair Ambiguous, which is the whole point of asking for a date at all.
func TestMatch_DateSeparatesTiedCandidates(t *testing.T) {
	ix := NewIndex([]SceneRef{
		{ID: "1", Stem: "scene-file-001", Title: "Two Sisters Share The Prize",
			Path: "/data/x/1.mp4", Duration: mins(20, 0), Date: "2024-03-10"},
		{ID: "2", Stem: "scene-file-002", Title: "Two Sisters Share The Prize",
			Path: "/data/x/2.mp4", Duration: mins(20, 2), Date: "2024-06-01"},
	}, testVocab())

	m := ix.Match(Subtitle{
		Stem:    "Two Sisters Share The Prize",
		Runtime: mins(20, 1),
		Date:    "2024-03-10",
	}, 5)

	best := m.Best()
	if best == nil || best.Scene.ID != "1" {
		t.Fatalf("best = %+v, want scene 1 (matches the caller's date)", best)
	}
	if len(m.Candidates) < 2 {
		t.Fatalf("want both candidates scored, got %+v", m.Candidates)
	}
	if gap := m.Candidates[0].Score - m.Candidates[1].Score; gap < 40 {
		t.Errorf("gap = %v, want >= 40 (candidates %+v)", gap, m.Candidates)
	}
	if m.Verdict == Ambiguous {
		t.Errorf("verdict = %s, want a decisive verdict once the date breaks the tie", m.Verdict)
	}
}

// A date mismatch is real evidence against a pairing, not disqualifying
// evidence: it must cap an otherwise-Confirmed verdict at Likely rather than
// silently overriding a hard identification like an exact filename match.
func TestMatch_DateMismatchCapsAtLikely(t *testing.T) {
	ix := NewIndex([]SceneRef{
		{ID: "1", Stem: "exact-match-stem", Title: "Some Title",
			Path: "/data/x/1.mp4", Duration: mins(15, 0), Date: "2024-01-01"},
	}, testVocab())

	m := ix.Match(Subtitle{
		Stem:    "exact-match-stem",
		Runtime: mins(15, 1),
		Date:    "2025-01-01",
	}, 5)

	best := m.Best()
	if best == nil {
		t.Fatal("no candidate")
	}
	if !hasReason(*best, "date mismatch") {
		t.Fatalf("reasons = %v, want a date mismatch", best.Reasons)
	}
	if !hasReason(*best, "filename match") {
		t.Fatalf("reasons = %v, want the exact filename match that would otherwise confirm", best.Reasons)
	}
	if m.Verdict != Likely {
		t.Errorf("verdict = %s, want LIKELY (a date mismatch must cap an otherwise-Confirmed verdict)", m.Verdict)
	}
}
