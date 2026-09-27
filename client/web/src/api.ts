// CaseWork's API client.
//
// Every call returns something already composed for a screen — a queue, a case —
// rather than library resources the browser would have to assemble. That is
// deliberate: domain rules live in Go, where a shortcut here cannot bypass them.

export type Staff = {
  reference: string;
  name: string;
  title: string;
  groups: string[];
};

export type Session = {
  signedInAs: Staff | null;
  roster: Staff[];
  agentMode: string;
};

export type QueueItem = {
  reference: string;
  type: "claim" | "application";
  stepName: string;
  assignee: string;
  waitingSince: string;
  isDraft: boolean;
};

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const response = await fetch(path, {
    ...init,
    headers: { "Content-Type": "application/json", ...(init?.headers ?? {}) },
  });

  if (!response.ok) {
    throw new Error((await response.text()) || response.statusText);
  }

  return (await response.json()) as T;
}

export const api = {
  session: () => request<Session>("/api/session"),
  signIn: (reference: string) =>
    request<Staff>("/api/session", {
      method: "POST",
      body: JSON.stringify({ reference }),
    }),
  queue: () => request<QueueItem[]>("/api/queue"),
  workflows: () => request<WorkflowSummary[]>("/api/workflows"),
  workflow: (definitionId: string) =>
    request<Workflow>(`/api/workflows/${definitionId}`),

  case: (reference: string) => request<Case>(`/api/cases/${reference}`),
  createCase: (body: NewSubmission) =>
    request<{ reference: string }>("/api/cases", {
      method: "POST",
      body: JSON.stringify(body),
    }),
  // Both of these return the whole case rather than the thing they created, so
  // the screen never has to guess what changed. Adding a document moves the
  // revision, which changes which documents are current — a response carrying
  // only the new row would leave the caller to work that out.
  addDocument: (reference: string, body: NewDocument) =>
    request<Case>(`/api/cases/${reference}/documents`, {
      method: "POST",
      body: JSON.stringify(body),
    }),
  submitCase: (reference: string) =>
    request<Case>(`/api/cases/${reference}/submit`, { method: "POST" }),
  samples: (submissionType?: string) =>
    request<Sample[]>(
      submissionType ? `/api/samples?type=${submissionType}` : "/api/samples",
    ),

  decide: (reference: string, body: Decision) =>
    request<Case>(`/api/cases/${reference}/decide`, {
      method: "POST",
      body: JSON.stringify(body),
    }),
  reassign: (reference: string, visitId: string, assignee: string) =>
    request<Case>(`/api/cases/${reference}/reassign`, {
      method: "POST",
      body: JSON.stringify({ visitId, assignee }),
    }),
  assignees: () => request<Assignee[]>("/api/assignees"),
};

export type Assignee = {
  reference: string;
  label: string;
  kind: "person" | "team";
};

export type Decision = {
  visitId: string;
  actionId: string;
  remark: string;
};

export type Visit = {
  stepName: string;
  assignee: string;
  isAgent: boolean;
  enteredAt: string;
  completedAt: string | null;
  completedBy: string | null;
  actionName: string | null;
  remark: string | null;
  // The revision this decision was made against, and the documents in force at
  // it. Both null and empty on an open visit, and on one whose version token
  // CaseWork did not write.
  revision: number | null;
  documentIds: string[];
};

export type CaseDocument = {
  id: string;
  name: string;
  kind: string;
  receivedAt: string;
  body: string | null;
  sourceFile: string | null;
  addedAtRevision: number;
  // Superseded means a newer document of the same kind has taken over. The row
  // stays on the case: an agent's remark refers to the document it actually
  // read, and hiding that document would leave the remark looking wrong.
  superseded: boolean;
};

export type CaseStep = {
  name: string;
  assignee: string;
  isAgent: boolean;
  waitingSince: string;
  visitId: string;
  actions: { id: string; name: string }[];
};

export type Case = {
  reference: string;
  type: "claim" | "application";
  status: "draft" | "submitted";
  workflowName: string;
  runStatus: string;
  submittedAt: string | null;
  // Revision is what the current documents are current as of, and what the next
  // completion will stamp on its visit.
  revision: number;
  claim: {
    policyNumber: string;
    claimantName: string;
    amount: string;
    occurredAt: string;
    incidentNarrative: string;
  } | null;
  application: {
    proposerName: string;
    coverType: string;
    sumInsured: string;
    disclosures: string;
  } | null;
  documents: CaseDocument[];
  // Null on a draft, and null again once the run has finished.
  currentStep: CaseStep | null;
  // Every visit, oldest first. Empty on a draft.
  history: Visit[];
};

// The two field sets are disjoint, and kept that way deliberately: `type` is the
// discriminator, and nothing reads a field belonging to the other kind.
export type NewClaim = {
  type: "claim";
  policyNumber: string;
  claimantName: string;
  amount: string;
  occurredAt: string;
  incidentNarrative: string;
};

export type NewApplication = {
  type: "application";
  proposerName: string;
  coverType: string;
  sumInsured: string;
  disclosures: string;
};

export type NewSubmission = NewClaim | NewApplication;

// Exactly one of sampleFile, or fileName plus body.
export type NewDocument = {
  sampleFile?: string;
  fileName?: string;
  body?: string;
  kind?: string;
  name?: string;
};

export type Sample = {
  fileName: string;
  title: string;
  kind: string;
  // What the document argues for, read out of its file name. Empty for a file
  // carrying no outcome.
  outcome: string;
  body: string;
};

export type WorkflowSummary = {
  definitionId: string;
  name: string;
  submissionType: "claim" | "application";
  active: boolean;
  stepCount: number;
};

export type WorkflowAction = {
  id: string;
  name: string;
  nextStepId: string | null;
  terminalStatusId: string | null;
};

export type WorkflowStep = {
  id: string;
  name: string;
  assignee: string;
  statusId: string;
  isAgent: boolean;
  actions: WorkflowAction[];
};

export type Workflow = {
  definitionId: string;
  name: string;
  submissionType: "claim" | "application";
  active: boolean;
  entryStepId: string;
  statuses: { id: string; name: string }[];
  steps: WorkflowStep[];
};
