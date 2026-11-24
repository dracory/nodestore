package nodestore

// == INTERFACE ===============================================================

// DeployRecordQueryInterface is the fluent query builder for listing
// and counting deploy record entities.
type DeployRecordQueryInterface interface {
	Validate() error

	HasID() bool
	GetID() string
	WithID(id string) DeployRecordQueryInterface

	HasWorkflowID() bool
	GetWorkflowID() string
	WithWorkflowID(workflowID string) DeployRecordQueryInterface

	HasLimit() bool
	GetLimit() uint64
	WithLimit(limit uint64) DeployRecordQueryInterface

	HasOffset() bool
	GetOffset() uint64
	WithOffset(offset uint64) DeployRecordQueryInterface

	GetSortOrder() string
	WithSortOrder(sortOrder string) DeployRecordQueryInterface

	IsCountOnly() bool
	WithCountOnly(countOnly bool) DeployRecordQueryInterface
}

// == IMPLEMENTATION ==========================================================

type deployRecordQueryImplementation struct {
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

// DeployRecordQuery creates a new deploy record query builder. Deploy
// history reads newest-first, so DESC is the default here (unlike the
// other entity queries).
func DeployRecordQuery() DeployRecordQueryInterface {
	return &deployRecordQueryImplementation{sortOrder: DESC}
}

func (q *deployRecordQueryImplementation) Validate() error { return nil }

func (q *deployRecordQueryImplementation) HasID() bool   { return q.hasID }
func (q *deployRecordQueryImplementation) GetID() string { return q.id }
func (q *deployRecordQueryImplementation) WithID(id string) DeployRecordQueryInterface {
	q.id, q.hasID = id, true
	return q
}

func (q *deployRecordQueryImplementation) HasWorkflowID() bool   { return q.hasWorkflowID }
func (q *deployRecordQueryImplementation) GetWorkflowID() string { return q.workflowID }
func (q *deployRecordQueryImplementation) WithWorkflowID(workflowID string) DeployRecordQueryInterface {
	q.workflowID, q.hasWorkflowID = workflowID, true
	return q
}

func (q *deployRecordQueryImplementation) HasLimit() bool   { return q.hasLimit }
func (q *deployRecordQueryImplementation) GetLimit() uint64 { return q.limit }
func (q *deployRecordQueryImplementation) WithLimit(limit uint64) DeployRecordQueryInterface {
	q.limit, q.hasLimit = limit, true
	return q
}

func (q *deployRecordQueryImplementation) HasOffset() bool   { return q.hasOffset }
func (q *deployRecordQueryImplementation) GetOffset() uint64 { return q.offset }
func (q *deployRecordQueryImplementation) WithOffset(offset uint64) DeployRecordQueryInterface {
	q.offset, q.hasOffset = offset, true
	return q
}

func (q *deployRecordQueryImplementation) GetSortOrder() string { return q.sortOrder }
func (q *deployRecordQueryImplementation) WithSortOrder(sortOrder string) DeployRecordQueryInterface {
	if sortOrder != ASC {
		sortOrder = DESC
	}
	q.sortOrder = sortOrder
	return q
}

func (q *deployRecordQueryImplementation) IsCountOnly() bool { return q.countOnly }
func (q *deployRecordQueryImplementation) WithCountOnly(countOnly bool) DeployRecordQueryInterface {
	q.countOnly = countOnly
	return q
}
