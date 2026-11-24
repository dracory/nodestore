package nodestore

import (
	"context"
	"errors"
	"sort"

	"github.com/dracory/entitystore"
)

func (st *Store) CredentialCount(ctx context.Context, query CredentialQueryInterface) (int64, error) {
	if query == nil {
		return 0, errors.New("credential count: query is nil")
	}
	if query.HasWorkflowID() {
		list, err := st.CredentialList(ctx, query.WithCountOnly(true))
		if err != nil {
			return 0, err
		}
		return int64(len(list)), nil
	}
	entityQuery := entitystore.EntityQuery().WithEntityType(EntityTypeCredential)
	if query.HasID() {
		entityQuery.WithID(query.GetID())
	}
	return st.entityStore.EntityCount(ctx, entityQuery)
}

func (st *Store) CredentialCreate(ctx context.Context, credential CredentialInterface) error {
	if credential == nil {
		return errors.New("credential create: credential is nil")
	}
	impl, ok := credential.(*credentialImplementation)
	if !ok {
		return errors.New("credential create: unsupported implementation")
	}
	attrs, err := impl.attributes()
	if err != nil {
		return err
	}
	if err := st.entityStore.EntityCreate(ctx, impl.entity); err != nil {
		return err
	}
	return st.entityStore.AttributesSet(ctx, impl.entity.GetID(), attrs)
}

func (st *Store) CredentialDeleteByID(ctx context.Context, id string) error {
	_, err := st.entityStore.EntityDelete(ctx, id)
	return err
}

// CredentialDeleteByWorkflowID drops every credential entity owned by
// the workflow (called when the workflow is deleted).
func (st *Store) CredentialDeleteByWorkflowID(ctx context.Context, workflowID string) error {
	list, err := st.CredentialList(ctx, CredentialQuery().WithWorkflowID(workflowID))
	if err != nil {
		return err
	}
	for _, c := range list {
		if err := st.CredentialDeleteByID(ctx, c.GetID()); err != nil {
			return err
		}
	}
	return nil
}

func (st *Store) CredentialFindByID(ctx context.Context, id string) (CredentialInterface, error) {
	entity, err := st.entityStore.EntityFindByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if entity == nil || entity.GetType() != EntityTypeCredential {
		return nil, nil
	}
	attrs, err := st.entityAttributes(ctx, entity.GetID())
	if err != nil {
		return nil, err
	}
	return credentialFromEntity(entity, attrs)
}

// CredentialFindByNode returns the single credential entity for a
// (workflow, node) pair, or nil when none is stored.
func (st *Store) CredentialFindByNode(ctx context.Context, workflowID, nodeID string) (CredentialInterface, error) {
	list, err := st.CredentialList(ctx, CredentialQuery().
		WithWorkflowID(workflowID).WithNodeID(nodeID))
	if err != nil {
		return nil, err
	}
	if len(list) == 0 {
		return nil, nil
	}
	return list[0], nil
}

// CredentialList lists credential entities. WithWorkflowID uses
// EntityListByAttribute (the single attribute-value lookup the store
// offers — see NodeList); node filtering, limit/offset and sort are
// applied in memory.
func (st *Store) CredentialList(ctx context.Context, query CredentialQueryInterface) ([]CredentialInterface, error) {
	if query == nil {
		query = CredentialQuery()
	}
	if err := query.Validate(); err != nil {
		return nil, err
	}

	var entities []entitystore.EntityInterface
	var err error
	if query.HasWorkflowID() {
		entities, err = st.entityStore.EntityListByAttribute(ctx,
			EntityTypeCredential, ATTRIBUTE_WORKFLOW_ID, query.GetWorkflowID())
	} else {
		entityQuery := entitystore.EntityQuery().WithEntityType(EntityTypeCredential)
		if query.HasID() {
			entityQuery.WithID(query.GetID())
		}
		entityQuery.WithPrefetchAttributes()
		entities, err = st.entityStore.EntityList(ctx, entityQuery)
	}
	if err != nil {
		return nil, err
	}

	matched := make([]CredentialInterface, 0, len(entities))
	for _, entity := range entities {
		if query.HasID() && entity.GetID() != query.GetID() {
			continue
		}
		attrs := entity.GetTempKeys()
		if len(attrs) == 0 {
			attrs, err = st.entityAttributes(ctx, entity.GetID())
			if err != nil {
				return nil, err
			}
		}
		if query.HasNodeID() && attrs[ATTRIBUTE_NODE_ID] != query.GetNodeID() {
			continue
		}
		credential, err := credentialFromEntity(entity, attrs)
		if err != nil {
			return nil, err
		}
		matched = append(matched, credential)
	}

	sort.SliceStable(matched, func(i, j int) bool {
		if query.GetSortOrder() == DESC {
			return matched[i].GetCreatedAt().After(matched[j].GetCreatedAt())
		}
		return matched[i].GetCreatedAt().Before(matched[j].GetCreatedAt())
	})

	if query.HasOffset() && query.GetOffset() < uint64(len(matched)) {
		matched = matched[query.GetOffset():]
	}
	if query.HasLimit() && query.GetLimit() < uint64(len(matched)) {
		matched = matched[:query.GetLimit()]
	}
	return matched, nil
}

func (st *Store) CredentialUpdate(ctx context.Context, credential CredentialInterface) error {
	if credential == nil {
		return errors.New("credential update: credential is nil")
	}
	impl, ok := credential.(*credentialImplementation)
	if !ok {
		return errors.New("credential update: unsupported implementation")
	}
	attrs, err := impl.attributes()
	if err != nil {
		return err
	}
	if err := st.entityStore.EntityUpdate(ctx, impl.entity); err != nil {
		return err
	}
	return st.entityAttributesReplace(ctx, impl.entity.GetID(), attrs, credentialManagedAttributes)
}

// CredentialMapForWorkflow returns the workflow's stored credentials in
// the deploy-time shape nodeID → key → value — the single read the
// engine's credential provider needs.
func (st *Store) CredentialMapForWorkflow(ctx context.Context, workflowID string) (map[string]map[string]string, error) {
	list, err := st.CredentialList(ctx, CredentialQuery().WithWorkflowID(workflowID))
	if err != nil {
		return nil, err
	}
	out := map[string]map[string]string{}
	for _, c := range list {
		if len(c.GetValues()) == 0 {
			continue
		}
		out[c.GetNodeID()] = c.GetValues()
	}
	return out, nil
}

// CredentialReplaceForWorkflow bulk-replaces a workflow's credential
// entities from a nodeID → key → value map: entities not present in
// the map are deleted, changed ones updated, new ones created. Used by
// the workflow save path after extractWorkflowCredentials moves the
// secrets out of node.Data.
func (st *Store) CredentialReplaceForWorkflow(ctx context.Context, workflowID string, creds map[string]map[string]string) error {
	existing, err := st.CredentialList(ctx, CredentialQuery().WithWorkflowID(workflowID))
	if err != nil {
		return err
	}
	for _, c := range existing {
		nodeID := c.GetNodeID()
		values, keep := creds[nodeID]
		if !keep || len(values) == 0 {
			if err := st.CredentialDeleteByID(ctx, c.GetID()); err != nil {
				return err
			}
			continue
		}
		c.SetValues(values)
		if err := st.CredentialUpdate(ctx, c); err != nil {
			return err
		}
		delete(creds, nodeID)
	}
	for nodeID, values := range creds {
		if len(values) == 0 {
			continue
		}
		if err := st.CredentialCreate(ctx, NewCredential(workflowID, nodeID).SetValues(values)); err != nil {
			return err
		}
	}
	return nil
}
