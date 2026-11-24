package nodestore

import (
	"context"
	"errors"
	"sort"

	"github.com/dracory/entitystore"
)

func (st *Store) DeployRecordCount(ctx context.Context, query DeployRecordQueryInterface) (int64, error) {
	if query == nil {
		return 0, errors.New("deploy record count: query is nil")
	}
	if query.HasWorkflowID() {
		list, err := st.DeployRecordList(ctx, query.WithCountOnly(true))
		if err != nil {
			return 0, err
		}
		return int64(len(list)), nil
	}
	entityQuery := entitystore.EntityQuery().WithEntityType(EntityTypeDeployRecord)
	if query.HasID() {
		entityQuery.WithID(query.GetID())
	}
	return st.entityStore.EntityCount(ctx, entityQuery)
}

// DeployRecordCreate appends an immutable deploy record and prunes the
// workflow's history beyond DeployRecordRetention (oldest first —
// deploy records are append-only, never updated, so there is no
// DeployRecordUpdate on the contract).
func (st *Store) DeployRecordCreate(ctx context.Context, record DeployRecordInterface) error {
	if record == nil {
		return errors.New("deploy record create: record is nil")
	}
	impl, ok := record.(*deployRecordImplementation)
	if !ok {
		return errors.New("deploy record create: unsupported implementation")
	}
	// Deploy records must only ever carry sentinel markers — a
	// snapshot taken after credential injection would otherwise
	// persist every secret in plaintext.
	if err := validateFlowJSONCredentials(record.GetFlow()); err != nil {
		return err
	}
	if err := st.entityStore.EntityCreate(ctx, impl.entity); err != nil {
		return err
	}
	if err := st.entityStore.AttributesSet(ctx, impl.entity.GetID(), impl.attributes()); err != nil {
		return err
	}
	return st.pruneDeployRecords(ctx, record.GetWorkflowID())
}

// pruneDeployRecords deletes the workflow's oldest records so at most
// DeployRecordRetention survive.
func (st *Store) pruneDeployRecords(ctx context.Context, workflowID string) error {
	if workflowID == "" {
		return nil
	}
	list, err := st.DeployRecordList(ctx, DeployRecordQuery().WithWorkflowID(workflowID).WithSortOrder(ASC))
	if err != nil {
		return err
	}
	for len(list) > DeployRecordRetention {
		if err := st.DeployRecordDeleteByID(ctx, list[0].GetID()); err != nil {
			return err
		}
		list = list[1:]
	}
	return nil
}

func (st *Store) DeployRecordDeleteByID(ctx context.Context, id string) error {
	_, err := st.entityStore.EntityDelete(ctx, id)
	return err
}

// DeployRecordDeleteByWorkflowID drops the workflow's whole deploy
// history (called when the workflow is deleted).
func (st *Store) DeployRecordDeleteByWorkflowID(ctx context.Context, workflowID string) error {
	list, err := st.DeployRecordList(ctx, DeployRecordQuery().WithWorkflowID(workflowID))
	if err != nil {
		return err
	}
	for _, r := range list {
		if err := st.DeployRecordDeleteByID(ctx, r.GetID()); err != nil {
			return err
		}
	}
	return nil
}

func (st *Store) DeployRecordFindByID(ctx context.Context, id string) (DeployRecordInterface, error) {
	entity, err := st.entityStore.EntityFindByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if entity == nil || entity.GetType() != EntityTypeDeployRecord {
		return nil, nil
	}
	attrs, err := st.entityAttributes(ctx, entity.GetID())
	if err != nil {
		return nil, err
	}
	return deployRecordFromEntity(entity, attrs), nil
}

// DeployRecordList lists deploy record entities; WithWorkflowID uses
// EntityListByAttribute like the other per-workflow listings. Sort
// order applies to the entity created_at (the deploy timestamp).
func (st *Store) DeployRecordList(ctx context.Context, query DeployRecordQueryInterface) ([]DeployRecordInterface, error) {
	if query == nil {
		query = DeployRecordQuery()
	}
	if err := query.Validate(); err != nil {
		return nil, err
	}

	var entities []entitystore.EntityInterface
	var err error
	if query.HasWorkflowID() {
		entities, err = st.entityStore.EntityListByAttribute(ctx,
			EntityTypeDeployRecord, ATTRIBUTE_WORKFLOW_ID, query.GetWorkflowID())
	} else {
		entityQuery := entitystore.EntityQuery().WithEntityType(EntityTypeDeployRecord)
		if query.HasID() {
			entityQuery.WithID(query.GetID())
		}
		entityQuery.WithPrefetchAttributes()
		entities, err = st.entityStore.EntityList(ctx, entityQuery)
	}
	if err != nil {
		return nil, err
	}

	matched := make([]DeployRecordInterface, 0, len(entities))
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
		matched = append(matched, deployRecordFromEntity(entity, attrs))
	}

	sort.SliceStable(matched, func(i, j int) bool {
		if query.GetSortOrder() == ASC {
			return matched[i].GetCreatedAt().Before(matched[j].GetCreatedAt())
		}
		return matched[i].GetCreatedAt().After(matched[j].GetCreatedAt())
	})

	if query.HasOffset() && query.GetOffset() < uint64(len(matched)) {
		matched = matched[query.GetOffset():]
	}
	if query.HasLimit() && query.GetLimit() < uint64(len(matched)) {
		matched = matched[:query.GetLimit()]
	}
	return matched, nil
}
