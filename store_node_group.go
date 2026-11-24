package nodestore

import (
	"context"
	"errors"
	"sort"

	"github.com/dracory/entitystore"
)

func (st *Store) NodeGroupCount(ctx context.Context, query NodeGroupQueryInterface) (int64, error) {
	if query == nil {
		return 0, errors.New("node group count: query is nil")
	}
	if query.HasWorkflowID() {
		list, err := st.NodeGroupList(ctx, query.WithCountOnly(true))
		if err != nil {
			return 0, err
		}
		return int64(len(list)), nil
	}
	entityQuery := entitystore.EntityQuery().WithEntityType(EntityTypeNodeGroup)
	if query.HasID() {
		entityQuery.WithID(query.GetID())
	}
	return st.entityStore.EntityCount(ctx, entityQuery)
}

func (st *Store) NodeGroupCreate(ctx context.Context, group NodeGroupInterface) error {
	if group == nil {
		return errors.New("node group create: group is nil")
	}
	impl, ok := group.(*nodeGroupImplementation)
	if !ok {
		return errors.New("node group create: unsupported implementation")
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

func (st *Store) NodeGroupDeleteByID(ctx context.Context, id string) error {
	_, err := st.entityStore.EntityDelete(ctx, id)
	return err
}

func (st *Store) NodeGroupFindByID(ctx context.Context, id string) (NodeGroupInterface, error) {
	entity, err := st.entityStore.EntityFindByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if entity == nil || entity.GetType() != EntityTypeNodeGroup {
		return nil, nil
	}
	attrs, err := st.entityAttributes(ctx, entity.GetID())
	if err != nil {
		return nil, err
	}
	return nodeGroupFromEntity(entity, attrs)
}

func (st *Store) NodeGroupList(ctx context.Context, query NodeGroupQueryInterface) ([]NodeGroupInterface, error) {
	if query == nil {
		query = NodeGroupQuery()
	}
	if err := query.Validate(); err != nil {
		return nil, err
	}

	var entities []entitystore.EntityInterface
	var err error
	if query.HasWorkflowID() {
		entities, err = st.entityStore.EntityListByAttribute(ctx,
			EntityTypeNodeGroup, ATTRIBUTE_WORKFLOW_ID, query.GetWorkflowID())
	} else {
		entityQuery := entitystore.EntityQuery().WithEntityType(EntityTypeNodeGroup)
		if query.HasID() {
			entityQuery.WithID(query.GetID())
		}
		entityQuery.WithPrefetchAttributes()
		entities, err = st.entityStore.EntityList(ctx, entityQuery)
	}
	if err != nil {
		return nil, err
	}

	matched := make([]NodeGroupInterface, 0, len(entities))
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
		group, err := nodeGroupFromEntity(entity, attrs)
		if err != nil {
			return nil, err
		}
		matched = append(matched, group)
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

func (st *Store) NodeGroupUpdate(ctx context.Context, group NodeGroupInterface) error {
	if group == nil {
		return errors.New("node group update: group is nil")
	}
	impl, ok := group.(*nodeGroupImplementation)
	if !ok {
		return errors.New("node group update: unsupported implementation")
	}
	attrs, err := impl.attributes()
	if err != nil {
		return err
	}
	if err := st.entityStore.EntityUpdate(ctx, impl.entity); err != nil {
		return err
	}
	return st.entityAttributesReplace(ctx, impl.entity.GetID(), attrs, nodeGroupManagedAttributes)
}
