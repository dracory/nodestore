package nodestore

// == INTERFACE ===============================================================

// CredentialQueryInterface is the fluent query builder for listing and
// counting credential entities.
type CredentialQueryInterface interface {
	Validate() error

	HasID() bool
	GetID() string
	WithID(id string) CredentialQueryInterface

	HasWorkflowID() bool
	GetWorkflowID() string
	WithWorkflowID(workflowID string) CredentialQueryInterface

	HasNodeID() bool
	GetNodeID() string
	WithNodeID(nodeID string) CredentialQueryInterface

	HasLimit() bool
	GetLimit() uint64
	WithLimit(limit uint64) CredentialQueryInterface

	HasOffset() bool
	GetOffset() uint64
	WithOffset(offset uint64) CredentialQueryInterface

	GetSortOrder() string
	WithSortOrder(sortOrder string) CredentialQueryInterface

	IsCountOnly() bool
	WithCountOnly(countOnly bool) CredentialQueryInterface
}

// == IMPLEMENTATION ==========================================================

type credentialQueryImplementation struct {
	id            string
	hasID         bool
	workflowID    string
	hasWorkflowID bool
	nodeID        string
	hasNodeID     bool
	limit         uint64
	hasLimit      bool
	offset        uint64
	hasOffset     bool
	sortOrder     string
	countOnly     bool
}

// CredentialQuery creates a new credential query builder
func CredentialQuery() CredentialQueryInterface {
	return &credentialQueryImplementation{sortOrder: ASC}
}

func (q *credentialQueryImplementation) Validate() error { return nil }

func (q *credentialQueryImplementation) HasID() bool   { return q.hasID }
func (q *credentialQueryImplementation) GetID() string { return q.id }
func (q *credentialQueryImplementation) WithID(id string) CredentialQueryInterface {
	q.id, q.hasID = id, true
	return q
}

func (q *credentialQueryImplementation) HasWorkflowID() bool   { return q.hasWorkflowID }
func (q *credentialQueryImplementation) GetWorkflowID() string { return q.workflowID }
func (q *credentialQueryImplementation) WithWorkflowID(workflowID string) CredentialQueryInterface {
	q.workflowID, q.hasWorkflowID = workflowID, true
	return q
}

func (q *credentialQueryImplementation) HasNodeID() bool   { return q.hasNodeID }
func (q *credentialQueryImplementation) GetNodeID() string { return q.nodeID }
func (q *credentialQueryImplementation) WithNodeID(nodeID string) CredentialQueryInterface {
	q.nodeID, q.hasNodeID = nodeID, true
	return q
}

func (q *credentialQueryImplementation) HasLimit() bool   { return q.hasLimit }
func (q *credentialQueryImplementation) GetLimit() uint64 { return q.limit }
func (q *credentialQueryImplementation) WithLimit(limit uint64) CredentialQueryInterface {
	q.limit, q.hasLimit = limit, true
	return q
}

func (q *credentialQueryImplementation) HasOffset() bool   { return q.hasOffset }
func (q *credentialQueryImplementation) GetOffset() uint64 { return q.offset }
func (q *credentialQueryImplementation) WithOffset(offset uint64) CredentialQueryInterface {
	q.offset, q.hasOffset = offset, true
	return q
}

func (q *credentialQueryImplementation) GetSortOrder() string { return q.sortOrder }
func (q *credentialQueryImplementation) WithSortOrder(sortOrder string) CredentialQueryInterface {
	if sortOrder != DESC {
		sortOrder = ASC
	}
	q.sortOrder = sortOrder
	return q
}

func (q *credentialQueryImplementation) IsCountOnly() bool { return q.countOnly }
func (q *credentialQueryImplementation) WithCountOnly(countOnly bool) CredentialQueryInterface {
	q.countOnly = countOnly
	return q
}
