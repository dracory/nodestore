package nodestore

import (
	"context"
	"errors"
	"sort"

	"github.com/dracory/entitystore"
)

func (st *Store) WorkflowGroupCount(ctx context.Context, query WorkflowGroupQueryInterface) (int64, error) {
	if query == nil {
		return 0, errors.New("workflow group count: query is nil")
	}
	entityQuery := entitystore.EntityQuery().WithEntityType(EntityTypeWorkflowGroup)
	if query.HasID() {
		entityQuery.WithID(query.GetID())
	}
	return st.entityStore.EntityCount(ctx, entityQuery)
}

func (st *Store) WorkflowGroupCreate(ctx context.Context, group WorkflowGroupInterface) error {
	if group == nil {
		return errors.New("workflow group create: group is nil")
	}
	impl, ok := group.(*workflowGroupImplementation)
	if !ok {
		return errors.New("workflow group create: unsupported implementation")
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

func (st *Store) WorkflowGroupDeleteByID(ctx context.Context, id string) error {
	_, err := st.entityStore.EntityDelete(ctx, id)
	return err
}

func (st *Store) WorkflowGroupFindByID(ctx context.Context, id string) (WorkflowGroupInterface, error) {
	entity, err := st.entityStore.EntityFindByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if entity == nil || entity.GetType() != EntityTypeWorkflowGroup {
		return nil, nil
	}
	attrs, err := st.entityAttributes(ctx, entity.GetID())
	if err != nil {
		return nil, err
	}
	return workflowGroupFromEntity(entity, attrs)
}

func (st *Store) WorkflowGroupList(ctx context.Context, query WorkflowGroupQueryInterface) ([]WorkflowGroupInterface, error) {
	if query == nil {
		query = WorkflowGroupQuery()
	}
	if err := query.Validate(); err != nil {
		return nil, err
	}

	entityQuery := entitystore.EntityQuery().WithEntityType(EntityTypeWorkflowGroup)
	if query.HasID() {
		entityQuery.WithID(query.GetID())
	}
	entityQuery.WithPrefetchAttributes()
	entities, err := st.entityStore.EntityList(ctx, entityQuery)
	if err != nil {
		return nil, err
	}

	matched := make([]WorkflowGroupInterface, 0, len(entities))
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
		if query.HasParentID() && attrs[ATTRIBUTE_PARENT_ID] != query.GetParentID() {
			continue
		}
		group, err := workflowGroupFromEntity(entity, attrs)
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

func (st *Store) WorkflowGroupUpdate(ctx context.Context, group WorkflowGroupInterface) error {
	if group == nil {
		return errors.New("workflow group update: group is nil")
	}
	impl, ok := group.(*workflowGroupImplementation)
	if !ok {
		return errors.New("workflow group update: unsupported implementation")
	}
	attrs, err := impl.attributes()
	if err != nil {
		return err
	}
	if err := st.entityStore.EntityUpdate(ctx, impl.entity); err != nil {
		return err
	}
	return st.entityAttributesReplace(ctx, impl.entity.GetID(), attrs, workflowGroupManagedAttributes)
}
