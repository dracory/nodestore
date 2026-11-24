package nodestore

// == INTERFACE ===============================================================

// WorkflowQueryInterface is the fluent query builder for listing and
// counting workflows.
type WorkflowQueryInterface interface {
	Validate() error

	HasID() bool
	GetID() string
	WithID(id string) WorkflowQueryInterface

	HasName() bool
	GetName() string
	WithName(name string) WorkflowQueryInterface

	HasDeployed() bool
	GetDeployed() bool
	WithDeployed(deployed bool) WorkflowQueryInterface

	HasLimit() bool
	GetLimit() uint64
	WithLimit(limit uint64) WorkflowQueryInterface

	HasOffset() bool
	GetOffset() uint64
	WithOffset(offset uint64) WorkflowQueryInterface

	GetSortOrder() string
	WithSortOrder(sortOrder string) WorkflowQueryInterface

	IsCountOnly() bool
	WithCountOnly(countOnly bool) WorkflowQueryInterface
}

// == IMPLEMENTATION ==========================================================

type workflowQueryImplementation struct {
	id        string
	hasID     bool
	name      string
	hasName   bool
	deployed  bool
	hasDeploy bool
	limit     uint64
	hasLimit  bool
	offset    uint64
	hasOffset bool
	sortOrder string
	countOnly bool
}

// WorkflowQuery creates a new workflow query builder
func WorkflowQuery() WorkflowQueryInterface {
	return &workflowQueryImplementation{sortOrder: ASC}
}

func (q *workflowQueryImplementation) Validate() error { return nil }

func (q *workflowQueryImplementation) HasID() bool   { return q.hasID }
func (q *workflowQueryImplementation) GetID() string { return q.id }
func (q *workflowQueryImplementation) WithID(id string) WorkflowQueryInterface {
	q.id, q.hasID = id, true
	return q
}

func (q *workflowQueryImplementation) HasName() bool   { return q.hasName }
func (q *workflowQueryImplementation) GetName() string { return q.name }
func (q *workflowQueryImplementation) WithName(name string) WorkflowQueryInterface {
	q.name, q.hasName = unifyName(name), true
	return q
}

func (q *workflowQueryImplementation) HasDeployed() bool { return q.hasDeploy }
func (q *workflowQueryImplementation) GetDeployed() bool { return q.deployed }
func (q *workflowQueryImplementation) WithDeployed(deployed bool) WorkflowQueryInterface {
	q.deployed, q.hasDeploy = deployed, true
	return q
}

func (q *workflowQueryImplementation) HasLimit() bool   { return q.hasLimit }
func (q *workflowQueryImplementation) GetLimit() uint64 { return q.limit }
func (q *workflowQueryImplementation) WithLimit(limit uint64) WorkflowQueryInterface {
	q.limit, q.hasLimit = limit, true
	return q
}

func (q *workflowQueryImplementation) HasOffset() bool   { return q.hasOffset }
func (q *workflowQueryImplementation) GetOffset() uint64 { return q.offset }
func (q *workflowQueryImplementation) WithOffset(offset uint64) WorkflowQueryInterface {
	q.offset, q.hasOffset = offset, true
	return q
}

func (q *workflowQueryImplementation) GetSortOrder() string { return q.sortOrder }
func (q *workflowQueryImplementation) WithSortOrder(sortOrder string) WorkflowQueryInterface {
	if sortOrder != DESC {
		sortOrder = ASC
	}
	q.sortOrder = sortOrder
	return q
}

func (q *workflowQueryImplementation) IsCountOnly() bool { return q.countOnly }
func (q *workflowQueryImplementation) WithCountOnly(countOnly bool) WorkflowQueryInterface {
	q.countOnly = countOnly
	return q
}
