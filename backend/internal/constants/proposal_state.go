package constants

// ProposalState describes the internal review lifecycle; accepted never means legal registration.
type ProposalState string

const (
	ProposalDraft     ProposalState = "draft"
	ProposalValidated ProposalState = "validated"
	ProposalSubmitted ProposalState = "submitted"
	ProposalReviewed  ProposalState = "reviewed"
	ProposalRevision  ProposalState = "revision"
	ProposalAccepted  ProposalState = "accepted"
	ProposalRejected  ProposalState = "rejected"
)

var proposalTransitions = map[ProposalState]map[ProposalState]bool{
	ProposalDraft:     {ProposalValidated: true},
	ProposalValidated: {ProposalSubmitted: true},
	ProposalSubmitted: {ProposalReviewed: true},
	ProposalReviewed:  {ProposalAccepted: true, ProposalRejected: true, ProposalRevision: true},
	ProposalRevision:  {ProposalDraft: true},
	ProposalAccepted:  {}, ProposalRejected: {},
}

func (s ProposalState) Valid() bool                     { _, ok := proposalTransitions[s]; return ok }
func CanProposalTransition(from, to ProposalState) bool { return proposalTransitions[from][to] }
