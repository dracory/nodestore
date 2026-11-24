package nodestore

import (
	"crypto/sha256"
	"encoding/hex"
	"time"

	"github.com/dracory/entitystore"
)

// == INTERFACE ===============================================================

// EdgeInterface is the interface form of flowrunner.Edge — a connection
// between two nodes. The edge IS an entitystore entity: its entity ID is
// the edge ID and created/updated timestamps come from the entity.
type EdgeInterface interface {
	GetID() string
	SetID(id string) EdgeInterface

	GetWorkflowID() string
	SetWorkflowID(workflowID string) EdgeInterface

	GetSource() string
	SetSource(source string) EdgeInterface
	GetSourceKey() string
	SetSourceKey(sourceKey string) EdgeInterface

	GetTarget() string
	SetTarget(target string) EdgeInterface
	GetTargetKey() string
	SetTargetKey(targetKey string) EdgeInterface

	GetCreatedAt() time.Time
	SetCreatedAt(createdAt time.Time) EdgeInterface

	GetUpdatedAt() time.Time
	SetUpdatedAt(updatedAt time.Time) EdgeInterface
}

// == IMPLEMENTATION ==========================================================

// edgeJSON mirrors flowrunner.Edge's JSON shape (module exchange,
// Node-RED import, legacy blob fallback)
type edgeJSON struct {
	ID        string `json:"id"`
	Source    string `json:"source"`
	SourceKey string `json:"sourceKey,omitempty"`
	Target    string `json:"target"`
	TargetKey string `json:"targetKey,omitempty"`
}

// edgeManagedAttributes lists the attribute keys an edge owns
var edgeManagedAttributes = map[string]bool{
	ATTRIBUTE_WORKFLOW_ID: true,
	ATTRIBUTE_SOURCE:      true,
	ATTRIBUTE_SOURCE_KEY:  true,
	ATTRIBUTE_TARGET:      true,
	ATTRIBUTE_TARGET_KEY:  true,
}

type edgeImplementation struct {
	entity     entitystore.EntityInterface
	workflowID string
	source     string
	sourceKey  string
	target     string
	targetKey  string
}

// maxEntityIDLength is the entitystore entity ID column limit — any id
// handed to EntityCreate must fit.
const maxEntityIDLength = entitystore.ID_COLUMN_LENGTH

// shortEntityID returns a deterministic storage-safe entity id. Ids
// within the column limit pass through unchanged; longer ids collapse
// to "<prefix>-<sha256 hex[:32]>" (prefix + 33 chars ≤ 40), so deriving
// the id twice (migration + read fallback, migration re-run) always
// yields the same value.
//
// Used for legacy blob children: pre-entity stores wrote synthetic
// composite edge ids ("edge-{workflowID}-{src}-{srcPort}-{dst}-{dstPort}",
// ~100 chars) that exceed the 40-char entity ID column.
func shortEntityID(prefix, id string) string {
	if len(id) <= maxEntityIDLength {
		return id
	}
	sum := sha256.Sum256([]byte(id))
	return prefix + "-" + hex.EncodeToString(sum[:])[:32]
}

// NewEdge creates a new workflow edge. Edge ids are never referenced by
// other entities (edges resolve via their source/target attributes), so
// an over-length id is deterministically shortened at the boundary.
func NewEdge(id, source, target string) EdgeInterface {
	entity := entitystore.NewEntity()
	entity.SetType(EntityTypeEdge)
	if id != "" {
		entity.SetID(shortEntityID("edge", id))
	}
	return &edgeImplementation{entity: entity, source: source, target: target}
}

// edgeFromEntity rebuilds an edge from a persisted entity + attributes
func edgeFromEntity(entity entitystore.EntityInterface, attrs map[string]string) (EdgeInterface, error) {
	return &edgeImplementation{
		entity:     entity,
		workflowID: attrs[ATTRIBUTE_WORKFLOW_ID],
		source:     attrs[ATTRIBUTE_SOURCE],
		sourceKey:  attrs[ATTRIBUTE_SOURCE_KEY],
		target:     attrs[ATTRIBUTE_TARGET],
		targetKey:  attrs[ATTRIBUTE_TARGET_KEY],
	}, nil
}

func (e *edgeImplementation) attributes() map[string]string {
	return map[string]string{
		ATTRIBUTE_WORKFLOW_ID: e.workflowID,
		ATTRIBUTE_SOURCE:      e.source,
		ATTRIBUTE_SOURCE_KEY:  e.sourceKey,
		ATTRIBUTE_TARGET:      e.target,
		ATTRIBUTE_TARGET_KEY:  e.targetKey,
	}
}

func (e *edgeImplementation) GetID() string                 { return e.entity.GetID() }
func (e *edgeImplementation) SetID(id string) EdgeInterface { e.entity.SetID(id); return e }

func (e *edgeImplementation) GetWorkflowID() string { return e.workflowID }
func (e *edgeImplementation) SetWorkflowID(workflowID string) EdgeInterface {
	e.workflowID = workflowID
	return e
}

func (e *edgeImplementation) GetSource() string                { return e.source }
func (e *edgeImplementation) SetSource(s string) EdgeInterface { e.source = s; return e }

func (e *edgeImplementation) GetSourceKey() string                { return e.sourceKey }
func (e *edgeImplementation) SetSourceKey(k string) EdgeInterface { e.sourceKey = k; return e }

func (e *edgeImplementation) GetTarget() string                { return e.target }
func (e *edgeImplementation) SetTarget(t string) EdgeInterface { e.target = t; return e }

func (e *edgeImplementation) GetTargetKey() string                { return e.targetKey }
func (e *edgeImplementation) SetTargetKey(k string) EdgeInterface { e.targetKey = k; return e }

func (e *edgeImplementation) GetCreatedAt() time.Time {
	t, _ := parseDatetime(e.entity.GetCreatedAt())
	return t
}
func (e *edgeImplementation) SetCreatedAt(createdAt time.Time) EdgeInterface {
	e.entity.SetCreatedAt(formatDatetime(createdAt))
	return e
}

func (e *edgeImplementation) GetUpdatedAt() time.Time {
	t, _ := parseDatetime(e.entity.GetUpdatedAt())
	return t
}
func (e *edgeImplementation) SetUpdatedAt(updatedAt time.Time) EdgeInterface {
	e.entity.SetUpdatedAt(formatDatetime(updatedAt))
	return e
}

func (e *edgeImplementation) toJSON() edgeJSON {
	return edgeJSON{
		ID:        e.GetID(),
		Source:    e.source,
		SourceKey: e.sourceKey,
		Target:    e.target,
		TargetKey: e.targetKey,
	}
}

func edgeFromJSON(j edgeJSON) EdgeInterface {
	return NewEdge(j.ID, j.Source, j.Target).SetSourceKey(j.SourceKey).SetTargetKey(j.TargetKey)
}
