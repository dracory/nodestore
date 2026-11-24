package nodestore

// == INTERFACE ===============================================================

// NodeGroupQueryInterface is the fluent query builder for listing and
// counting node group entities.
type NodeGroupQueryInterface interface {
	Validate() error

	HasID() bool
	GetID() string
	WithID(id string) NodeGroupQueryInterface

	HasWorkflowID() bool
	GetWorkflowID() string
	WithWorkflowID(workflowID string) NodeGroupQueryInterface

	HasLimit() bool
	GetLimit() uint64
	WithLimit(limit uint64) NodeGroupQueryInterface

	HasOffset() bool
	GetOffset() uint64
	WithOffset(offset uint64) NodeGroupQueryInterface

	GetSortOrder() string
	WithSortOrder(sortOrder string) NodeGroupQueryInterface

	IsCountOnly() bool
	WithCountOnly(countOnly bool) NodeGroupQueryInterface
}

// == IMPLEMENTATION ==========================================================

type nodeGroupQueryImplementation struct {
	id            string
	hasID         bool
	workflowID    string
	hasWorkflowID bool
	limit         uint64
	hasLimit      bool
	offset        uint64
	hasOffset     bool
	sortOrder     string
	countOnly     bool
}

// NodeGroupQuery creates a new node group query builder
func NodeGroupQuery() NodeGroupQueryInterface {
	return &nodeGroupQueryImplementation{sortOrder: ASC}
}

func (q *nodeGroupQueryImplementation) Validate() error { return nil }

func (q *nodeGroupQueryImplementation) HasID() bool   { return q.hasID }
func (q *nodeGroupQueryImplementation) GetID() string { return q.id }
func (q *nodeGroupQueryImplementation) WithID(id string) NodeGroupQueryInterface {
	q.id, q.hasID = id, true
	return q
}

func (q *nodeGroupQueryImplementation) HasWorkflowID() bool   { return q.hasWorkflowID }
func (q *nodeGroupQueryImplementation) GetWorkflowID() string { return q.workflowID }
func (q *nodeGroupQueryImplementation) WithWorkflowID(workflowID string) NodeGroupQueryInterface {
	q.workflowID, q.hasWorkflowID = workflowID, true
	return q
}

func (q *nodeGroupQueryImplementation) HasLimit() bool   { return q.hasLimit }
func (q *nodeGroupQueryImplementation) GetLimit() uint64 { return q.limit }
func (q *nodeGroupQueryImplementation) WithLimit(limit uint64) NodeGroupQueryInterface {
	q.limit, q.hasLimit = limit, true
	return q
}

func (q *nodeGroupQueryImplementation) HasOffset() bool   { return q.hasOffset }
func (q *nodeGroupQueryImplementation) GetOffset() uint64 { return q.offset }
func (q *nodeGroupQueryImplementation) WithOffset(offset uint64) NodeGroupQueryInterface {
	q.offset, q.hasOffset = offset, true
	return q
}

func (q *nodeGroupQueryImplementation) GetSortOrder() string { return q.sortOrder }
func (q *nodeGroupQueryImplementation) WithSortOrder(sortOrder string) NodeGroupQueryInterface {
	if sortOrder != DESC {
		sortOrder = ASC
	}
	q.sortOrder = sortOrder
	return q
}

func (q *nodeGroupQueryImplementation) IsCountOnly() bool { return q.countOnly }
func (q *nodeGroupQueryImplementation) WithCountOnly(countOnly bool) NodeGroupQueryInterface {
	q.countOnly = countOnly
	return q
}
