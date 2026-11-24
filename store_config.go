package nodestore

import (
	"context"
	"errors"

	"github.com/dracory/entitystore"
)

// ConfigGet returns the raw value stored under a config key, or the
// empty string when the key does not exist.
func (st *Store) ConfigGet(ctx context.Context, key string) (string, error) {
	if key == "" {
		return "", errors.New("config get: key is required")
	}
	entity, err := st.entityStore.EntityFindByHandle(ctx, EntityTypeConfig, unifyName(key))
	if err != nil {
		return "", err
	}
	if entity == nil {
		return "", nil
	}
	attrs, err := st.entityAttributes(ctx, entity.GetID())
	if err != nil {
		return "", err
	}
	return attrs[AttributeConfigValue], nil
}

// ConfigSet upserts a raw value under a config key.
func (st *Store) ConfigSet(ctx context.Context, key string, value string) error {
	if key == "" {
		return errors.New("config set: key is required")
	}
	entity, err := st.entityStore.EntityFindByHandle(ctx, EntityTypeConfig, unifyName(key))
	if err != nil {
		return err
	}
	if entity == nil {
		entity = entitystore.NewEntity().SetType(EntityTypeConfig).SetHandle(unifyName(key))
		if err := st.entityStore.EntityCreate(ctx, entity); err != nil {
			return err
		}
	}
	return st.entityAttributesReplace(ctx, entity.GetID(), map[string]string{
		AttributeConfigValue: value,
	}, map[string]bool{AttributeConfigValue: true})
}
