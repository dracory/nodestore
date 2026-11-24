package nodestore

import (
	"encoding/json"
	"time"

	"github.com/dracory/entitystore"
)

// == INTERFACE ===============================================================

// WorkflowGroupInterface is the entity form of a sidebar workflow
// grouping entry — previously the global-config "workflowGroups" JSON
// section shaped {id, name, parentId, workflows:[id]}. Members are the
// workflow IDs listed in the group.
type WorkflowGroupInterface interface {
	GetID() string
	SetID(id string) WorkflowGroupInterface

	GetName() string
	SetName(name string) WorkflowGroupInterface

	// GetParentID returns the parent group ID ("" = top level).
	GetParentID() string
	SetParentID(parentID string) WorkflowGroupInterface

	// GetWorkflowIDs returns the member workflow IDs.
	GetWorkflowIDs() []string
	SetWorkflowIDs(ids []string) WorkflowGroupInterface

	GetCreatedAt() time.Time
	SetCreatedAt(createdAt time.Time) WorkflowGroupInterface

	GetUpdatedAt() time.Time
	SetUpdatedAt(updatedAt time.Time) WorkflowGroupInterface
}

// == IMPLEMENTATION ==========================================================

var workflowGroupManagedAttributes = map[string]bool{
	ATTRIBUTE_NAME:      true,
	ATTRIBUTE_PARENT_ID: true,
	ATTRIBUTE_WORKFLOWS: true,
}

type workflowGroupImplementation struct {
	entity      entitystore.EntityInterface
	name        string
	parentID    string
	workflowIDs []string
}

// NewWorkflowGroup creates a new workflow group entity
func NewWorkflowGroup(name string) WorkflowGroupInterface {
	entity := entitystore.NewEntity()
	entity.SetType(EntityTypeWorkflowGroup)
	return &workflowGroupImplementation{
		entity:      entity,
		name:        name,
		workflowIDs: []string{},
	}
}

func workflowGroupFromEntity(entity entitystore.EntityInterface, attrs map[string]string) (WorkflowGroupInterface, error) {
	g := &workflowGroupImplementation{
		entity:      entity,
		name:        attrs[ATTRIBUTE_NAME],
		parentID:    attrs[ATTRIBUTE_PARENT_ID],
		workflowIDs: []string{},
	}
	if raw := attrs[ATTRIBUTE_WORKFLOWS]; raw != "" {
		if err := json.Unmarshal([]byte(raw), &g.workflowIDs); err != nil {
			return nil, err
		}
	}
	return g, nil
}

func (g *workflowGroupImplementation) attributes() (map[string]string, error) {
	ids, err := json.Marshal(g.workflowIDs)
	if err != nil {
		return nil, err
	}
	return map[string]string{
		ATTRIBUTE_NAME:      g.name,
		ATTRIBUTE_PARENT_ID: g.parentID,
		ATTRIBUTE_WORKFLOWS: string(ids),
	}, nil
}

func (g *workflowGroupImplementation) GetID() string { return g.entity.GetID() }
func (g *workflowGroupImplementation) SetID(id string) WorkflowGroupInterface {
	g.entity.SetID(id)
	return g
}

func (g *workflowGroupImplementation) GetName() string { return g.name }
func (g *workflowGroupImplementation) SetName(name string) WorkflowGroupInterface {
	g.name = name
	return g
}

func (g *workflowGroupImplementation) GetParentID() string { return g.parentID }
func (g *workflowGroupImplementation) SetParentID(parentID string) WorkflowGroupInterface {
	g.parentID = parentID
	return g
}

func (g *workflowGroupImplementation) GetWorkflowIDs() []string { return g.workflowIDs }
func (g *workflowGroupImplementation) SetWorkflowIDs(ids []string) WorkflowGroupInterface {
	if ids == nil {
		ids = []string{}
	}
	g.workflowIDs = ids
	return g
}

func (g *workflowGroupImplementation) GetCreatedAt() time.Time {
	t, _ := parseDatetime(g.entity.GetCreatedAt())
	return t
}
func (g *workflowGroupImplementation) SetCreatedAt(createdAt time.Time) WorkflowGroupInterface {
	g.entity.SetCreatedAt(formatDatetime(createdAt))
	return g
}

func (g *workflowGroupImplementation) GetUpdatedAt() time.Time {
	t, _ := parseDatetime(g.entity.GetUpdatedAt())
	return t
}
func (g *workflowGroupImplementation) SetUpdatedAt(updatedAt time.Time) WorkflowGroupInterface {
	g.entity.SetUpdatedAt(formatDatetime(updatedAt))
	return g
}
