package nodestore

import (
	"context"
	"encoding/json"

	"github.com/dracory/entitystore"
)

// MigrateWorkflowBlobAttributes is a one-shot migration for pre-change
// nodestore.db files: workflows that still carry the legacy
// ATTRIBUTE_NODES/ATTRIBUTE_EDGES JSON blobs get real node/edge child
// entities created (entity type node/edge, workflow_id link), and the
// blob attributes are removed afterwards.
//
// It is idempotent — workflows already migrated have no blob keys and
// are skipped (children loaded by loadWorkflowChildren take precedence
// anyway). Callers invoke it explicitly, e.g. once at startup:
//
//	migrated, err := store.MigrateWorkflowBlobAttributes(ctx)
//
// Returns the number of workflows migrated.
func (st *Store) MigrateWorkflowBlobAttributes(ctx context.Context) (int, error) {
	entities, err := st.entityStore.EntityList(ctx,
		entitystore.EntityQuery().WithEntityType(EntityTypeWorkflow).WithPrefetchAttributes())
	if err != nil {
		return 0, err
	}

	migrated := 0
	for _, entity := range entities {
		if err := ctx.Err(); err != nil {
			return migrated, err
		}
		attrs := entity.GetTempKeys()
		if len(attrs) == 0 {
			attrs, err = st.entityAttributes(ctx, entity.GetID())
			if err != nil {
				return migrated, err
			}
		}
		nodesRaw, hasNodes := attrs[ATTRIBUTE_NODES]
		edgesRaw, hasEdges := attrs[ATTRIBUTE_EDGES]
		if !hasNodes && !hasEdges {
			continue
		}

		workflowID := entity.GetID()

		// Node ids are referenced by edge source/target and by node
		// Data reference keys, so an over-length legacy node id needs
		// a deterministic short replacement plus a remap of every
		// reference. First pass: build the raw→short id map.
		nodeIDMap := map[string]string{}
		if hasNodes && nodesRaw != "" {
			var list []nodeJSON
			if err := json.Unmarshal([]byte(nodesRaw), &list); err != nil {
				return migrated, err
			}
			for _, j := range list {
				if len(j.ID) > maxEntityIDLength {
					nodeIDMap[j.ID] = shortEntityID("node", j.ID)
				}
			}
			for _, j := range list {
				newID := j.ID
				if mapped, ok := nodeIDMap[j.ID]; ok {
					newID = mapped
				}
				// Skip if a child entity with this ID already exists.
				existing, err := st.entityStore.EntityFindByID(ctx, newID)
				if err != nil {
					return migrated, err
				}
				if existing != nil {
					continue
				}
				node := nodeFromJSON(j).SetWorkflowID(workflowID).SetID(newID)
				if len(nodeIDMap) > 0 {
					data := node.GetData()
					for _, key := range nodeRefKeys {
						if rv, ok := data[key]; ok {
							data[key] = remapIDRefs(rv, nodeIDMap)
						}
					}
				}
				// Legacy blobs may carry plaintext credential values —
				// move them into credential entities (sentinel markers
				// stay in node data) before the guarded node create.
				tmp := NewWorkflow("migration").SetNodes([]NodeInterface{node})
				if creds := ExtractWorkflowCredentials(tmp, nil); len(creds) > 0 {
					for nodeID, values := range creds {
						if len(values) == 0 {
							continue
						}
						if err := st.CredentialCreate(ctx, NewCredential(workflowID, nodeID).SetValues(values)); err != nil {
							return migrated, err
						}
					}
				}
				if err := st.nodeCreate(ctx, node); err != nil {
					return migrated, err
				}
			}
		}
		if hasEdges && edgesRaw != "" {
			var list []edgeJSON
			if err := json.Unmarshal([]byte(edgesRaw), &list); err != nil {
				return migrated, err
			}
			for _, j := range list {
				// Repoint endpoints at shortened node ids (if any).
				if mapped, ok := nodeIDMap[j.Source]; ok {
					j.Source = mapped
				}
				if mapped, ok := nodeIDMap[j.Target]; ok {
					j.Target = mapped
				}
				edge := edgeFromJSON(j).SetWorkflowID(workflowID)
				// NewEdge already shortened over-length ids, so the
				// dedup check keys on the id that will be persisted.
				existing, err := st.entityStore.EntityFindByID(ctx, edge.GetID())
				if err != nil {
					return migrated, err
				}
				if existing != nil {
					continue
				}
				if err := st.EdgeCreate(ctx, edge); err != nil {
					return migrated, err
				}
			}
		}

		// Children created — strip the legacy blob keys. Managed keys
		// include them, so a replace with the workflow's current attrs
		// removes exactly those.
		current, err := st.entityAttributes(ctx, workflowID)
		if err != nil {
			return migrated, err
		}
		delete(current, ATTRIBUTE_NODES)
		delete(current, ATTRIBUTE_EDGES)
		if err := st.entityAttributesReplace(ctx, workflowID, current, workflowManagedAttributes); err != nil {
			return migrated, err
		}
		migrated++
	}
	return migrated, nil
}
