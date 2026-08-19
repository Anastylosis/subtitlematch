# Usage

```go
import "github.com/Anastylosis/subtitlematch"
```

The package name matches the module. It is deliberately explicit — this sits
beside several Go libraries that parse and retime subtitle *files*, and the
distinguishing job here is deciding which video a subtitle belongs to.

Most callers alias it, which is normal Go and costs one line:

```go
import subs "github.com/Anastylosis/subtitlematch"
```

## Shape of the API

The package is pure: it never touches a media manager, a database or a
filesystem. Callers supply the library listing and act on the verdict.

```
[]SceneRef ──NewIndex──▶ *Index ──Match(Subtitle)──▶ Match{Verdict, Candidates}
```

## Building the index

Preprocess the library once, then match many subtitles against it.

```go
vocab := subs.NewVocab(performerAndStudioNames) // may be nil
index := subs.NewIndex(scenes, vocab)
```

`vocab` may be `nil`, in which case every token counts as content. Matching
still works — it just loses the ability to tell "same creator" from "same
scene". See [design.md](design.md) for why that distinction matters.

Scenes are described by `SceneRef`: the stem, title, studio, performers and
duration your backend already knows.

## Describing a subtitle

```go
sub := subs.Subtitle{
    Path:    p,
    Stem:    stem,                  // basename without extension
    Runtime: runtime,               // zero when unknown
    Lang:    lang,
    Size:    size,                  // breaks ties between duplicate transcripts
    Dirs:    dirs,                  // folders between scan root and file
    Date:    date,                  // YYYY-MM-DD, optional
}
```

Three fields carry more weight than they look:

- **`Runtime`** is the decisive signal. Populate it via `subs.Runtime(r)`, which
  reads the last cue time from an SRT/VTT stream.
- **`Dirs`** contributes creator evidence only. People file subtitles under a
  performer or studio folder, which names the creator even when the filename
  does not.
- **`Date`** is a second discriminator alongside runtime: same title, same
  performers and a runtime within seconds recur across a studio's catalog, and
  the scene date is what tells those apart. Set it when you have one from
  metadata — a media manager, a fingerprint database. When empty, the matcher
  falls back to a YYYY-MM-DD parsed out of `Stem`, which only ever fires for
  the one dated-filename convention this package was originally built against;
  most libraries carry no date in their filenames at all, so callers with a
  metadata date should set the field rather than lean on that fallback. See
  [design.md](design.md) for the tolerance and the verdict cap on disagreement.

## Matching

```go
m := index.Match(sub, 5) // up to 5 ranked candidates

switch m.Verdict {
case subs.Confirmed:
    apply(m.Best())
case subs.Likely:
    apply(m.Best())          // or queue for review
case subs.Ambiguous:
    review(m.Candidates)     // do not guess
case subs.Unmatched:
    skip()
}
```

**Handle `Ambiguous` explicitly.** It exists so you can decline to act — moving
a subtitle onto the wrong video is worse than leaving it alone. Treating it as
"no" is safe; treating it as "yes" is not.

`Match.Best()` returns the top candidate or `nil`. Each `Candidate` carries
`Score`, `Delta` (scene duration minus subtitle runtime), `NameSim` and
`Reasons` — the last being human-readable strings for reports.

## Helpers

| function | purpose |
|---|---|
| `Runtime(r)` | last cue time from an SRT/VTT stream |
| `RuntimeFit(sub, scene)` | agreement score and delta for two durations |
| `DetectLanguage(r)` | ISO-639-1 code from subtitle text |
| `Tokens(s)`, `Codes(s)` | normalised token and studio/JAV code sets |
| `StripNoise(stem)` | drop release-tag noise from a filename stem |
| `F1(a, b)`, `Overlap(a, b)` | set similarity primitives |
| `HMS(d)` | duration as `H:MM:SS`, for reports |

## Who uses it

- **Custodian** matches against a local filesystem, then moves files into place.
- **MoanSubs** matches against a fingerprint database and serves the result over
  an API.

Neither is visible from this package, which is what lets them share it
unchanged.
