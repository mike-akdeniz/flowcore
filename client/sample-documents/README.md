# Sample documents

Text documents you can add to a case in CaseWork, either by uploading the file or by picking it from
the list in the application — they are embedded in the binary, so a hosted instance offers the same
set.

**The name is the convention.** Each file is `<order>-<kind>-<outcome>.txt`, and the outcome is what
the document argues for:

- `simple` / `complex` — whether a claim can go down the fast track. Read by `triage`.
- `complete` / `incomplete` — whether the file has what an assessor needs. Read by
  `documentation check`.
- `consistent` / `contradicts` — whether it agrees with the claimant's own account. Read by
  `narrative consistency`.

**The number is a suggested order**, and nothing else — the application strips it before reading the
name. It sorts the list so the most useful document is offered first:

| # | File | What it does |
|---|------|--------------|
| 1 | `1-estimate-complete.txt` | The one document the seeded claim is waiting for. Add it at `awaiting documents` and the claim moves on. **Start here.** |
| 2 | `2-police-report-consistent.txt` | Changes the ending: `narrative consistency` sends the claim to the adjuster instead of to fraud. |
| 3 | `3-witness-statement-contradicts.txt` | A second voice against the claimant's account. |
| 4 | `4-witness-statement-consistent.txt` | A second voice for it. |
| 5 | `5-intake-note-simple.txt` | On a claim you create yourself, sends it down the fast track at `triage`. |
| 6 | `6-intake-note-complex.txt` | Already on the seeded claim — the reason it takes the long route. |
| 7 | `7-estimate-incomplete.txt` | Already on the seeded claim — the reason it stalls. |
| 8 | `8-police-report-contradicts.txt` | Already on the seeded claim — the reason it ends in a fraud referral. |

**Adding one never removes another.** A second estimate supersedes the first rather than replacing
it: both stay on the case, the older is labelled, and the agent steps read the newest document of
each kind. That is what keeps an earlier decision's remark pointing at the document it was actually
about. Two documents only compete when they are the same kind, so an estimate and a police report
never displace each other.

**Why the name matters.** With `ANTHROPIC_API_KEY` set, the agent steps read what is *inside* these
files and decide for themselves; the names are then only for your convenience. Without a key there
is no model, so the agent steps are simulated — and they simulate by reading the name. A document
whose name carries no outcome is simulated by choosing at random, and the application says so.

Everything here is fictional.
