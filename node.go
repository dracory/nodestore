package nodestore

import (
	"encoding/json"
	"time"

	"github.com/dracory/entitystore"
)

// == INTERFACE ===============================================================

// NodeInterface is the interface form of flowrunner.Node — a single node
// in a workflow graph. The node IS an entitystore entity: its entity ID
// is the node ID and created/updated timestamps come from the entity.
type NodeInterface interface {
	GetID() string
	SetID(id string) NodeInterface

	GetWorkflowID() string
	SetWorkflowID(workflowID string) NodeInterface

	// GetGroupID returns the node_group entity this node belongs to
	// ("" = ungrouped) — group membership lives on the node, not on
	// the group (GOD ruling 2026-11-24).
	GetGroupID() string
	SetGroupID(groupID string) NodeInterface

	GetType() string
	SetType(nodeType string) NodeInterface

	GetPositionX() float64
	SetPositionX(x float64) NodeInterface
	GetPositionY() float64
	SetPositionY(y float64) NodeInterface
	SetPosition(x, y float64) NodeInterface

	GetData() map[string]any
	SetData(data map[string]any) NodeInterface

	GetCreatedAt() time.Time
	SetCreatedAt(createdAt time.Time) NodeInterface

	GetUpdatedAt() time.Time
	SetUpdatedAt(updatedAt time.Time) NodeInterface
}

// == IMPLEMENTATION ==========================================================

// nodeJSON mirrors flowrunner.Node's JSON shape for round-trip fidelity
// (used by module exchange, Node-RED import and the legacy blob fallback)
type nodeJSON struct {
	ID       string         `json:"id"`
	Type     string         `json:"type"`
	Position positionJSON   `json:"position"`
	Data     map[string]any `json:"data"`
}

type positionJSON struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}

// nodeManagedAttributes lists the attribute keys a node owns; anything
// else persisted on the entity is left untouched by updates.
var nodeManagedAttributes = map[string]bool{
	ATTRIBUTE_WORKFLOW_ID: true,
	ATTRIBUTE_TYPE:        true,
	ATTRIBUTE_POSITION:    true,
	ATTRIBUTE_DATA:        true,
	ATTRIBUTE_GROUP_ID:    true,
}

type nodeImplementation struct {
	entity     entitystore.EntityInterface
	workflowID string
	nodeType   string
	x          float64
	y          float64
	data       map[string]any
	groupID    string
}

// NewNode creates a new workflow node
func NewNode(id, nodeType string) NodeInterface {
	entity := entitystore.NewEntity()
	entity.SetType(EntityTypeNode)
	if id != "" {
		entity.SetID(id)
	}
	return &nodeImplementation{
		entity:   entity,
		nodeType: nodeType,
		data:     map[string]any{},
	}
}

// nodeFromEntity rebuilds a node from a persisted entity + attributes
func nodeFromEntity(entity entitystore.EntityInterface, attrs map[string]string) (NodeInterface, error) {
	n := &nodeImplementation{
		entity: entity,
		data:   map[string]any{},
	}
	n.workflowID = attrs[ATTRIBUTE_WORKFLOW_ID]
	n.nodeType = attrs[ATTRIBUTE_TYPE]
	n.groupID = attrs[ATTRIBUTE_GROUP_ID]
	if raw := attrs[ATTRIBUTE_POSITION]; raw != "" {
		var pos positionJSON
		if err := json.Unmarshal([]byte(raw), &pos); err != nil {
			return nil, err
		}
		n.x, n.y = pos.X, pos.Y
	}
	if raw := attrs[ATTRIBUTE_DATA]; raw != "" {
		if err := json.Unmarshal([]byte(raw), &n.data); err != nil {
			return nil, err
		}
	}
	return n, nil
}

// attributes serializes the node's domain fields into entitystore
// attribute key/value pairs (timestamps live on the entity itself)
func (n *nodeImplementation) attributes() (map[string]string, error) {
	position, err := json.Marshal(positionJSON{X: n.x, Y: n.y})
	if err != nil {
		return nil, err
	}
	data, err := json.Marshal(n.data)
	if err != nil {
		return nil, err
	}
	return map[string]string{
		ATTRIBUTE_WORKFLOW_ID: n.workflowID,
		ATTRIBUTE_TYPE:        n.nodeType,
		ATTRIBUTE_POSITION:    string(position),
		ATTRIBUTE_DATA:        string(data),
		ATTRIBUTE_GROUP_ID:    n.groupID,
	}, nil
}

func (n *nodeImplementation) GetID() string                 { return n.entity.GetID() }
func (n *nodeImplementation) SetID(id string) NodeInterface { n.entity.SetID(id); return n }

func (n *nodeImplementation) GetWorkflowID() string { return n.workflowID }
func (n *nodeImplementation) SetWorkflowID(workflowID string) NodeInterface {
	n.workflowID = workflowID
	return n
}

func (n *nodeImplementation) GetGroupID() string { return n.groupID }
func (n *nodeImplementation) SetGroupID(groupID string) NodeInterface {
	n.groupID = groupID
	return n
}

func (n *nodeImplementation) GetType() string                      { return n.nodeType }
func (n *nodeImplementation) SetType(t string) NodeInterface       { n.nodeType = t; return n }
func (n *nodeImplementation) GetPositionX() float64                { return n.x }
func (n *nodeImplementation) SetPositionX(x float64) NodeInterface { n.x = x; return n }
func (n *nodeImplementation) GetPositionY() float64                { return n.y }
func (n *nodeImplementation) SetPositionY(y float64) NodeInterface { n.y = y; return n }
func (n *nodeImplementation) SetPosition(x, y float64) NodeInterface {
	n.x, n.y = x, y
	return n
}

func (n *nodeImplementation) GetData() map[string]any { return n.data }
func (n *nodeImplementation) SetData(data map[string]any) NodeInterface {
	if data == nil {
		data = map[string]any{}
	}
	n.data = data
	return n
}

func (n *nodeImplementation) GetCreatedAt() time.Time {
	t, _ := parseDatetime(n.entity.GetCreatedAt())
	return t
}
func (n *nodeImplementation) SetCreatedAt(createdAt time.Time) NodeInterface {
	n.entity.SetCreatedAt(formatDatetime(createdAt))
	return n
}

func (n *nodeImplementation) GetUpdatedAt() time.Time {
	t, _ := parseDatetime(n.entity.GetUpdatedAt())
	return t
}
func (n *nodeImplementation) SetUpdatedAt(updatedAt time.Time) NodeInterface {
	n.entity.SetUpdatedAt(formatDatetime(updatedAt))
	return n
}

func (n *nodeImplementation) toJSON() nodeJSON {
	return nodeJSON{
		ID:       n.GetID(),
		Type:     n.nodeType,
		Position: positionJSON{X: n.x, Y: n.y},
		Data:     n.data,
	}
}

func nodeFromJSON(j nodeJSON) NodeInterface {
	node := NewNode(j.ID, j.Type).SetPosition(j.Position.X, j.Position.Y)
	if j.Data != nil {
		node.SetData(j.Data)
	}
	return node
}
