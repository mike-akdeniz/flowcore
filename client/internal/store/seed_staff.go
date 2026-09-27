package store

import "context"

// SeedStaff inserts the cast once, at start-up.
//
// Shared across every session and never copied: what belongs to a visitor is the
// work, not the people. Two visitors both signing in as Dana are the same Dana
// looking at different claims.
//
// Idempotent, so it runs on every boot without a guard.
func (s *Store) SeedStaff(ctx context.Context) error {
	// One group each, and every group has exactly one member, so switching
	// identity always changes the queue in a legible way.
	//
	// Tom used to be in two. It demonstrated nothing FlowCore is responsible for —
	// expanding a person into references is already shown by anyone with one
	// group, since `WorklistReferences` returns the person *and* the group — while
	// raising a question about multi-group membership, which is identity modelling
	// the library deliberately has no opinion on.
	//
	// Losing it also improves the underwriting path: `refer up` is now a handoff
	// between two people rather than one person passing work to themselves.
	cast := []Staff{
		{Reference: "user:ines", Name: "Inés Moreau",
			Groups: []string{"group:intake-handlers"}},
		{Reference: "user:dana", Name: "Dana Whitfield",
			Groups: []string{"group:claims-adjusters"}},
		{Reference: "user:marek", Name: "Marek Sobczak",
			Groups: []string{"group:fraud-investigators"}},
		{Reference: "user:priya", Name: "Priya Raman",
			Groups: []string{"group:underwriters"}},
		{Reference: "user:tom", Name: "Tom Bexley",
			Groups: []string{"group:senior-underwriters"}},
	}

	for order, member := range cast {
		_, err := s.pool.Exec(ctx,
			`insert into casework.staff (reference, name, groups, sort_order)
			 values ($1, $2, $3, $4)
			 on conflict (reference) do update
			 set name = excluded.name, groups = excluded.groups,
			     sort_order = excluded.sort_order`,
			member.Reference, member.Name, member.Groups, order)
		if err != nil {
			return err
		}
	}

	return nil
}
