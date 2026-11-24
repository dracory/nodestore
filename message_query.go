package nodestore

// == INTERFACE ===============================================================

// MessageQueryInterface is the fluent query builder for listing and
// counting messages.
type MessageQueryInterface interface {
	Validate() error

	HasID() bool
	GetID() string
	WithID(id string) MessageQueryInterface

	HasLimit() bool
	GetLimit() uint64
	WithLimit(limit uint64) MessageQueryInterface

	HasOffset() bool
	GetOffset() uint64
	WithOffset(offset uint64) MessageQueryInterface

	GetSortOrder() string
	WithSortOrder(sortOrder string) MessageQueryInterface

	IsCountOnly() bool
	WithCountOnly(countOnly bool) MessageQueryInterface
}

// == IMPLEMENTATION ==========================================================

type messageQueryImplementation struct {
	id        string
	hasID     bool
	limit     uint64
	hasLimit  bool
	offset    uint64
	hasOffset bool
	sortOrder string
	countOnly bool
}

// MessageQuery creates a new message query builder
func MessageQuery() MessageQueryInterface {
	return &messageQueryImplementation{sortOrder: ASC}
}

func (q *messageQueryImplementation) Validate() error { return nil }

func (q *messageQueryImplementation) HasID() bool   { return q.hasID }
func (q *messageQueryImplementation) GetID() string { return q.id }
func (q *messageQueryImplementation) WithID(id string) MessageQueryInterface {
	q.id, q.hasID = id, true
	return q
}

func (q *messageQueryImplementation) HasLimit() bool   { return q.hasLimit }
func (q *messageQueryImplementation) GetLimit() uint64 { return q.limit }
func (q *messageQueryImplementation) WithLimit(limit uint64) MessageQueryInterface {
	q.limit, q.hasLimit = limit, true
	return q
}

func (q *messageQueryImplementation) HasOffset() bool   { return q.hasOffset }
func (q *messageQueryImplementation) GetOffset() uint64 { return q.offset }
func (q *messageQueryImplementation) WithOffset(offset uint64) MessageQueryInterface {
	q.offset, q.hasOffset = offset, true
	return q
}

func (q *messageQueryImplementation) GetSortOrder() string { return q.sortOrder }
func (q *messageQueryImplementation) WithSortOrder(sortOrder string) MessageQueryInterface {
	if sortOrder != DESC {
		sortOrder = ASC
	}
	q.sortOrder = sortOrder
	return q
}

func (q *messageQueryImplementation) IsCountOnly() bool { return q.countOnly }
func (q *messageQueryImplementation) WithCountOnly(countOnly bool) MessageQueryInterface {
	q.countOnly = countOnly
	return q
}
