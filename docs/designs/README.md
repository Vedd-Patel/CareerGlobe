# Designs (draft v1, awaiting approval)

The design system and screens for CareerGlobe, made on a Claude Design canvas. Review them
there (zoom, comment, switch Light / Dark in each board's Tweaks), then approve or ask for
changes. **No UI code is written until these are approved.**

| File | What it is |
|---|---|
| `tokens.json` | Colours (light + dark), globe palette, type scale, spacing, radii, elevation, motion, icons. The web CSS, Compose and SwiftUI themes are generated from this. |
| `careerglobe.css` | The same tokens and components as CSS, used by the canvas boards. |
| `canvas/*.dc.html` | One board per screen group (sources of the canvas). |

Boards: design system (tokens, components) · A before signup (phone S02–S07, web S01, S04/S05) ·
B onboarding (S08–S13) · C globe (phone S14–S17, web S14/S15) · D companies and jobs (phone
S18–S22, web S19/S20) · E tracker (phone S23–S25, web S23/S25) · F saved, inbox, profile,
settings (S26–S29) · G admin (S30) · screen states (loading, empty, error, offline, hints).

Phone boards show one shared layout; the iOS app uses Apple's controls (HIG) and the Android
app Material 3 controls on top of it, as the spec asks. Names and numbers in the boards are
examples; counts on S01 and S30 are from the real crawl of 22 Oct 2026.
