# Sample documents

Text documents you can add to a case in CaseWork, either by uploading the file or by picking it from
the list in the application — they are embedded in the binary, so a hosted instance offers the same
set.

**The name says what the document argues for.** Each file is `<order>-<document>-<outcome>.txt`, and
the outcome is one of two words:

- `pass` — the step judging this document should be satisfied.
- `fail` — it should not.

Two words rather than one pair per step. An intake note that passes sends a claim down the fast track
at `triage` and one that fails needs full assessment; an estimate that passes `estimate check` is
itemised and one that fails is a scribbled figure. What "pass" means is a property of the step, not
of the vocabulary.

**The name is a label, not an instruction.** With `ANTHROPIC_API_KEY` set, the agent steps read what
is *inside* these files, along with the rest of the case, and decide for themselves; the outcome in
the name is what the text was written to argue, and the picker shows it so you can choose which way to
push the model. Without a key nothing reads the documents at all: each agent step takes a fixed branch
and its remark says so (client decision 39). The tables below describe what happens with a key.

**The number is a suggested order**, and nothing else — the application strips it before reading the
name. Numbering restarts for each kind of submission, because the picker only ever shows one kind at
a time.

## Claims

The longer workflow: seven steps, three of them decided by an agent, and a loop back through
`estimate follow-up`. Worth doing second.

| # | File | What it does |
|---|------|--------------|
| 1 | `1-estimate-pass.txt` | The document the seeded claim is waiting for. Add it at `estimate follow-up` and the claim moves on. |
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

## Required documents

Each step can require kinds of document, and a decision on it waits until one of each is on the case
(client decision 36). The seeded claim already holds everything its agent steps require, so it can be
submitted as it stands; the samples above are what you add to change a decision or to satisfy a step
you have edited to require more.

**Policy applications take documents on the same terms as claims.** A claim and a proposal have
nothing in common at the detail level and run through identical document machinery, which is what the
library being subject-agnostic looks like from the application's side.

**Adding one never removes another.** A second estimate supersedes the first rather than replacing
it: both stay on the case, the older is labelled, and the agent steps read the newest document of
each kind. That is what keeps an earlier decision's finding pointing at the document it was actually
about.

Everything here is fictional.
