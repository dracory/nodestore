package nodestore

// == INTERFACE ===============================================================

// NodeQueryInterface is the fluent query builder for listing and
// counting node entities.
type NodeQueryInterface interface {
	Validate() error

	HasID() bool
	GetID() string
	WithID(id string) NodeQueryInterface

	HasWorkflowID() bool
	GetWorkflowID() string
	WithWorkflowID(workflowID string) NodeQueryInterface

	HasLimit() bool
	GetLimit() uint64
	WithLimit(limit uint64) NodeQueryInterface

	HasOffset() bool
	GetOffset() uint64
	WithOffset(offset uint64) NodeQueryInterface

	GetSortOrder() string
	WithSortOrder(sortOrder string) NodeQueryInterface

	IsCountOnly() bool
	WithCountOnly(countOnly bool) NodeQueryInterface
}

// == IMPLEMENTATION ==========================================================

type nodeQueryImplementation struct {
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

// NodeQuery creates a new node query builder
func NodeQuery() NodeQueryInterface {
	return &nodeQueryImplementation{sortOrder: ASC}
}

func (q *nodeQueryImplementation) Validate() error { return nil }

func (q *nodeQueryImplementation) HasID() bool   { return q.hasID }
func (q *nodeQueryImplementation) GetID() string { return q.id }
func (q *nodeQueryImplementation) WithID(id string) NodeQueryInterface {
	q.id, q.hasID = id, true
	return q
}

func (q *nodeQueryImplementation) HasWorkflowID() bool   { return q.hasWorkflowID }
func (q *nodeQueryImplementation) GetWorkflowID() string { return q.workflowID }
func (q *nodeQueryImplementation) WithWorkflowID(workflowID string) NodeQueryInterface {
	q.workflowID, q.hasWorkflowID = workflowID, true
	return q
}

func (q *nodeQueryImplementation) HasLimit() bool   { return q.hasLimit }
func (q *nodeQueryImplementation) GetLimit() uint64 { return q.limit }
func (q *nodeQueryImplementation) WithLimit(limit uint64) NodeQueryInterface {
	q.limit, q.hasLimit = limit, true
	return q
}

func (q *nodeQueryImplementation) HasOffset() bool   { return q.hasOffset }
func (q *nodeQueryImplementation) GetOffset() uint64 { return q.offset }
func (q *nodeQueryImplementation) WithOffset(offset uint64) NodeQueryInterface {
	q.offset, q.hasOffset = offset, true
	return q
}

func (q *nodeQueryImplementation) GetSortOrder() string { return q.sortOrder }
func (q *nodeQueryImplementation) WithSortOrder(sortOrder string) NodeQueryInterface {
	if sortOrder != DESC {
		sortOrder = ASC
	}
	q.sortOrder = sortOrder
	return q
}

func (q *nodeQueryImplementation) IsCountOnly() bool { return q.countOnly }
func (q *nodeQueryImplementation) WithCountOnly(countOnly bool) NodeQueryInterface {
	q.countOnly = countOnly
	return q
}
