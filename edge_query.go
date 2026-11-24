package nodestore

// == INTERFACE ===============================================================

// EdgeQueryInterface is the fluent query builder for listing and
// counting edge entities.
type EdgeQueryInterface interface {
	Validate() error

	HasID() bool
	GetID() string
	WithID(id string) EdgeQueryInterface

	HasWorkflowID() bool
	GetWorkflowID() string
	WithWorkflowID(workflowID string) EdgeQueryInterface

	HasLimit() bool
	GetLimit() uint64
	WithLimit(limit uint64) EdgeQueryInterface

	HasOffset() bool
	GetOffset() uint64
	WithOffset(offset uint64) EdgeQueryInterface

	GetSortOrder() string
	WithSortOrder(sortOrder string) EdgeQueryInterface

	IsCountOnly() bool
	WithCountOnly(countOnly bool) EdgeQueryInterface
}

// == IMPLEMENTATION ==========================================================

type edgeQueryImplementation struct {
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

// EdgeQuery creates a new edge query builder
func EdgeQuery() EdgeQueryInterface {
	return &edgeQueryImplementation{sortOrder: ASC}
}

func (q *edgeQueryImplementation) Validate() error { return nil }

func (q *edgeQueryImplementation) HasID() bool   { return q.hasID }
func (q *edgeQueryImplementation) GetID() string { return q.id }
func (q *edgeQueryImplementation) WithID(id string) EdgeQueryInterface {
	q.id, q.hasID = id, true
	return q
}

func (q *edgeQueryImplementation) HasWorkflowID() bool   { return q.hasWorkflowID }
func (q *edgeQueryImplementation) GetWorkflowID() string { return q.workflowID }
func (q *edgeQueryImplementation) WithWorkflowID(workflowID string) EdgeQueryInterface {
	q.workflowID, q.hasWorkflowID = workflowID, true
	return q
}

func (q *edgeQueryImplementation) HasLimit() bool   { return q.hasLimit }
func (q *edgeQueryImplementation) GetLimit() uint64 { return q.limit }
func (q *edgeQueryImplementation) WithLimit(limit uint64) EdgeQueryInterface {
	q.limit, q.hasLimit = limit, true
	return q
}

func (q *edgeQueryImplementation) HasOffset() bool   { return q.hasOffset }
func (q *edgeQueryImplementation) GetOffset() uint64 { return q.offset }
func (q *edgeQueryImplementation) WithOffset(offset uint64) EdgeQueryInterface {
	q.offset, q.hasOffset = offset, true
	return q
}

func (q *edgeQueryImplementation) GetSortOrder() string { return q.sortOrder }
func (q *edgeQueryImplementation) WithSortOrder(sortOrder string) EdgeQueryInterface {
	if sortOrder != DESC {
		sortOrder = ASC
	}
	q.sortOrder = sortOrder
	return q
}

func (q *edgeQueryImplementation) IsCountOnly() bool { return q.countOnly }
func (q *edgeQueryImplementation) WithCountOnly(countOnly bool) EdgeQueryInterface {
	q.countOnly = countOnly
	return q
}
