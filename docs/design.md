# How the matcher decides

Every rule here was measured against real libraries rather than reasoned from
first principles. This document holds that evidence so the code can stay short.

## The problem

A media manager attaches a caption only when the file sits beside the video and
is named `<stem>.srt` or `<stem>.<lang>.srt`. Subtitles downloaded under a
scraper's original filename — or orphaned when the video moved — are never seen.

Reconnecting them is not string comparison. Real pairs:

```
StudioHandleXXX_Mommy-and-Son-Nesting-Season+(transcribed+on+25-Jul-2024).srt
  → 2023-05-23_Some.Performer-stepMommy.and.stepSon.Nesting.Season_1080p.mp4

thehousenextdoor2024 - The Reluctant Dog Sitter - Compressed.srt
  → The-Reluctant-Pet-Sitter-Part-1.mp4
```

## Scoring

Additive across independent signals, so two weak agreements can outweigh one
strong-looking name match:

| signal | contribution |
|---|---|
| studio / JAV code | +60 — near-decisive when present |
| exact stem match | +100 — already named after the video |
| title similarity | +0…70 (token F1) |
| runtime agreement | +0…45; a contradiction subtracts 30 |
| matching date (±2 days) | +25; a wider disagreement subtracts 40 |

## Runtime is the decisive signal

Two scenes can share a title, a performer and a studio. Their runtimes differ.

In one library the correct match landed within **a couple of seconds** while a
same-titled decoy was **23 minutes** away. No amount of filename similarity
separates those; runtime does it immediately.

The whole stream is scanned rather than just the tail, because cues are not
always stored chronologically and a trailing credit line can carry a timestamp
earlier than the true end.

## Date is a discriminator, not just a bonus

Studios title lazily: the same title, the same performers and a duration
within seconds of each other recur across a catalog, so name and runtime
agreement alone will eventually produce a false positive at scale. The scene
date is what breaks that tie when it is available.

`Subtitle.Date` (YYYY-MM-DD) is optional. Most libraries carry no date in
their filenames at all — `subDate(sub.Stem)` only ever fires for one dated
naming convention — so a caller with a date from anywhere else (a media
manager, a fingerprint database) should set the field explicitly rather than
rely on the filename fallback.

Agreement within **2 days** scores the same +25 as before; anything wider
scores **−40**, enough to outrank a runtime coincidence (+45 max) but not a
hard identification (a code at +60 or an exact filename at +100). The
tolerance exists because scrapers and studios routinely disagree by a day or
two on release date versus publish date — that is noise, not evidence.

A date disagreement is real evidence against a pairing, but not proof against
one: `decide()` caps a candidate carrying a date mismatch at `Likely`, never
`Confirmed`, even when every other signal (including an exact filename) would
otherwise confirm it outright. An unparseable or missing date on either side
is treated as absent, never as a mismatch — garbage input must not penalise a
candidate that simply lacks the evidence.

## Title and filename are scored separately

They are deliberately **not** merged. Libraries carry the identifying
information in one place or the other, not both:

- In a measured **61k-scene** library, **52% of scenes had no title at all** and
  relied entirely on a structured filename.
- A Stash-Box tagged library is the reverse: good titles beside filenames like
  `1080p.wmv`.

Merging them means a junk filename dilutes a good title, and a long filename
drags similarity down through sheer token count. Taking the better of the two
lets each convention work on its own terms.

## The creator vocabulary

Names of performers and studios are built into a vocabulary from the library
itself, replacing a hand-maintained handle list that could only grow. It also
keeps personal names out of committed source.

**Creator words are not discarded.** Some aliases are ordinary words — real
examples include *Molly*, *Tamara*, *Vanity* — and dropping them would destroy
genuine title words like "Molly Catches Her Step-Son". Instead every name splits
into two token sets, applied identically to subtitles and scenes:

| set | meaning | use |
|---|---|---|
| content | what the scene is called | title similarity |
| creator | who made or appears in it | a separate, bounded signal |

That split stops a creator handle inflating similarity between two unrelated
clips by the same creator, without throwing the information away.

### Only squashed handles, never individual words

The vocabulary holds whole names with separators removed — `riverstone`,
`thehousenextdoor` — as scrapers write them.

This restriction was found the hard way. Taking every word from every alias in a
real **4,298-performer** library produced **~9,500 creator words**, which swept
up ordinary vocabulary — *sinful*, *aunt*, *slut* all appear in someone's alias
list — and gutted the content tokens title matching depends on. Measured against
the live library, it pushed a correct candidate (runtime +1s) **out of the top
three entirely**.

Squashed handles are what actually needed suppressing, and they are inherently
distinctive, so they carry the benefit without the collateral damage.

### Length thresholds

A multi-word name is distinctive once it is long enough; a single-word one has
to be longer still, or a short common word (*molly*, *vanity*) captures every
token starting with it.

| name | minimum squashed length |
|---|---|
| multi-word | 6 |
| single word | 8 |

Scrapers concatenate handles and bolt suffixes on (`riverstonexxx`,
`thehousenextdoor2024`), so a token also counts as a creator when it *starts*
with a known handle. Only prefixes match, so a handle cannot match mid-word.

## Folder names

People file subtitles under a performer or studio folder, which names the
creator even when the filename does not. Those folder names contribute
**creator evidence only, never title evidence** — otherwise a directory name
would inflate title similarity for everything inside it.

## Cost control

Tokens appearing in more scenes than `tokenDFCap` (4000) are ignored during
lookup. Words like *mom* carry no discriminating power in a library where
thousands of titles contain them, and scanning their postings dominates the
cost.

## Verdicts

| verdict | meaning |
|---|---|
| `Confirmed` | name and runtime agree, no rival comes close |
| `Likely` | good evidence, weaker on one axis |
| `Ambiguous` | several candidates too close to separate safely |
| `Unmatched` | nothing credible in the library |

`Ambiguous` is load-bearing. It exists so a caller can decline to act rather
than guess — moving a subtitle onto the wrong video is worse than leaving it
where it is. A boolean API would have forced a coin flip.

The verdict also requires real name agreement rather than trusting a score that
runtime alone could have inflated, which is why `Candidate.NameSim` is kept
separately from `Score`.
