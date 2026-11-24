package nodestore

import (
	"context"
	"errors"
	"sort"

	"github.com/dracory/entitystore"
)

func (st *Store) MessageCount(ctx context.Context, query MessageQueryInterface) (int64, error) {
	if query == nil {
		return 0, errors.New("message count: query is nil")
	}
	entityQuery := entitystore.EntityQuery().WithEntityType(EntityTypeMessage)
	if query.HasID() {
		entityQuery.WithID(query.GetID())
	}
	return st.entityStore.EntityCount(ctx, entityQuery)
}

func (st *Store) MessageCreate(ctx context.Context, message MessageInterface) error {
	if message == nil {
		return errors.New("message create: message is nil")
	}
	impl, ok := message.(*messageImplementation)
	if !ok {
		return errors.New("message create: unsupported implementation")
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

func (st *Store) MessageDeleteByID(ctx context.Context, id string) error {
	_, err := st.entityStore.EntityDelete(ctx, id)
	return err
}

func (st *Store) MessageFindByID(ctx context.Context, id string) (MessageInterface, error) {
	entity, err := st.entityStore.EntityFindByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if entity == nil || entity.GetType() != EntityTypeMessage {
		return nil, nil
	}
	attrs, err := st.entityAttributes(ctx, entity.GetID())
	if err != nil {
		return nil, err
	}
	return messageFromEntity(entity, attrs)
}

func (st *Store) MessageList(ctx context.Context, query MessageQueryInterface) ([]MessageInterface, error) {
	if query == nil {
		query = MessageQuery()
	}
	if err := query.Validate(); err != nil {
		return nil, err
	}

	entityQuery := entitystore.EntityQuery().WithEntityType(EntityTypeMessage)
	if query.HasID() {
		entityQuery.WithID(query.GetID())
	}
	entityQuery.WithPrefetchAttributes()

	entities, err := st.entityStore.EntityList(ctx, entityQuery)
	if err != nil {
		return nil, err
	}

	matched := make([]MessageInterface, 0, len(entities))
	for _, entity := range entities {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		message, err := messageFromEntity(entity, entity.GetTempKeys())
		if err != nil {
			return nil, err
		}
		matched = append(matched, message)
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

func (st *Store) MessageUpdate(ctx context.Context, message MessageInterface) error {
	if message == nil {
		return errors.New("message update: message is nil")
	}
	impl, ok := message.(*messageImplementation)
	if !ok {
		return errors.New("message update: unsupported implementation")
	}
	attrs, err := impl.attributes()
	if err != nil {
		return err
	}
	if err := st.entityStore.EntityUpdate(ctx, impl.entity); err != nil {
		return err
	}
	return st.entityAttributesReplace(ctx, impl.entity.GetID(), attrs, messageManagedAttributes)
}
