# subtitlematch

[![CI](https://github.com/Anastylosis/subtitlematch/actions/workflows/ci.yml/badge.svg)](https://github.com/Anastylosis/subtitlematch/actions/workflows/ci.yml)
[![codecov](https://codecov.io/gh/Anastylosis/subtitlematch/branch/master/graph/badge.svg)](https://codecov.io/gh/Anastylosis/subtitlematch)
[![Go Reference](https://pkg.go.dev/badge/github.com/Anastylosis/subtitlematch.svg)](https://pkg.go.dev/github.com/Anastylosis/subtitlematch)
[![Go Report Card](https://goreportcard.com/badge/github.com/Anastylosis/subtitlematch)](https://goreportcard.com/report/github.com/Anastylosis/subtitlematch)

Matches loose subtitle files to the videos they belong to, when the two names
share almost nothing.

```
StudioHandleXXX_Mommy-and-Son-Nesting-Season+(transcribed+on+25-Jul-2024).srt
  → 2023-05-23_Some.Performer-stepMommy.and.stepSon.Nesting.Season_1080p.mp4
```

Names alone cannot resolve that. The matcher combines studio/JAV codes,
normalised title tokens, and — decisively — the subtitle's own runtime against
the scene duration.

```go
// The package name is long; alias it and call sites stay readable.
import subs "github.com/Anastylosis/subtitlematch"

index := subs.NewIndex(scenes, subs.NewVocab(names))
m := index.Match(sub, 5)
```

It answers with a `Verdict`, not a boolean:

| verdict | meaning |
|---|---|
| `Confirmed` | name and runtime agree, no rival comes close |
| `Likely` | good evidence, weaker on one axis |
| `Ambiguous` | too close to separate safely |
| `Unmatched` | nothing credible in the library |

`Ambiguous` is the point: a caller can decline to act rather than guess, because
moving a subtitle onto the wrong video is worse than leaving it alone.

## Scope

Nothing here touches a media manager, database or filesystem. The input is
filenames, durations and subtitle contents; the output is a verdict per
candidate pair. Callers supply the library listing and act on the result.

That is what lets a local-filesystem consumer and a fingerprint-database
consumer share it unchanged.

## Documentation

- [docs/usage.md](docs/usage.md) — the API, and which fields carry more weight
  than they look
- [docs/design.md](docs/design.md) — why it decides the way it does, and the
  measurements behind each rule

## License

Copyright (C) 2026 Wasylq

GPL-3.0-only.
