package nodestore

import (
	"context"
	"errors"
	"sort"

	"github.com/dracory/entitystore"
)

func (st *Store) NodeCount(ctx context.Context, query NodeQueryInterface) (int64, error) {
	if query == nil {
		return 0, errors.New("node count: query is nil")
	}
	// entitystore cannot count entities by attribute value in one call —
	// when WithWorkflowID is set, count via the listing (same JOIN query
	// EntityListByAttribute uses, just counted in memory).
	if query.HasWorkflowID() {
		list, err := st.NodeList(ctx, query.WithCountOnly(true))
		if err != nil {
			return 0, err
		}
		return int64(len(list)), nil
	}
	entityQuery := entitystore.EntityQuery().WithEntityType(EntityTypeNode)
	if query.HasID() {
		entityQuery.WithID(query.GetID())
	}
	return st.entityStore.EntityCount(ctx, entityQuery)
}

func (st *Store) NodeCreate(ctx context.Context, node NodeInterface) error {
	if node == nil {
		return errors.New("node create: node is nil")
	}
	impl, ok := node.(*nodeImplementation)
	if !ok {
		return errors.New("node create: unsupported implementation")
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

func (st *Store) NodeDeleteByID(ctx context.Context, id string) error {
	_, err := st.entityStore.EntityDelete(ctx, id)
	return err
}

func (st *Store) NodeFindByID(ctx context.Context, id string) (NodeInterface, error) {
	entity, err := st.entityStore.EntityFindByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if entity == nil || entity.GetType() != EntityTypeNode {
		return nil, nil
	}
	attrs, err := st.entityAttributes(ctx, entity.GetID())
	if err != nil {
		return nil, err
	}
	return nodeFromEntity(entity, attrs)
}

// NodeList lists node entities. WithWorkflowID uses
// entitystore.EntityListByAttribute — a single entities×attributes JOIN
// on entity type + workflow_id attribute (AttributeQuery has no value
// filter, so this is the only attribute-value lookup the store offers).
// Limit/offset/sort are applied in memory, consistent with WorkflowList.
func (st *Store) NodeList(ctx context.Context, query NodeQueryInterface) ([]NodeInterface, error) {
	if query == nil {
		query = NodeQuery()
	}
	if err := query.Validate(); err != nil {
		return nil, err
	}

	var entities []entitystore.EntityInterface
	var err error
	if query.HasWorkflowID() {
		entities, err = st.entityStore.EntityListByAttribute(ctx,
			EntityTypeNode, ATTRIBUTE_WORKFLOW_ID, query.GetWorkflowID())
	} else {
		entityQuery := entitystore.EntityQuery().WithEntityType(EntityTypeNode)
		if query.HasID() {
			entityQuery.WithID(query.GetID())
		}
		entityQuery.WithPrefetchAttributes()
		entities, err = st.entityStore.EntityList(ctx, entityQuery)
	}
	if err != nil {
		return nil, err
	}

	matched := make([]NodeInterface, 0, len(entities))
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
		node, err := nodeFromEntity(entity, attrs)
		if err != nil {
			return nil, err
		}
		matched = append(matched, node)
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

func (st *Store) NodeUpdate(ctx context.Context, node NodeInterface) error {
	if node == nil {
		return errors.New("node update: node is nil")
	}
	impl, ok := node.(*nodeImplementation)
	if !ok {
		return errors.New("node update: unsupported implementation")
	}
	attrs, err := impl.attributes()
	if err != nil {
		return err
	}
	if err := st.entityStore.EntityUpdate(ctx, impl.entity); err != nil {
		return err
	}
	return st.entityAttributesReplace(ctx, impl.entity.GetID(), attrs, nodeManagedAttributes)
}
