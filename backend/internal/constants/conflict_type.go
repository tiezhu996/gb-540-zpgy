package constants

// ConflictType is the stable vocabulary shared by geometry services and clients.
type ConflictType string

const (
	ConflictOverlap          ConflictType = "overlap"
	ConflictGap              ConflictType = "gap"
	ConflictSelfIntersection ConflictType = "self_intersection"
	ConflictDanglingEdge     ConflictType = "dangling_edge"
)

const (
	ConflictDetected           = "detected"
	ConflictConfirmed          = "confirmed"
	ConflictFalsePositive      = "false_positive"
	ConflictResolutionProposed = "resolution_proposed"
	ConflictResolved           = "resolved"
	ConflictClosed             = "closed"
)

var conflictTransitions = map[string]map[string]bool{
	ConflictDetected:           {ConflictConfirmed: true, ConflictFalsePositive: true},
	ConflictConfirmed:          {ConflictResolutionProposed: true},
	ConflictFalsePositive:      {ConflictClosed: true},
	ConflictResolutionProposed: {ConflictResolved: true},
	ConflictResolved:           {ConflictClosed: true},
	ConflictClosed:             {},
}

func CanConflictTransition(from, to string) bool { return conflictTransitions[from][to] }
