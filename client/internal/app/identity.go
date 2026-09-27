package app

// Identity is a person the visitor can act as.
//
// This is the identity half of the boundary. FlowCore has no users, no groups, no
// membership and no authorization: it receives opaque strings and compares them
// for equality, and everything that gives those strings meaning lives here.
//
// The cast itself lives in `casework.staff` and is seeded by the store. It was a
// hard-coded list in this file until one of them survived a change of scenario
// and started feeding the reassignment dropdown a cast that no longer existed —
// two sources for one fact, and the wrong one had the only caller.
//
// A real client would resolve these from a directory or an OIDC token. The shape
// of what it hands FlowCore would be identical.
type Identity struct {
	// Reference is what FlowCore records as completedBy, and one of the values it
	// matches assignee_id against. Opaque: the "user:" prefix is this client's
	// convention and means nothing to the library.
	Reference string
	Name      string
	Title     string
	// Groups are this person's memberships, resolved here and passed to the
	// worklist as extra references to match. Deciding whether Dana is in
	// group:security is exactly the question FlowCore refuses to answer, which is
	// why it takes a set of references rather than a user.
	Groups []string
}

// Label is a human name where there is one, and the raw reference otherwise.
//
// An agent completing a step arrives here as a bare reference with no roster
// entry, which is the point: the library takes a string for completedBy and does
// not care whether a person is behind it.
func (i Identity) Label() string {
	if i.Name != "" {
		return i.Name
	}

	return i.Reference
}

// WorklistReferences is what to ask the worklist about: this person, plus every
// group they belong to.
func (i Identity) WorklistReferences() []string {
	references := make([]string, 0, len(i.Groups)+1)
	references = append(references, i.Reference)

	return append(references, i.Groups...)
}

// CanActAs reports whether this identity would find the given assignee in its own
// worklist.
//
// The library does not check it and never will — FlowCore records who acted and
// never decides whether they were allowed to, which is precisely what leaves the
// question here. So this is CaseWork's authorization rule rather than a display
// hint: the decide handler refuses a step whose assignee it does not match
// (client decision 23), and reassignment deliberately does not consult it.
func (i Identity) CanActAs(assignee string) bool {
	for _, reference := range i.WorklistReferences() {
		if reference == assignee {
			return true
		}
	}

	return false
}
