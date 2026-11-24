package nodestore

import (
	"encoding/json"
	"time"

	"github.com/dracory/entitystore"
)

// == INTERFACE ===============================================================

// CredentialInterface is the entity form of a node's stored secrets —
// one entity per (workflow, node) pair carrying a key→value map. The
// workflow document keeps only CredentialSentinel markers; the real
// values live here and are merged back into node.Data at deploy time
// (Node-RED flows_cred.json convention).
type CredentialInterface interface {
	GetID() string
	SetID(id string) CredentialInterface

	GetWorkflowID() string
	SetWorkflowID(workflowID string) CredentialInterface

	GetNodeID() string
	SetNodeID(nodeID string) CredentialInterface

	// GetValues returns the stored key→value credential map.
	GetValues() map[string]string
	SetValues(values map[string]string) CredentialInterface
	// GetValue returns one stored credential value ("" when unset).
	GetValue(key string) string
	// SetValue upserts one credential key; an empty value removes it.
	SetValue(key, value string) CredentialInterface

	GetCreatedAt() time.Time
	SetCreatedAt(createdAt time.Time) CredentialInterface

	GetUpdatedAt() time.Time
	SetUpdatedAt(updatedAt time.Time) CredentialInterface
}

// == IMPLEMENTATION ==========================================================

// credentialManagedAttributes lists the attribute keys a credential
// entity owns.
var credentialManagedAttributes = map[string]bool{
	ATTRIBUTE_WORKFLOW_ID: true,
	ATTRIBUTE_NODE_ID:     true,
	ATTRIBUTE_DATA:        true,
}

type credentialImplementation struct {
	entity     entitystore.EntityInterface
	workflowID string
	nodeID     string
	values     map[string]string
}

// NewCredential creates a new credential entity for a node. The ID may
// be left empty (entitystore assigns one); callers that need a
// deterministic handle (workflowID:nodeID) can SetID themselves.
func NewCredential(workflowID, nodeID string) CredentialInterface {
	entity := entitystore.NewEntity()
	entity.SetType(EntityTypeCredential)
	return &credentialImplementation{
		entity:     entity,
		workflowID: workflowID,
		nodeID:     nodeID,
		values:     map[string]string{},
	}
}

// credentialFromEntity rebuilds a credential from a persisted entity
// and its attributes.
func credentialFromEntity(entity entitystore.EntityInterface, attrs map[string]string) (CredentialInterface, error) {
	c := &credentialImplementation{
		entity:     entity,
		workflowID: attrs[ATTRIBUTE_WORKFLOW_ID],
		nodeID:     attrs[ATTRIBUTE_NODE_ID],
		values:     map[string]string{},
	}
	if raw := attrs[ATTRIBUTE_DATA]; raw != "" {
		if err := json.Unmarshal([]byte(raw), &c.values); err != nil {
			return nil, err
		}
	}
	return c, nil
}

// attributes serializes the credential's domain fields.
func (c *credentialImplementation) attributes() (map[string]string, error) {
	data, err := json.Marshal(c.values)
	if err != nil {
		return nil, err
	}
	return map[string]string{
		ATTRIBUTE_WORKFLOW_ID: c.workflowID,
		ATTRIBUTE_NODE_ID:     c.nodeID,
		ATTRIBUTE_DATA:        string(data),
	}, nil
}

func (c *credentialImplementation) GetID() string { return c.entity.GetID() }
func (c *credentialImplementation) SetID(id string) CredentialInterface {
	c.entity.SetID(id)
	return c
}

func (c *credentialImplementation) GetWorkflowID() string { return c.workflowID }
func (c *credentialImplementation) SetWorkflowID(workflowID string) CredentialInterface {
	c.workflowID = workflowID
	return c
}

func (c *credentialImplementation) GetNodeID() string { return c.nodeID }
func (c *credentialImplementation) SetNodeID(nodeID string) CredentialInterface {
	c.nodeID = nodeID
	return c
}

func (c *credentialImplementation) GetValues() map[string]string { return c.values }
func (c *credentialImplementation) SetValues(values map[string]string) CredentialInterface {
	if values == nil {
		values = map[string]string{}
	}
	c.values = values
	return c
}
func (c *credentialImplementation) GetValue(key string) string { return c.values[key] }
func (c *credentialImplementation) SetValue(key, value string) CredentialInterface {
	if value == "" {
		delete(c.values, key)
		return c
	}
	c.values[key] = value
	return c
}

func (c *credentialImplementation) GetCreatedAt() time.Time {
	t, _ := parseDatetime(c.entity.GetCreatedAt())
	return t
}
func (c *credentialImplementation) SetCreatedAt(createdAt time.Time) CredentialInterface {
	c.entity.SetCreatedAt(formatDatetime(createdAt))
	return c
}

func (c *credentialImplementation) GetUpdatedAt() time.Time {
	t, _ := parseDatetime(c.entity.GetUpdatedAt())
	return t
}
func (c *credentialImplementation) SetUpdatedAt(updatedAt time.Time) CredentialInterface {
	c.entity.SetUpdatedAt(formatDatetime(updatedAt))
	return c
}
