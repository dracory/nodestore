package nodestore

import (
	"context"
	"errors"
	"sort"

	"github.com/dracory/entitystore"
)

func (st *Store) EdgeCount(ctx context.Context, query EdgeQueryInterface) (int64, error) {
	if query == nil {
		return 0, errors.New("edge count: query is nil")
	}
	if query.HasWorkflowID() {
		list, err := st.EdgeList(ctx, query.WithCountOnly(true))
		if err != nil {
			return 0, err
		}
		return int64(len(list)), nil
	}
	entityQuery := entitystore.EntityQuery().WithEntityType(EntityTypeEdge)
	if query.HasID() {
		entityQuery.WithID(query.GetID())
	}
	return st.entityStore.EntityCount(ctx, entityQuery)
}

func (st *Store) EdgeCreate(ctx context.Context, edge EdgeInterface) error {
	if edge == nil {
		return errors.New("edge create: edge is nil")
	}
	impl, ok := edge.(*edgeImplementation)
	if !ok {
		return errors.New("edge create: unsupported implementation")
	}
	if err := st.entityStore.EntityCreate(ctx, impl.entity); err != nil {
		return err
	}
	return st.entityStore.AttributesSet(ctx, impl.entity.GetID(), impl.attributes())
}

func (st *Store) EdgeDeleteByID(ctx context.Context, id string) error {
	_, err := st.entityStore.EntityDelete(ctx, id)
	return err
}

func (st *Store) EdgeFindByID(ctx context.Context, id string) (EdgeInterface, error) {
	entity, err := st.entityStore.EntityFindByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if entity == nil || entity.GetType() != EntityTypeEdge {
		return nil, nil
	}
	attrs, err := st.entityAttributes(ctx, entity.GetID())
	if err != nil {
		return nil, err
	}
	return edgeFromEntity(entity, attrs)
}

// EdgeList mirrors NodeList — WithWorkflowID goes through
// EntityListByAttribute (entities×attributes JOIN on workflow_id).
func (st *Store) EdgeList(ctx context.Context, query EdgeQueryInterface) ([]EdgeInterface, error) {
	if query == nil {
		query = EdgeQuery()
	}
	if err := query.Validate(); err != nil {
		return nil, err
	}

	var entities []entitystore.EntityInterface
	var err error
	if query.HasWorkflowID() {
		entities, err = st.entityStore.EntityListByAttribute(ctx,
			EntityTypeEdge, ATTRIBUTE_WORKFLOW_ID, query.GetWorkflowID())
	} else {
		entityQuery := entitystore.EntityQuery().WithEntityType(EntityTypeEdge)
		if query.HasID() {
			entityQuery.WithID(query.GetID())
		}
		entityQuery.WithPrefetchAttributes()
		entities, err = st.entityStore.EntityList(ctx, entityQuery)
	}
	if err != nil {
		return nil, err
	}

	matched := make([]EdgeInterface, 0, len(entities))
	for _, entity := range entities {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
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
		edge, err := edgeFromEntity(entity, attrs)
		if err != nil {
			return nil, err
		}
		matched = append(matched, edge)
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

func (st *Store) EdgeUpdate(ctx context.Context, edge EdgeInterface) error {
	if edge == nil {
		return errors.New("edge update: edge is nil")
	}
	impl, ok := edge.(*edgeImplementation)
	if !ok {
		return errors.New("edge update: unsupported implementation")
	}
	if err := st.entityStore.EntityUpdate(ctx, impl.entity); err != nil {
		return err
	}
	return st.entityAttributesReplace(ctx, impl.entity.GetID(), impl.attributes(), edgeManagedAttributes)
}
