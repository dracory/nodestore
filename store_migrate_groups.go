package nodestore

import (
	"context"
	"encoding/json"
)

// store_migrate_groups.go holds the group-persistence migrations for
// the P4 cutover (task cutover_p4_entities):
//
//   - MigrateWorkflowGroupsConfig moves sidebar workflow groupings out
//     of the global-config JSON blob ("workflowGroups" section) into
//     workflow_group entities.
//   - MigrateViewGroups moves editor view.groups entries into
//     node_group entities and stamps group_id on the member node
//     entities (membership lives on the node — GOD ruling 2026-11-24).
//
// Both are idempotent (entities are upserted by their source ID) and
// safe to run at every startup — the P5 wiring task owns where they
// are invoked; nodeadmin calls them on mount.

// workflowGroupsJSON is the persisted shape of one sidebar workflow
// group: {id, name, parentId, workflows:[id]}.
type workflowGroupsJSON struct {
	ID        string   `json:"id"`
	Name      string   `json:"name"`
	ParentID  string   `json:"parentId"`
	Workflows []string `json:"workflows"`
}

// MigrateWorkflowGroupsConfig converts the "workflowGroups" section of
// the named config blob into workflow_group entities, then removes the
// section so the config store no longer carries structured domain
// data. Returns the number of groups upserted. Idempotent: groups are
// keyed by their original ID and updated in place on re-run.
func (st *Store) MigrateWorkflowGroupsConfig(ctx context.Context, configKey string) (int, error) {
	raw, err := st.ConfigGet(ctx, configKey)
	if err != nil || raw == "" {
		return 0, err
	}
	cfg := map[string]any{}
	if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
		return 0, err
	}
	section, ok := cfg["workflowGroups"]
	if !ok {
		return 0, nil // already migrated (or never existed)
	}
	sectionRaw, err := json.Marshal(section)
	if err != nil {
		return 0, err
	}
	var groups []workflowGroupsJSON
	if err := json.Unmarshal(sectionRaw, &groups); err != nil {
		return 0, err
	}

	migrated := 0
	for _, g := range groups {
		if g.ID == "" {
			continue
		}
		existing, err := st.WorkflowGroupFindByID(ctx, g.ID)
		if err != nil {
			return migrated, err
		}
		if existing == nil {
			existing = NewWorkflowGroup(g.Name).SetID(g.ID)
		}
		existing.SetName(g.Name).SetParentID(g.ParentID).SetWorkflowIDs(g.Workflows)
		if err := st.upsertWorkflowGroup(ctx, existing); err != nil {
			return migrated, err
		}
		migrated++
	}

	delete(cfg, "workflowGroups")
	out, err := json.Marshal(cfg)
	if err != nil {
		return migrated, err
	}
	if err := st.ConfigSet(ctx, configKey, string(out)); err != nil {
		return migrated, err
	}
	return migrated, nil
}

// upsertWorkflowGroup creates or updates a workflow_group entity —
// entitystore.EntityFindByID is the existence check because FindByID on
// the store returns the domain object either way.
func (st *Store) upsertWorkflowGroup(ctx context.Context, group WorkflowGroupInterface) error {
	entity, err := st.entityStore.EntityFindByID(ctx, group.GetID())
	if err != nil {
		return err
	}
	if entity == nil {
		return st.WorkflowGroupCreate(ctx, group)
	}
	return st.WorkflowGroupUpdate(ctx, group)
}

// MigrateViewGroups converts each workflow's view.groups array into
// node_group entities and stamps group_id on the member node entities.
// Once converted, the "groups" key is stripped from the stored view
// attribute — since P5 the node_group entities are the authoritative
// read path (loadWorkflowGroups re-synthesizes the wire shape), so a
// lingering blob could only go stale. Returns the number of node groups
// upserted. Idempotent: group entities keep the entry ID, memberships
// are rewritten, and re-runs see a view attribute with no "groups" key.
func (st *Store) MigrateViewGroups(ctx context.Context) (int, error) {
	workflows, err := st.WorkflowList(ctx, WorkflowQuery())
	if err != nil {
		return 0, err
	}
	migrated := 0
	for _, wf := range workflows {
		n, err := st.migrateWorkflowViewGroups(ctx, wf)
		if err != nil {
			return migrated, err
		}
		migrated += n
	}
	return migrated, nil
}

// migrateWorkflowViewGroups migrates one workflow's view.groups.
func (st *Store) migrateWorkflowViewGroups(ctx context.Context, wf WorkflowInterface) (int, error) {
	view := wf.GetView()
	rawGroups, ok := view["groups"].([]any)
	if !ok || len(rawGroups) == 0 {
		return 0, nil
	}

	migrated := 0
	for _, raw := range rawGroups {
		entry, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		id, _ := entry["id"].(string)
		if id == "" {
			continue
		}
		name, _ := entry["name"].(string)

		// The data attribute keeps every entry key except id/name/
		// nodeIds verbatim (x, y, w, h, disabled, childFlow, env) so the
		// wire shape round-trips.
		data := map[string]any{}
		for k, v := range entry {
			if k == "id" || k == "name" || k == "nodeIds" {
				continue
			}
			data[k] = v
		}

		entity, err := st.entityStore.EntityFindByID(ctx, id)
		if err != nil {
			return migrated, err
		}
		if entity == nil {
			if err := st.NodeGroupCreate(ctx,
				NewNodeGroup(wf.GetID(), name).SetID(id).SetData(data)); err != nil {
				return migrated, err
			}
		} else {
			group, err := st.NodeGroupFindByID(ctx, id)
			if err != nil {
				return migrated, err
			}
			if group != nil {
				group.SetName(name).SetData(data).SetWorkflowID(wf.GetID())
				if err := st.NodeGroupUpdate(ctx, group); err != nil {
					return migrated, err
				}
			}
		}

		// Stamp group_id on member node entities (membership lives on
		// the node entity, not on the group).
		for _, memberRaw := range arrayOf(entry["nodeIds"]) {
			memberID, _ := memberRaw.(string)
			if memberID == "" {
				continue
			}
			node, err := st.NodeFindByID(ctx, memberID)
			if err != nil {
				return migrated, err
			}
			if node == nil || node.GetGroupID() == id {
				continue
			}
			node.SetGroupID(id)
			if err := st.NodeUpdate(ctx, node); err != nil {
				return migrated, err
			}
		}
		migrated++
	}

	// P5 strip: the stored view must not keep a "groups" key once the
	// entities exist — reads are synthesized from node_group entities,
	// so the blob is dead weight that can only diverge.
	if err := st.stripViewGroupsAttribute(ctx, wf.GetID()); err != nil {
		return migrated, err
	}
	return migrated, nil
}

// stripViewGroupsAttribute removes the "groups" key from a workflow's
// stored view attribute when present. A no-op for already-migrated
// records — the key is absent, nothing is written.
func (st *Store) stripViewGroupsAttribute(ctx context.Context, workflowID string) error {
	attrs, err := st.entityAttributes(ctx, workflowID)
	if err != nil {
		return err
	}
	raw := attrs[ATTRIBUTE_VIEW]
	if raw == "" {
		return nil
	}
	storedView := map[string]any{}
	if err := json.Unmarshal([]byte(raw), &storedView); err != nil {
		return err
	}
	if _, has := storedView["groups"]; !has {
		return nil
	}
	delete(storedView, "groups")
	out, err := json.Marshal(storedView)
	if err != nil {
		return err
	}
	return st.entityStore.AttributesSet(ctx, workflowID,
		map[string]string{ATTRIBUTE_VIEW: string(out)})
}
