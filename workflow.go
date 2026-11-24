package nodestore

import (
	"encoding/json"
	"time"

	"github.com/dracory/entitystore"
)

// == INTERFACE ===============================================================

// WorkflowInterface is the interface form of flowrunner.Workflow — a
// complete workflow definition persisted in the entity store.
type WorkflowInterface interface {
	GetID() string
	SetID(id string) WorkflowInterface

	GetName() string
	SetName(name string) WorkflowInterface

	GetDescription() string
	SetDescription(description string) WorkflowInterface

	GetNodes() []NodeInterface
	SetNodes(nodes []NodeInterface) WorkflowInterface
	AddNode(node NodeInterface) WorkflowInterface
	FindNodeByID(id string) NodeInterface

	GetEdges() []EdgeInterface
	SetEdges(edges []EdgeInterface) WorkflowInterface
	AddEdge(edge EdgeInterface) WorkflowInterface

	GetView() map[string]any
	SetView(view map[string]any) WorkflowInterface

	GetDeployed() bool
	SetDeployed(deployed bool) WorkflowInterface

	GetCreatedAt() time.Time
	SetCreatedAt(createdAt time.Time) WorkflowInterface

	GetUpdatedAt() time.Time
	SetUpdatedAt(updatedAt time.Time) WorkflowInterface
}

// == IMPLEMENTATION ==========================================================

type workflowImplementation struct {
	entity      entitystore.EntityInterface
	name        string
	description string
	nodes       []NodeInterface
	edges       []EdgeInterface
	view        map[string]any
	deployed    bool
}

// NewWorkflow creates a new workflow
func NewWorkflow(name string) WorkflowInterface {
	entity := entitystore.NewEntity()
	entity.SetType(EntityTypeWorkflow)

	workflow := &workflowImplementation{
		entity: entity,
		nodes:  []NodeInterface{},
		edges:  []EdgeInterface{},
		view:   map[string]any{},
	}
	workflow.SetName(name)
	return workflow
}

// workflowManagedAttributes lists the attribute keys a workflow owns —
// including the legacy ATTRIBUTE_NODES/ATTRIBUTE_EDGES blob keys so
// updates clean them up after migration.
var workflowManagedAttributes = map[string]bool{
	ATTRIBUTE_NAME:        true,
	ATTRIBUTE_DESCRIPTION: true,
	ATTRIBUTE_NODES:       true, // legacy, cleaned up on update
	ATTRIBUTE_EDGES:       true, // legacy, cleaned up on update
	ATTRIBUTE_VIEW:        true,
	ATTRIBUTE_DEPLOYED:    true,
}

// workflowFromEntity rebuilds a workflow from a persisted entity + its
// attributes. Nodes and edges are separate child entities loaded by the
// store layer (see Store.loadWorkflowChildren); the legacy
// ATTRIBUTE_NODES/ATTRIBUTE_EDGES JSON blobs are still parsed here as a
// read fallback for unmigrated records.
func workflowFromEntity(entity entitystore.EntityInterface, attrs map[string]string) (WorkflowInterface, error) {
	w := &workflowImplementation{
		entity: entity,
		nodes:  []NodeInterface{},
		edges:  []EdgeInterface{},
		view:   map[string]any{},
	}
	w.name = attrs[ATTRIBUTE_NAME]
	w.description = attrs[ATTRIBUTE_DESCRIPTION]
	w.deployed = attrs[ATTRIBUTE_DEPLOYED] == "true"

	if raw := attrs[ATTRIBUTE_NODES]; raw != "" {
		var list []nodeJSON
		if err := json.Unmarshal([]byte(raw), &list); err != nil {
			return nil, err
		}
		for _, j := range list {
			w.nodes = append(w.nodes, nodeFromJSON(j))
		}
	}
	if raw := attrs[ATTRIBUTE_EDGES]; raw != "" {
		var list []edgeJSON
		if err := json.Unmarshal([]byte(raw), &list); err != nil {
			return nil, err
		}
		for _, j := range list {
			w.edges = append(w.edges, edgeFromJSON(j))
		}
	}
	if raw := attrs[ATTRIBUTE_VIEW]; raw != "" {
		if err := json.Unmarshal([]byte(raw), &w.view); err != nil {
			return nil, err
		}
	}
	return w, nil
}

// attributes serializes the workflow's own domain fields into
// entitystore attribute key/value pairs. Nodes and edges are NOT here —
// they are child entities persisted via syncWorkflowChildren. The view
// "groups" key is stripped too (P5): group state lives in node_group
// entities + node.group_id and is re-synthesized on read.
func (w *workflowImplementation) attributes() (map[string]string, error) {
	view := make(map[string]any, len(w.view))
	for k, v := range w.view {
		if k == "groups" {
			continue
		}
		view[k] = v
	}
	viewRaw, err := json.Marshal(view)
	if err != nil {
		return nil, err
	}

	deployed := "false"
	if w.deployed {
		deployed = "true"
	}

	return map[string]string{
		ATTRIBUTE_NAME:        w.name,
		ATTRIBUTE_DESCRIPTION: w.description,
		ATTRIBUTE_VIEW:        string(viewRaw),
		ATTRIBUTE_DEPLOYED:    deployed,
	}, nil
}

func (w *workflowImplementation) entity_() entitystore.EntityInterface {
	return w.entity
}

func (w *workflowImplementation) GetID() string                     { return w.entity.GetID() }
func (w *workflowImplementation) SetID(id string) WorkflowInterface { w.entity.SetID(id); return w }

func (w *workflowImplementation) GetName() string { return w.name }
func (w *workflowImplementation) SetName(name string) WorkflowInterface {
	w.name = name
	w.entity.SetHandle(unifyName(name))
	return w
}

func (w *workflowImplementation) GetDescription() string { return w.description }
func (w *workflowImplementation) SetDescription(description string) WorkflowInterface {
	w.description = description
	return w
}

func (w *workflowImplementation) GetNodes() []NodeInterface { return w.nodes }
func (w *workflowImplementation) SetNodes(nodes []NodeInterface) WorkflowInterface {
	if nodes == nil {
		nodes = []NodeInterface{}
	}
	w.nodes = nodes
	return w
}
func (w *workflowImplementation) AddNode(node NodeInterface) WorkflowInterface {
	w.nodes = append(w.nodes, node)
	return w
}
func (w *workflowImplementation) FindNodeByID(id string) NodeInterface {
	for _, n := range w.nodes {
		if n.GetID() == id {
			return n
		}
	}
	return nil
}

func (w *workflowImplementation) GetEdges() []EdgeInterface { return w.edges }
func (w *workflowImplementation) SetEdges(edges []EdgeInterface) WorkflowInterface {
	if edges == nil {
		edges = []EdgeInterface{}
	}
	w.edges = edges
	return w
}
func (w *workflowImplementation) AddEdge(edge EdgeInterface) WorkflowInterface {
	w.edges = append(w.edges, edge)
	return w
}

func (w *workflowImplementation) GetView() map[string]any { return w.view }
func (w *workflowImplementation) SetView(view map[string]any) WorkflowInterface {
	if view == nil {
		view = map[string]any{}
	}
	w.view = view
	return w
}

func (w *workflowImplementation) GetDeployed() bool { return w.deployed }
func (w *workflowImplementation) SetDeployed(deployed bool) WorkflowInterface {
	w.deployed = deployed
	return w
}

func (w *workflowImplementation) GetCreatedAt() time.Time {
	t, _ := parseDatetime(w.entity.GetCreatedAt())
	return t
}
func (w *workflowImplementation) SetCreatedAt(createdAt time.Time) WorkflowInterface {
	w.entity.SetCreatedAt(formatDatetime(createdAt))
	return w
}

func (w *workflowImplementation) GetUpdatedAt() time.Time {
	t, _ := parseDatetime(w.entity.GetUpdatedAt())
	return t
}
func (w *workflowImplementation) SetUpdatedAt(updatedAt time.Time) WorkflowInterface {
	w.entity.SetUpdatedAt(formatDatetime(updatedAt))
	return w
}
