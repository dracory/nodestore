package nodestore

// == INTERFACE ===============================================================

// WorkflowGroupQueryInterface is the fluent query builder for listing
// and counting workflow group entities.
type WorkflowGroupQueryInterface interface {
	Validate() error

	HasID() bool
	GetID() string
	WithID(id string) WorkflowGroupQueryInterface

	HasParentID() bool
	GetParentID() string
	WithParentID(parentID string) WorkflowGroupQueryInterface

	HasLimit() bool
	GetLimit() uint64
	WithLimit(limit uint64) WorkflowGroupQueryInterface

	HasOffset() bool
	GetOffset() uint64
	WithOffset(offset uint64) WorkflowGroupQueryInterface

	GetSortOrder() string
	WithSortOrder(sortOrder string) WorkflowGroupQueryInterface

	IsCountOnly() bool
	WithCountOnly(countOnly bool) WorkflowGroupQueryInterface
}

// == IMPLEMENTATION ==========================================================

type workflowGroupQueryImplementation struct {
	id          string
	hasID       bool
	parentID    string
	hasParentID bool
	limit       uint64
	hasLimit    bool
	offset      uint64
	hasOffset   bool
	sortOrder   string
	countOnly   bool
}

// WorkflowGroupQuery creates a new workflow group query builder
func WorkflowGroupQuery() WorkflowGroupQueryInterface {
	return &workflowGroupQueryImplementation{sortOrder: ASC}
}

func (q *workflowGroupQueryImplementation) Validate() error { return nil }

func (q *workflowGroupQueryImplementation) HasID() bool   { return q.hasID }
func (q *workflowGroupQueryImplementation) GetID() string { return q.id }
func (q *workflowGroupQueryImplementation) WithID(id string) WorkflowGroupQueryInterface {
	q.id, q.hasID = id, true
	return q
}

func (q *workflowGroupQueryImplementation) HasParentID() bool   { return q.hasParentID }
func (q *workflowGroupQueryImplementation) GetParentID() string { return q.parentID }
func (q *workflowGroupQueryImplementation) WithParentID(parentID string) WorkflowGroupQueryInterface {
	q.parentID, q.hasParentID = parentID, true
	return q
}

func (q *workflowGroupQueryImplementation) HasLimit() bool   { return q.hasLimit }
func (q *workflowGroupQueryImplementation) GetLimit() uint64 { return q.limit }
func (q *workflowGroupQueryImplementation) WithLimit(limit uint64) WorkflowGroupQueryInterface {
	q.limit, q.hasLimit = limit, true
	return q
}

func (q *workflowGroupQueryImplementation) HasOffset() bool   { return q.hasOffset }
func (q *workflowGroupQueryImplementation) GetOffset() uint64 { return q.offset }
func (q *workflowGroupQueryImplementation) WithOffset(offset uint64) WorkflowGroupQueryInterface {
	q.offset, q.hasOffset = offset, true
	return q
}

func (q *workflowGroupQueryImplementation) GetSortOrder() string { return q.sortOrder }
func (q *workflowGroupQueryImplementation) WithSortOrder(sortOrder string) WorkflowGroupQueryInterface {
	if sortOrder != DESC {
		sortOrder = ASC
	}
	q.sortOrder = sortOrder
	return q
}

func (q *workflowGroupQueryImplementation) IsCountOnly() bool { return q.countOnly }
func (q *workflowGroupQueryImplementation) WithCountOnly(countOnly bool) WorkflowGroupQueryInterface {
	q.countOnly = countOnly
	return q
}
