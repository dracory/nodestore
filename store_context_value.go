package nodestore

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"sort"

	"github.com/dracory/entitystore"
)

// ContextValueStoreInterface is the narrow persistence contract for
// runtime context values. Values are opaque serialized strings here —
// the runtime-facing adapter owns JSON encoding and Node-RED scope
// semantics, while nodestore owns durable storage and workflow-owned
// cleanup. The interface is kept separate from StoreInterface so
// embedders with custom stores can opt in structurally.
type ContextValueStoreInterface interface {
	ContextValueGet(ctx context.Context, scope, scopeID, key string) (value string, ok bool, err error)
	ContextValueSet(ctx context.Context, workflowID, scope, scopeID, key, value string) error
	ContextValueDelete(ctx context.Context, scope, scopeID, key string) error
	ContextValueDeleteScope(ctx context.Context, scope, scopeID string) error
	ContextValueKeys(ctx context.Context, scope, scopeID string) ([]string, error)
	ContextValueDeleteByWorkflowID(ctx context.Context, workflowID string) error
}

var contextValueManagedAttributes = map[string]bool{
	ATTRIBUTE_WORKFLOW_ID: true,
	ATTRIBUTE_SCOPE:       true,
	ATTRIBUTE_SCOPE_ID:    true,
	ATTRIBUTE_CONTEXT_KEY: true,
	ATTRIBUTE_VALUE:       true,
}

// contextValueEntityID returns the deterministic entity ID for one
// context key. JSON array encoding avoids delimiter ambiguity between
// the three parts; the SHA-256 prefix keeps IDs inside the entitystore
// 40-character column regardless of workflow/node/key length.
func contextValueEntityID(scope, scopeID, key string) string {
	raw, _ := json.Marshal([]string{scope, scopeID, key})
	sum := sha256.Sum256(raw)
	return "ctx-" + hex.EncodeToString(sum[:])[:32]
}

func validateContextValueScope(scope string) error {
	if scope == "" {
		return errors.New("context value: scope is required")
	}
	return nil
}

// ContextValueGet returns the serialized value stored under key in a
// scope bucket. ok is false when the key has never been set.
func (st *Store) ContextValueGet(ctx context.Context, scope, scopeID, key string) (string, bool, error) {
	if err := validateContextValueScope(scope); err != nil {
		return "", false, err
	}
	entity, err := st.entityStore.EntityFindByID(ctx, contextValueEntityID(scope, scopeID, key))
	if err != nil {
		return "", false, err
	}
	if entity == nil || entity.GetType() != EntityTypeContextValue {
		return "", false, nil
	}
	attrs, err := st.entityAttributes(ctx, entity.GetID())
	if err != nil {
		return "", false, err
	}
	return attrs[ATTRIBUTE_VALUE], true, nil
}

// ContextValueSet upserts one serialized value. workflowID is stored
// as ownership metadata for cascade deletion; it is empty for values
// not owned by a workflow (notably global context).
func (st *Store) ContextValueSet(ctx context.Context, workflowID, scope, scopeID, key, value string) error {
	if err := validateContextValueScope(scope); err != nil {
		return err
	}
	id := contextValueEntityID(scope, scopeID, key)
	attrs := map[string]string{
		ATTRIBUTE_WORKFLOW_ID: workflowID,
		ATTRIBUTE_SCOPE:       scope,
		ATTRIBUTE_SCOPE_ID:    scopeID,
		ATTRIBUTE_CONTEXT_KEY: key,
		ATTRIBUTE_VALUE:       value,
	}

	entity, err := st.entityStore.EntityFindByID(ctx, id)
	if err != nil {
		return err
	}
	if entity == nil {
		entity = entitystore.NewEntity()
		entity.SetID(id)
		entity.SetType(EntityTypeContextValue)
		entity.SetHandle(id)
		if err := st.entityStore.EntityCreate(ctx, entity); err != nil {
			// Two writers can race the first insert of the same key. The
			// loser re-reads the winner's row and continues as an update.
			existing, findErr := st.entityStore.EntityFindByID(ctx, id)
			if findErr != nil || existing == nil {
				return err
			}
			entity = existing
		}
	}
	if err := st.entityStore.EntityUpdate(ctx, entity); err != nil {
		return err
	}
	return st.entityAttributesReplace(ctx, id, attrs, contextValueManagedAttributes)
}

// ContextValueDelete removes one key. A missing key is not an error.
func (st *Store) ContextValueDelete(ctx context.Context, scope, scopeID, key string) error {
	if err := validateContextValueScope(scope); err != nil {
		return err
	}
	_, err := st.entityStore.EntityDelete(ctx, contextValueEntityID(scope, scopeID, key))
	return err
}

// contextValueEntitiesByScopeID loads candidate entities whose
// scope_id attribute matches exactly, then filters them by scope. The
// entitystore attribute lookup avoids scanning unrelated workflow-owned
// rows; the second filter is needed because scope_id is not unique
// across scopes (notably the empty ID).
func (st *Store) contextValueEntitiesByScopeID(ctx context.Context, scope, scopeID string) ([]entitystore.EntityInterface, error) {
	entities, err := st.entityStore.EntityListByAttribute(ctx, EntityTypeContextValue, ATTRIBUTE_SCOPE_ID, scopeID)
	if err != nil {
		return nil, err
	}
	matched := make([]entitystore.EntityInterface, 0, len(entities))
	for _, entity := range entities {
		attrs, err := st.entityAttributes(ctx, entity.GetID())
		if err != nil {
			return nil, err
		}
		if attrs[ATTRIBUTE_SCOPE] == scope {
			matched = append(matched, entity)
		}
	}
	return matched, nil
}

// ContextValueDeleteScope removes every key in one scope bucket.
func (st *Store) ContextValueDeleteScope(ctx context.Context, scope, scopeID string) error {
	if scope == "" {
		return errors.New("context value delete scope: scope is required")
	}
	entities, err := st.contextValueEntitiesByScopeID(ctx, scope, scopeID)
	if err != nil {
		return err
	}
	for _, entity := range entities {
		if _, err := st.entityStore.EntityDelete(ctx, entity.GetID()); err != nil {
			return err
		}
	}
	return nil
}

// ContextValueKeys lists keys in one scope bucket, sorted for
// deterministic runtime behavior.
func (st *Store) ContextValueKeys(ctx context.Context, scope, scopeID string) ([]string, error) {
	if scope == "" {
		return nil, errors.New("context value keys: scope is required")
	}
	entities, err := st.contextValueEntitiesByScopeID(ctx, scope, scopeID)
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	keys := make([]string, 0, len(entities))
	for _, entity := range entities {
		attrs, err := st.entityAttributes(ctx, entity.GetID())
		if err != nil {
			return nil, err
		}
		key, hasKey := attrs[ATTRIBUTE_CONTEXT_KEY]
		if !hasKey || seen[key] {
			continue
		}
		seen[key] = true
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys, nil
}

// ContextValueDeleteByWorkflowID removes every context value owned by
// a workflow. Global/unowned values have an empty workflow owner and
// are deliberately preserved.
func (st *Store) ContextValueDeleteByWorkflowID(ctx context.Context, workflowID string) error {
	if workflowID == "" {
		return errors.New("context value delete by workflow: workflow id is required")
	}
	entities, err := st.entityStore.EntityListByAttribute(ctx, EntityTypeContextValue, ATTRIBUTE_WORKFLOW_ID, workflowID)
	if err != nil {
		return err
	}
	for _, entity := range entities {
		if _, err := st.entityStore.EntityDelete(ctx, entity.GetID()); err != nil {
			return err
		}
	}
	return nil
}

var _ ContextValueStoreInterface = (*Store)(nil)
