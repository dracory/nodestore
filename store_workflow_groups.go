package nodestore

import (
	"context"
)

// store_workflow_groups.go is the P5 cutover for editor view groups:
// node_group entities (plus group_id on the member node entities) are
// now the authoritative read AND write path — the view.groups blob is
// no longer stored (workflow.attributes strips it) and is only a
// pre-migration read fallback.
//
// Read path: loadWorkflowGroups synthesizes view["groups"] from the
// workflow's node_group entities whenever they exist, so every consumer
// (editor, Node-RED export, module exchange) keeps the same wire shape
// while the data comes from entities.
//
// Write path: syncWorkflowGroups runs inside WorkflowCreate/Update —
// whenever the incoming view carries a "groups" key the entities are
// converged to match it exactly (upsert kept, stale deleted, member
// nodes re-stamped). Writers that omit "groups" leave group state
// untouched.
//
// Result: writes converge AND the blob is fully migrated — entity reads
// win wherever both exist.

// viewGroupEntry is one parsed view.groups item.
type viewGroupEntry struct {
	id      string
	name    string
	data    map[string]any // every entry key except id/name/nodeIds
	nodeIDs []string
}

// parseViewGroups decodes a raw view["groups"] value into entries.
// Entries without an id are skipped (they can never map to an entity).
func parseViewGroups(raw any) []viewGroupEntry {
	entries := []viewGroupEntry{}
	for _, item := range arrayOf(raw) {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		id, _ := m["id"].(string)
		if id == "" {
			continue
		}
		name, _ := m["name"].(string)
		data := map[string]any{}
		for k, v := range m {
			if k == "id" || k == "name" || k == "nodeIds" {
				continue
			}
			data[k] = v
		}
		nodeIDs := []string{}
		for _, memberRaw := range arrayOf(m["nodeIds"]) {
			if memberID, _ := memberRaw.(string); memberID != "" {
				nodeIDs = append(nodeIDs, memberID)
			}
		}
		entries = append(entries, viewGroupEntry{id: id, name: name, data: data, nodeIDs: nodeIDs})
	}
	return entries
}

// loadWorkflowGroups overlays view["groups"] with the workflow's
// node_group entities — the P5 read path. When the workflow has no
// group entities the view is left untouched (legacy blob fallback for
// unmigrated records). Membership is rebuilt from group_id on the
// already-loaded child nodes.
func (st *Store) loadWorkflowGroups(ctx context.Context, w *workflowImplementation) error {
	groups, err := st.NodeGroupList(ctx, NodeGroupQuery().WithWorkflowID(w.GetID()))
	if err != nil {
		return err
	}
	if len(groups) == 0 {
		return nil
	}

	members := map[string][]any{}
	for _, n := range w.nodes {
		if gid := n.GetGroupID(); gid != "" {
			members[gid] = append(members[gid], n.GetID())
		}
	}

	entries := make([]any, 0, len(groups))
	for _, g := range groups {
		entry := map[string]any{}
		for k, v := range g.GetData() {
			entry[k] = v
		}
		entry["id"] = g.GetID()
		entry["name"] = g.GetName()
		nodeIDs := members[g.GetID()]
		if nodeIDs == nil {
			nodeIDs = []any{}
		}
		entry["nodeIds"] = nodeIDs
		entries = append(entries, entry)
	}
	w.view["groups"] = entries
	return nil
}

// syncWorkflowGroups converges the workflow's node_group entities to
// the view["groups"] the caller supplied — the P5 write path. A view
// without a "groups" key is a no-op: non-editor writers (deploy flag
// flips, module metadata edits) never clobber group state.
//
// Must run AFTER the workflow's child nodes are persisted — membership
// is stamped as group_id on the node entities.
func (st *Store) syncWorkflowGroups(ctx context.Context, w *workflowImplementation) error {
	raw, present := w.view["groups"]
	if !present {
		return nil
	}
	entries := parseViewGroups(raw)

	keep := map[string]bool{}
	// member group assignment — last claim wins, matching the editor's
	// one-group-per-node model.
	memberOf := map[string]string{}
	for _, e := range entries {
		keep[e.id] = true
		entity, err := st.entityStore.EntityFindByID(ctx, e.id)
		if err != nil {
			return err
		}
		if entity == nil {
			if err := st.NodeGroupCreate(ctx,
				NewNodeGroup(w.GetID(), e.name).SetID(e.id).SetData(e.data)); err != nil {
				return err
			}
		} else {
			group, err := st.NodeGroupFindByID(ctx, e.id)
			if err != nil {
				return err
			}
			if group != nil {
				group.SetName(e.name).SetData(e.data).SetWorkflowID(w.GetID())
				if err := st.NodeGroupUpdate(ctx, group); err != nil {
					return err
				}
			}
		}
		for _, nodeID := range e.nodeIDs {
			memberOf[nodeID] = e.id
		}
	}

	// Groups the caller no longer sends are deleted — the entity set is
	// authoritative, not additive.
	existing, err := st.NodeGroupList(ctx, NodeGroupQuery().WithWorkflowID(w.GetID()))
	if err != nil {
		return err
	}
	for _, g := range existing {
		if keep[g.GetID()] {
			continue
		}
		if err := st.NodeGroupDeleteByID(ctx, g.GetID()); err != nil {
			return err
		}
	}

	// Re-stamp membership: every node gets exactly the group the view
	// claims (or none), so moves and removals both converge.
	nodes, err := st.NodeList(ctx, NodeQuery().WithWorkflowID(w.GetID()))
	if err != nil {
		return err
	}
	for _, n := range nodes {
		desired := memberOf[n.GetID()]
		if n.GetGroupID() == desired {
			continue
		}
		n.SetGroupID(desired)
		if err := st.NodeUpdate(ctx, n); err != nil {
			return err
		}
	}
	return nil
}
