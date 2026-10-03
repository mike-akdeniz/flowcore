package app

import (
	"github.com/google/uuid"
	"github.com/mike-akdeniz/flowcore"
)

// The two seeded workflows.
//
// Each agent step carries its instructions on the step itself, in FlowCore, so a
// run keeps the ones it started under and an edit reaches only new runs. They
// are neutral text: FlowCore does not know a model will read them.
//
// Every step in the claim workflow demonstrates something structural that no
// other step does — that was the test the owner set, and decision 17 records the
// two steps it removed. Assignee strings are opaque to FlowCore: `agent:triage`
// and `group:adjusters` are meaningful only to CaseWork, which is what makes
// an agent an ordinary actor rather than a special case.

func claimAssessmentDefinition() flowcore.WorkflowDefinition {
	var (
		inAssessment = uuid.Must(uuid.NewV7())
		settled      = uuid.Must(uuid.NewV7())
		declined     = uuid.Must(uuid.NewV7())

		triage        = uuid.Must(uuid.NewV7())
		fastTrack     = uuid.Must(uuid.NewV7())
		documentation = uuid.Must(uuid.NewV7())
		awaiting      = uuid.Must(uuid.NewV7())
		consistency   = uuid.Must(uuid.NewV7())
		adjuster      = uuid.Must(uuid.NewV7())
		fraud         = uuid.Must(uuid.NewV7())
	)

	return flowcore.WorkflowDefinition{
		Name:                    "Claim assessment",
		InitialStepDefinitionID: &triage,
		Statuses: []flowcore.WorkflowStatusDefinition{
			{ID: inAssessment, Name: "in assessment"},
			{ID: settled, Name: "settled"},
			{ID: declined, Name: "declined"},
		},
		Steps: []flowcore.StepDefinition{
			{
				// An AI entry step that branches. The first thing a visitor sees
				// happen after submitting is this deciding which path the claim takes.
				ID: triage, WorkflowStatusDefinitionID: inAssessment,
				Name: "triage", AssigneeID: "agent:triage",
				Instructions: stepInstructions(`Decide whether this claim can take the fast track or needs full assessment.

Read the intake note against the claimant's account.`),
				Actions: []flowcore.ActionDefinition{
					{Name: "fast track", NextStepDefinitionID: &fastTrack},
					{Name: "full assessment", NextStepDefinitionID: &documentation},
				},
			},
			{
				// One side of the branch, with a cross-over out of it.
				ID: fastTrack, WorkflowStatusDefinitionID: inAssessment,
				Name: "fast-track review", AssigneeID: "group:claims-adjusters",
				Instructions: stepInstructions("Settle the claim if the quote fits the damage described, " +
					"escalate it if anything needs a closer look, or decline it if the policy does not cover it."),
				Actions: []flowcore.ActionDefinition{
					{Name: "settle", TerminalWorkflowStatusDefinitionID: &settled},
					{Name: "escalate", NextStepDefinitionID: &documentation},
					{Name: "decline", TerminalWorkflowStatusDefinitionID: &declined},
				},
			},
			{
				// An AI step whose judgment loops back for a better document. It
				// judges whether the estimate is adequate, never whether one exists:
				// its required types are on the case before it can be reached.
				ID: documentation, WorkflowStatusDefinitionID: inAssessment,
				Name: "estimate check", AssigneeID: "agent:estimates",
				Instructions: stepInstructions(`Decide whether the repair estimate can be assessed as it stands.

It is adequate when an assessor could check it line by line: parts, labour, hours, rate
and VAT all stated. It needs detail when it is a single approximate figure or leaves any
of those out.`),
				Actions: []flowcore.ActionDefinition{
					{Name: "adequate", NextStepDefinitionID: &consistency},
					{Name: "needs detail", NextStepDefinitionID: &awaiting},
				},
			},
			{
				// The loop target: an AI step re-run against a genuinely different file.
				ID: awaiting, WorkflowStatusDefinitionID: inAssessment,
				Name: "estimate follow-up", AssigneeID: "group:intake-handlers",
				Instructions: stepInstructions("Get an itemised estimate from the repairer, add it to the case, " +
					"then resubmit the claim to the estimate check."),
				Actions: []flowcore.ActionDefinition{
					{Name: "resubmit", NextStepDefinitionID: &documentation},
				},
			},
			{
				// An AI step that diverts to a specialist.
				ID: consistency, WorkflowStatusDefinitionID: inAssessment,
				Name: "narrative consistency", AssigneeID: "agent:fraud",
				Instructions: stepInstructions(`Compare the claimant's account with the independent documents on file.

It is consistent when the police report and any witness statement agree with the
account on where, when and how the damage happened. It is inconsistent when any of
them contradicts the account on one of those.`),
				Actions: []flowcore.ActionDefinition{
					{Name: "consistent", NextStepDefinitionID: &adjuster},
					{Name: "inconsistent", NextStepDefinitionID: &fraud},
				},
			},
			{
				// The main human decision, with the cross-over back.
				ID: adjuster, WorkflowStatusDefinitionID: inAssessment,
				Name: "adjuster review", AssigneeID: "group:claims-adjusters",
				Instructions: stepInstructions("Settle the claim if the documents support the amount, " +
					"downgrade it to fast-track review if it turns out simple, or decline it if they do not."),
				Actions: []flowcore.ActionDefinition{
					{Name: "settle", TerminalWorkflowStatusDefinitionID: &settled},
					{Name: "downgrade", NextStepDefinitionID: &fastTrack},
					{Name: "decline", TerminalWorkflowStatusDefinitionID: &declined},
				},
			},
			{
				// A side branch that rejoins the main path or ends the run.
				ID: fraud, WorkflowStatusDefinitionID: inAssessment,
				Name: "fraud referral", AssigneeID: "group:fraud-investigators",
				Instructions: stepInstructions("Investigate the inconsistency in the history, then clear the claim " +
					"back to the adjuster or confirm the fraud to decline it."),
				Actions: []flowcore.ActionDefinition{
					{Name: "cleared", NextStepDefinitionID: &adjuster},
					{Name: "confirmed", TerminalWorkflowStatusDefinitionID: &declined},
				},
			},
		},
	}
}

// underwritingDefinition is the simple one: an AI entry step and two approvers.
// Its job is to be short, and a plain referral is worth demonstrating once.
func underwritingDefinition() flowcore.WorkflowDefinition {
	var (
		inUnderwriting = uuid.Must(uuid.NewV7())
		accepted       = uuid.Must(uuid.NewV7())
		declined       = uuid.Must(uuid.NewV7())

		riskScreen = uuid.Must(uuid.NewV7())
		underwrite = uuid.Must(uuid.NewV7())
		senior     = uuid.Must(uuid.NewV7())
	)

	return flowcore.WorkflowDefinition{
		Name:                    "Policy assessment",
		InitialStepDefinitionID: &riskScreen,
		Statuses: []flowcore.WorkflowStatusDefinition{
			{ID: inUnderwriting, Name: "in underwriting"},
			{ID: accepted, Name: "accepted"},
			{ID: declined, Name: "declined"},
		},
		Steps: []flowcore.StepDefinition{
			{
				ID: riskScreen, WorkflowStatusDefinitionID: inUnderwriting,
				Name: "risk screen", AssigneeID: "agent:risk",
				Instructions: stepInstructions(`Screen this policy application.

It is standard when the disclosures and the previous insurer's letter describe an
ordinary risk and agree with each other. Refer it when there are convictions, declined
cover, anything undisclosed, or anything that contradicts the disclosures.`),
				Actions: []flowcore.ActionDefinition{
					{Name: "standard", NextStepDefinitionID: &underwrite},
					{Name: "refer", NextStepDefinitionID: &senior},
				},
			},
			{
				ID: underwrite, WorkflowStatusDefinitionID: inUnderwriting,
				Name: "underwriter review", AssigneeID: "group:underwriters",
				Instructions: stepInstructions("Accept the application if the risk is ordinary, " +
					"refer it up if you are unsure, or decline it."),
				Actions: []flowcore.ActionDefinition{
					{Name: "accept", TerminalWorkflowStatusDefinitionID: &accepted},
					{Name: "refer up", NextStepDefinitionID: &senior},
					{Name: "decline", TerminalWorkflowStatusDefinitionID: &declined},
				},
			},
			{
				ID: senior, WorkflowStatusDefinitionID: inUnderwriting,
				Name: "senior underwriter", AssigneeID: "group:senior-underwriters",
				Instructions: stepInstructions("Accept the referred application if the risk can be carried, " +
					"or decline it."),
				Actions: []flowcore.ActionDefinition{
					{Name: "accept", TerminalWorkflowStatusDefinitionID: &accepted},
					{Name: "decline", TerminalWorkflowStatusDefinitionID: &declined},
				},
			},
		},
	}
}

func stepInstructions(text string) *string { return &text }
