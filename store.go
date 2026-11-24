package nodestore

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"os"

	"github.com/dracory/entitystore"
)

// == INTERFACE ===============================================================

// StoreInterface is the nodestore contract — entitystore-backed persistence
// for the flowrunner domain: workflows (with nodes and edges) and messages.
type StoreInterface interface {
	// GetEntityStore returns the underlying entity store
	GetEntityStore() entitystore.StoreInterface

	// EnableDebug enables debug mode
	EnableDebug(debugEnabled bool) StoreInterface

	// == Workflow Methods ==

	WorkflowCount(ctx context.Context, query WorkflowQueryInterface) (int64, error)
	WorkflowCreate(ctx context.Context, workflow WorkflowInterface) error
	WorkflowDeleteByID(ctx context.Context, id string) error
	WorkflowFindByID(ctx context.Context, id string) (WorkflowInterface, error)
	WorkflowFindByName(ctx context.Context, name string) (WorkflowInterface, error)
	WorkflowList(ctx context.Context, query WorkflowQueryInterface) ([]WorkflowInterface, error)
	WorkflowTrashByID(ctx context.Context, id string) error
	WorkflowUpdate(ctx context.Context, workflow WorkflowInterface) error

	// == Node Methods ==

	NodeCount(ctx context.Context, query NodeQueryInterface) (int64, error)
	NodeCreate(ctx context.Context, node NodeInterface) error
	NodeDeleteByID(ctx context.Context, id string) error
	NodeFindByID(ctx context.Context, id string) (NodeInterface, error)
	NodeList(ctx context.Context, query NodeQueryInterface) ([]NodeInterface, error)
	NodeUpdate(ctx context.Context, node NodeInterface) error

	// == Edge Methods ==

	EdgeCount(ctx context.Context, query EdgeQueryInterface) (int64, error)
	EdgeCreate(ctx context.Context, edge EdgeInterface) error
	EdgeDeleteByID(ctx context.Context, id string) error
	EdgeFindByID(ctx context.Context, id string) (EdgeInterface, error)
	EdgeList(ctx context.Context, query EdgeQueryInterface) ([]EdgeInterface, error)
	EdgeUpdate(ctx context.Context, edge EdgeInterface) error

	// == Message Methods ==

	MessageCount(ctx context.Context, query MessageQueryInterface) (int64, error)
	MessageCreate(ctx context.Context, message MessageInterface) error
	MessageDeleteByID(ctx context.Context, id string) error
	MessageFindByID(ctx context.Context, id string) (MessageInterface, error)
	MessageList(ctx context.Context, query MessageQueryInterface) ([]MessageInterface, error)
	MessageUpdate(ctx context.Context, message MessageInterface) error

	// == Credential Methods ==

	CredentialCount(ctx context.Context, query CredentialQueryInterface) (int64, error)
	CredentialCreate(ctx context.Context, credential CredentialInterface) error
	CredentialDeleteByID(ctx context.Context, id string) error
	CredentialDeleteByWorkflowID(ctx context.Context, workflowID string) error
	CredentialFindByID(ctx context.Context, id string) (CredentialInterface, error)
	CredentialFindByNode(ctx context.Context, workflowID, nodeID string) (CredentialInterface, error)
	CredentialList(ctx context.Context, query CredentialQueryInterface) ([]CredentialInterface, error)
	CredentialUpdate(ctx context.Context, credential CredentialInterface) error
	// CredentialMapForWorkflow returns nodeID → key → value — the
	// deploy-time credential injection map.
	CredentialMapForWorkflow(ctx context.Context, workflowID string) (map[string]map[string]string, error)
	// CredentialReplaceForWorkflow bulk-replaces a workflow's stored
	// credentials (save path, after extraction).
	CredentialReplaceForWorkflow(ctx context.Context, workflowID string, creds map[string]map[string]string) error

	// == Deploy Record Methods ==

	DeployRecordCount(ctx context.Context, query DeployRecordQueryInterface) (int64, error)
	// DeployRecordCreate appends an immutable record and prunes the
	// workflow's history beyond DeployRecordRetention.
	DeployRecordCreate(ctx context.Context, record DeployRecordInterface) error
	DeployRecordDeleteByID(ctx context.Context, id string) error
	DeployRecordDeleteByWorkflowID(ctx context.Context, workflowID string) error
	DeployRecordFindByID(ctx context.Context, id string) (DeployRecordInterface, error)
	DeployRecordList(ctx context.Context, query DeployRecordQueryInterface) ([]DeployRecordInterface, error)

	// == Node Group Methods (editor view groups) ==

	NodeGroupCount(ctx context.Context, query NodeGroupQueryInterface) (int64, error)
	NodeGroupCreate(ctx context.Context, group NodeGroupInterface) error
	NodeGroupDeleteByID(ctx context.Context, id string) error
	NodeGroupFindByID(ctx context.Context, id string) (NodeGroupInterface, error)
	NodeGroupList(ctx context.Context, query NodeGroupQueryInterface) ([]NodeGroupInterface, error)
	NodeGroupUpdate(ctx context.Context, group NodeGroupInterface) error

	// == Workflow Group Methods (sidebar grouping) ==

	WorkflowGroupCount(ctx context.Context, query WorkflowGroupQueryInterface) (int64, error)
	WorkflowGroupCreate(ctx context.Context, group WorkflowGroupInterface) error
	WorkflowGroupDeleteByID(ctx context.Context, id string) error
	WorkflowGroupFindByID(ctx context.Context, id string) (WorkflowGroupInterface, error)
	WorkflowGroupList(ctx context.Context, query WorkflowGroupQueryInterface) ([]WorkflowGroupInterface, error)
	WorkflowGroupUpdate(ctx context.Context, group WorkflowGroupInterface) error

	// == Config Methods ==

	ConfigGet(ctx context.Context, key string) (string, error)
	ConfigSet(ctx context.Context, key string, value string) error
}

// == IMPLEMENTATION ==========================================================

// Store is the nodestore implementation backed by an entity store
type Store struct {
	entityStore  entitystore.StoreInterface
	debugEnabled bool
	logger       *slog.Logger
}

// NewStoreOptions define the options for creating a new node store
type NewStoreOptions struct {
	// EntityStore is the underlying storage (required)
	EntityStore entitystore.StoreInterface
	// DebugEnabled enables debug logging
	DebugEnabled bool
}

// NewStore creates a new node store on top of an entity store
func NewStore(opts NewStoreOptions) (*Store, error) {
	if opts.EntityStore == nil {
		return nil, errors.New("node store: EntityStore is required")
	}

	return &Store{
		entityStore:  opts.EntityStore,
		debugEnabled: opts.DebugEnabled,
		logger:       slog.New(slog.NewTextHandler(os.Stdout, nil)),
	}, nil
}

func (st *Store) GetEntityStore() entitystore.StoreInterface {
	return st.entityStore
}

func (st *Store) GetDB() *sql.DB {
	return st.entityStore.GetDB()
}

func (st *Store) EnableDebug(debugEnabled bool) StoreInterface {
	st.debugEnabled = debugEnabled
	return st
}

// entityAttributes loads all attributes of an entity into a flat map
func (st *Store) entityAttributes(ctx context.Context, entityID string) (map[string]string, error) {
	list, err := st.entityStore.EntityAttributeList(ctx, entityID)
	if err != nil {
		return nil, err
	}
	attrs := map[string]string{}
	for _, attr := range list {
		attrs[attr.GetKey()] = attr.GetValue()
	}
	return attrs, nil
}

// attributeDeleter is implemented by entitystore's store but not declared
// on its StoreInterface — asserted structurally
type attributeDeleter interface {
	AttributeDelete(ctx context.Context, id string) error
}

// entityAttributesReplace upserts attrs and removes stale keys — but
// ONLY keys listed in managedKeys. Attributes owned by other writers
// (not produced by the object's attributes() method) are left
// untouched; previously any unknown persisted key was wiped.
// When the entity store cannot delete attributes, stale managed keys
// are reset to empty instead.
func (st *Store) entityAttributesReplace(ctx context.Context, entityID string, attrs map[string]string, managedKeys map[string]bool) error {
	existing, err := st.entityStore.EntityAttributeList(ctx, entityID)
	if err != nil {
		return err
	}
	deleter, canDelete := st.entityStore.(attributeDeleter)
	for _, attr := range existing {
		key := attr.GetKey()
		if _, keep := attrs[key]; keep {
			continue
		}
		if !managedKeys[key] {
			continue // not owned by this object — leave it alone
		}
		if canDelete {
			if err := deleter.AttributeDelete(ctx, attr.GetID()); err != nil {
				return err
			}
		} else {
			attrs[key] = ""
		}
	}
	return st.entityStore.AttributesSet(ctx, entityID, attrs)
}
