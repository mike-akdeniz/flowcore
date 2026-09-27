# Sample documents

Text documents you can add to a case in CaseWork, either by uploading the file or by picking it from
the list in the application — they are embedded in the binary, so a hosted instance offers the same
set.

**The name is the convention.** Each file is `<order>-<document>-<outcome>.txt`, and the outcome is
one of two words:

- `pass` — the step reading this document is satisfied.
- `fail` — it is not.

Two words rather than one pair per step. A claim that passes `triage` takes the fast track and one
that fails needs full assessment; an estimate that passes `documentation check` is itemised and one
that fails is a scribbled figure. What "pass" means is a property of the step, not of the vocabulary.

**The reason is not in the file name.** It is in the finding the agent step records, which is a
sentence about the case rather than a word squeezed into a filename — and, without an API key, a
canned one:

> The report places the vehicle two miles from the address given, already damaged, some hours before
> the claimant says the damage occurred. The account of an overnight impact outside the home cannot
> both be true.
>
> Simulated: this finding is canned, and "inconsistent" was chosen from the file name
> `8-police-report-fail.txt`. No model was consulted and nothing inside the document was read.

The second paragraph is not optional. The first reads like an assessment, and nothing here assessed
anything.

**The number is a suggested order**, and nothing else — the application strips it before reading the
name. Numbering restarts for each kind of submission, because the picker only ever shows one kind at
a time.

## Claims

The longer workflow: seven steps, three of them decided by an agent, and a loop back through
`awaiting documents`. Worth doing second.

| # | File | What it does |
|---|------|--------------|
| 1 | `1-estimate-pass.txt` | The document the seeded claim is waiting for. Add it at `awaiting documents` and the claim moves on. |
| 2 | `2-police-report-pass.txt` | Changes the ending: `narrative consistency` sends the claim to the adjuster instead of to fraud. |
| 3 | `3-witness-statement-fail.txt` | A second voice against the claimant's account. |
| 4 | `4-witness-statement-pass.txt` | A second voice for it. |
| 5 | `5-intake-note-pass.txt` | On a claim you file yourself, sends it down the fast track at `triage`. |
| 6 | `6-intake-note-fail.txt` | Already on the seeded claim — the reason it takes the long route. |
| 7 | `7-estimate-fail.txt` | Already on the seeded claim — the reason it stalls. |
| 8 | `8-police-report-fail.txt` | Already on the seeded claim — the reason it ends in a fraud referral. |

## Policy applications

| # | File | What it does |
|---|------|--------------|
| 1 | `1-prior-insurer-pass.txt` | Supersedes the letter on the seeded application, so `risk screen` passes it to an ordinary underwriter. **Start here** — the policy application is the shorter workflow and the better one to meet first. |
| 2 | `2-inspection-fail.txt` | A vehicle that is not the one proposed. |
| 3 | `3-inspection-pass.txt` | A vehicle that is. |
| 4 | `4-prior-insurer-fail.txt` | Already on the seeded application — the reason it goes to a senior underwriter. |

## How a step finds its document

Each agent step reads one kind of document: `triage` reads the intake note, `documentation check` the
estimate, `narrative consistency` the police report or a witness statement, `risk screen` the
inspection or the previous insurer's letter. Two documents only compete when a step reads both kinds,
and then the newer wins.

This matters because `pass` and `fail` say nothing about *which* step they answer. Without it, a
claim's intake note would decide whether its estimate was adequate.

**Policy applications take documents on the same terms as claims.** A claim and a proposal have
nothing in common at the detail level and run through identical document machinery, which is what the
library being subject-agnostic looks like from the application's side.

**Adding one never removes another.** A second estimate supersedes the first rather than replacing
it: both stay on the case, the older is labelled, and the agent steps read the newest document of
each kind. That is what keeps an earlier decision's finding pointing at the document it was actually
about.

**Why the name matters.** With `ANTHROPIC_API_KEY` set, the agent steps read what is *inside* these
files and decide for themselves; the names are then only for your convenience. Without a key there is
no model, so the agent steps are simulated — and they simulate by reading the name. A document whose
name carries no outcome, or one a step does not read, leaves the step choosing at random, and the
application says so.

Everything here is fictional.
