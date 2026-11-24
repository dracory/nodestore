package nodestore

import (
	"encoding/json"
	"time"

	"github.com/dracory/entitystore"
)

// == INTERFACE ===============================================================

// NodeGroupInterface is the entity form of an editor view group (one of
// the view.groups entries: {id, name, x, y, w, h, disabled, module,
// nodeIds}). Membership is NOT stored here — it lives as group_id on
// the member node entities (GOD ruling 2026-11-24); the geometry and
// flags ride in the data attribute so the wire shape round-trips.
type NodeGroupInterface interface {
	GetID() string
	SetID(id string) NodeGroupInterface

	GetWorkflowID() string
	SetWorkflowID(workflowID string) NodeGroupInterface

	GetName() string
	SetName(name string) NodeGroupInterface

	// GetData returns the group's freeform view payload (x, y, w, h,
	// disabled, module, env) — the view.groups entry minus id/name/
	// nodeIds.
	GetData() map[string]any
	SetData(data map[string]any) NodeGroupInterface

	GetCreatedAt() time.Time
	SetCreatedAt(createdAt time.Time) NodeGroupInterface

	GetUpdatedAt() time.Time
	SetUpdatedAt(updatedAt time.Time) NodeGroupInterface
}

// == IMPLEMENTATION ==========================================================

var nodeGroupManagedAttributes = map[string]bool{
	ATTRIBUTE_WORKFLOW_ID: true,
	ATTRIBUTE_NAME:        true,
	ATTRIBUTE_DATA:        true,
}

type nodeGroupImplementation struct {
	entity     entitystore.EntityInterface
	workflowID string
	name       string
	data       map[string]any
}

// NewNodeGroup creates a new node group entity
func NewNodeGroup(workflowID, name string) NodeGroupInterface {
	entity := entitystore.NewEntity()
	entity.SetType(EntityTypeNodeGroup)
	return &nodeGroupImplementation{
		entity:     entity,
		workflowID: workflowID,
		name:       name,
		data:       map[string]any{},
	}
}

func nodeGroupFromEntity(entity entitystore.EntityInterface, attrs map[string]string) (NodeGroupInterface, error) {
	g := &nodeGroupImplementation{
		entity:     entity,
		workflowID: attrs[ATTRIBUTE_WORKFLOW_ID],
		name:       attrs[ATTRIBUTE_NAME],
		data:       map[string]any{},
	}
	if raw := attrs[ATTRIBUTE_DATA]; raw != "" {
		if err := json.Unmarshal([]byte(raw), &g.data); err != nil {
			return nil, err
		}
	}
	return g, nil
}

func (g *nodeGroupImplementation) attributes() (map[string]string, error) {
	data, err := json.Marshal(g.data)
	if err != nil {
		return nil, err
	}
	return map[string]string{
		ATTRIBUTE_WORKFLOW_ID: g.workflowID,
		ATTRIBUTE_NAME:        g.name,
		ATTRIBUTE_DATA:        string(data),
	}, nil
}

func (g *nodeGroupImplementation) GetID() string { return g.entity.GetID() }
func (g *nodeGroupImplementation) SetID(id string) NodeGroupInterface {
	g.entity.SetID(id)
	return g
}

func (g *nodeGroupImplementation) GetWorkflowID() string { return g.workflowID }
func (g *nodeGroupImplementation) SetWorkflowID(workflowID string) NodeGroupInterface {
	g.workflowID = workflowID
	return g
}

func (g *nodeGroupImplementation) GetName() string { return g.name }
func (g *nodeGroupImplementation) SetName(name string) NodeGroupInterface {
	g.name = name
	return g
}

func (g *nodeGroupImplementation) GetData() map[string]any { return g.data }
func (g *nodeGroupImplementation) SetData(data map[string]any) NodeGroupInterface {
	if data == nil {
		data = map[string]any{}
	}
	g.data = data
	return g
}

func (g *nodeGroupImplementation) GetCreatedAt() time.Time {
	t, _ := parseDatetime(g.entity.GetCreatedAt())
	return t
}
func (g *nodeGroupImplementation) SetCreatedAt(createdAt time.Time) NodeGroupInterface {
	g.entity.SetCreatedAt(formatDatetime(createdAt))
	return g
}

func (g *nodeGroupImplementation) GetUpdatedAt() time.Time {
	t, _ := parseDatetime(g.entity.GetUpdatedAt())
	return t
}
func (g *nodeGroupImplementation) SetUpdatedAt(updatedAt time.Time) NodeGroupInterface {
	g.entity.SetUpdatedAt(formatDatetime(updatedAt))
	return g
}
