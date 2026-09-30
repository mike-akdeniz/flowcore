// CaseWork's API client.
//
// Every call returns something already composed for a screen — a queue, a case —
// rather than library resources the browser would have to assemble. That is
// deliberate: domain rules live in Go, where a shortcut here cannot bypass them.

export type Staff = {
  reference: string;
  name: string;
  // groups is what the interface matches against a step's assignee; teams is the
  // same thing spelled for a person to read, and is what gets displayed.
  //
  // There was a job title here too. It described the same person in words nothing
  // matched on, next to a group saying almost the same thing.
  groups: string[];
  teams: string;
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
  // Only ever true in the all-cases list. A worklist has nothing finished in it
  // by definition: nothing is waiting on anyone.
  finished: boolean;
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

function edit<T>(path: string, method: string, body?: unknown): Promise<T> {
  return request<T>(path, {
    method,
    ...(body === undefined ? {} : { body: JSON.stringify(body) }),
  });
}

export const api = {
  session: () => request<Session>("/api/session"),
  signIn: (reference: string) =>
    request<Staff>("/api/session", {
      method: "POST",
      body: JSON.stringify({ reference }),
    }),
  queue: () => request<QueueItem[]>("/api/queue"),
  allCases: () => request<QueueItem[]>("/api/cases"),
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
  reopenCase: (reference: string) =>
    request<Case>(`/api/cases/${reference}/reopen`, { method: "POST" }),
  removeDocument: (reference: string, documentId: string) =>
    request<Case>(`/api/cases/${reference}/documents/${documentId}`, {
      method: "DELETE",
    }),
  samples: () => request<Sample[]>("/api/samples"),

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

  // Editing. Every one of these answers with the whole workflow, because one
  // edit moves several things at once — adding an action can clear a concern,
  // deleting a step can strand two others.
  createWorkflow: (body: NewWorkflow) =>
    request<Workflow>("/api/workflows", {
      method: "POST",
      body: JSON.stringify(body),
    }),
  renameWorkflow: (id: string, name: string) =>
    edit<Workflow>(`/api/workflows/${id}`, "PATCH", { name }),
  activateWorkflow: (id: string, submissionType: string) =>
    edit<Workflow>(`/api/workflows/${id}/activate`, "POST", { submissionType }),
  addStatus: (id: string, name: string) =>
    edit<Workflow>(`/api/workflows/${id}/statuses`, "POST", { name }),
  renameStatus: (id: string, statusId: string, name: string) =>
    edit<Workflow>(`/api/workflows/${id}/statuses/${statusId}`, "PATCH", { name }),
  deleteStatus: (id: string, statusId: string) =>
    edit<Workflow>(`/api/workflows/${id}/statuses/${statusId}`, "DELETE"),
  addStep: (id: string, body: StepEdit) =>
    edit<Workflow>(`/api/workflows/${id}/steps`, "POST", body),
  updateStep: (id: string, stepId: string, body: StepEdit) =>
    edit<Workflow>(`/api/workflows/${id}/steps/${stepId}`, "PATCH", body),
  deleteStep: (id: string, stepId: string) =>
    edit<Workflow>(`/api/workflows/${id}/steps/${stepId}`, "DELETE"),
  setEntryStep: (id: string, stepId: string) =>
    edit<Workflow>(`/api/workflows/${id}/steps/${stepId}/entry`, "POST"),
  addAction: (
    id: string,
    stepId: string,
    body: { name: string; nextStepId?: string; terminalStatusId?: string },
  ) => edit<Workflow>(`/api/workflows/${id}/steps/${stepId}/actions`, "POST", body),
  renameAction: (id: string, actionId: string, name: string) =>
    edit<Workflow>(`/api/workflows/${id}/actions/${actionId}`, "PATCH", { name }),
  deleteAction: (id: string, actionId: string) =>
    edit<Workflow>(`/api/workflows/${id}/actions/${actionId}`, "DELETE"),

  documentTypes: () => request<DocumentType[]>("/api/document-types"),
  // Created allowed on a kind of case, so a step of a workflow for it can
  // require the new type straight away.
  createDocumentType: (name: string, title: string, submissionType: string) =>
    request<DocumentType>("/api/document-types", {
      method: "POST",
      body: JSON.stringify({ name, title, submissionType }),
    }),
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
  // The run this visit belongs to. A case reopened after finishing has more than
  // one, and the timeline marks where each begins.
  runId: string;
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
  // What a sample's name argues for; empty for an uploaded file. Lets the
  // interface name a document the way the picker does.
  outcome: string;
  addedAtRevision: number;
  // This document's ordinal among its own kind, oldest first — "Repair estimate
  // v2". Per kind, not per case: addedAtRevision is a case-level number.
  version: number;
  // The decisions that had this document on file. Empty means no decision has
  // seen it. Removal also requires the case to be draft.
  readBy: string[];
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
  // Every kind this session knows about, for the upload selector.
  documentTypes: DocumentType[];
  // What the step this case is waiting on has been configured to read. Empty
  // means it declares nothing, and the picker narrows nothing.
  expects: string[];
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

export type DocumentType = {
  name: string;
  title: string;
  // The kinds of case that may hold this type. Present on the list, absent on
  // a type just created.
  allowedFor?: string[];
};

export type Sample = {
  fileName: string;
  // The document type's name. Its title comes from the case's documentTypes, so
  // renaming a type relabels every sample of it.
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
  // The document types this step reads, by name.
  expects: string[];
  actions: WorkflowAction[];
};

// Something wrong with the graph's shape. stepId is empty when it is about the
// workflow rather than one step. Warnings, never refusals — see client decision
// 29.
export type Concern = {
  stepId: string;
  message: string;
};

export type Workflow = {
  definitionId: string;
  name: string;
  submissionType: "claim" | "application";
  active: boolean;
  entryStepId: string;
  statuses: { id: string; name: string }[];
  steps: WorkflowStep[];
  concerns: Concern[];
  // How many cases are part-way through this workflow. They keep the version
  // they started on — saying so is the only way that guarantee is visible.
  runningCases: number;
};

export type StepEdit = {
  name: string;
  assignee: string;
  statusId: string;
  // The whole set, not a delta.
  expects: string[];
};

export type NewWorkflow = {
  name: string;
  submissionType: "claim" | "application";
  statusName: string;
  stepName: string;
  assignee: string;
};
