// What the two kinds of submission are called, in one place.
//
// They had four spellings between them — a badge reading `application`, a toggle
// reading "Policy application", prose reading "new policy applications", and a
// wire value of `"application"` leaking onto the screen. Nothing was wrong
// exactly; it just read as three different things.
//
// The names are deliberately unabbreviated. Someone looking at this is almost
// never an insurer, and "claim" on its own is ambiguous outside the trade —
// "Insurance claim" costs a word and removes the question.
//
// A *workflow* is not named here, and that is not an oversight. "Claim
// assessment" and "Policy assessment" are the names of processes, and which
// process handles which kind of submission is configuration — the whole reason
// the workflow registry exists. The filing form says "This will run: Policy
// assessment" precisely because the two are different things.
export type SubmissionType = "claim" | "application";

const names: Record<SubmissionType, string> = {
  claim: "Insurance claim",
  application: "Policy application",
};

export function submissionName(type: SubmissionType | string): string {
  return names[type as SubmissionType] ?? type;
}

// The plural, for prose about what a workflow applies to.
export function submissionPlural(type: SubmissionType | string): string {
  return type === "claim" ? "insurance claims" : "policy applications";
}
