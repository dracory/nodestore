package nodestore

import (
	"context"
	"encoding/json"
	"testing"
)

// TestMigrateWorkflowGroupsConfig covers the global-config →
// workflow_group migration: entities are created from the blob, the
// section is removed, and a re-run is a no-op.
func TestMigrateWorkflowGroupsConfig(t *testing.T) {
	store := initTestStore(t)
	ctx := context.Background()

	cfg := map[string]any{
		"env": []any{map[string]any{"name": "A", "type": "str", "value": "1"}},
		"workflowGroups": []any{
			map[string]any{"id": "wg1", "name": "Prod", "parentId": "", "workflows": []any{"wf1"}},
			map[string]any{"id": "wg2", "name": "EU", "parentId": "wg1", "workflows": []any{}},
		},
	}
	raw, _ := json.Marshal(cfg)
	if err := store.ConfigSet(ctx, "global_config", string(raw)); err != nil {
		t.Fatalf("seed config: %v", err)
	}

	n, err := store.MigrateWorkflowGroupsConfig(ctx, "global_config")
	if err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if n != 2 {
		t.Fatalf("expected 2 migrated, got %d", n)
	}
	g, err := store.WorkflowGroupFindByID(ctx, "wg2")
	if err != nil || g == nil {
		t.Fatalf("find wg2: %v nil=%v", err, g == nil)
	}
	if g.GetParentID() != "wg1" || g.GetName() != "EU" {
		t.Fatalf("wg2 fields: %v %v", g.GetParentID(), g.GetName())
	}
	// Section removed, siblings preserved.
	out, _ := store.ConfigGet(ctx, "global_config")
	var after map[string]any
	_ = json.Unmarshal([]byte(out), &after)
	if _, ok := after["workflowGroups"]; ok {
		t.Fatal("workflowGroups section must be removed after migration")
	}
	if _, ok := after["env"]; !ok {
		t.Fatal("sibling env section must survive")
	}
	// Idempotent: second run reports nothing new and keeps entities.
	if n, err := store.MigrateWorkflowGroupsConfig(ctx, "global_config"); err != nil || n != 0 {
		t.Fatalf("rerun: n=%d err=%v", n, err)
	}
	if g2, _ := store.WorkflowGroupFindByID(ctx, "wg1"); g2 == nil {
		t.Fatal("entity lost on rerun")
	}
}

// TestMigrateViewGroups covers the view.groups → node_group migration:
// entities keep the entry ID, members get group_id on their node
// entities, and a re-run is convergent.
func TestMigrateViewGroups(t *testing.T) {
	store := initTestStore(t)
	ctx := context.Background()

	wf := NewWorkflow("w").SetID("wf1")
	wf.SetNodes([]NodeInterface{
		NewNode("n1", "inject"),
		NewNode("n2", "debug"),
	})
	wf.SetView(map[string]any{
		"groups": []any{
			map[string]any{
				"id": "g1", "name": "Inputs",
				"x": 1.0, "y": 2.0, "w": 3.0, "h": 4.0,
				"nodeIds": []any{"n1", "n2"},
			},
		},
	})
	if err := store.WorkflowCreate(ctx, wf); err != nil {
		t.Fatalf("workflow create: %v", err)
	}

	n, err := store.MigrateViewGroups(ctx)
	if err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if n != 1 {
		t.Fatalf("expected 1 migrated, got %d", n)
	}
	g, err := store.NodeGroupFindByID(ctx, "g1")
	if err != nil || g == nil {
		t.Fatalf("find g1: %v nil=%v", err, g == nil)
	}
	if g.GetWorkflowID() != "wf1" || g.GetName() != "Inputs" || g.GetData()["w"] != 3.0 {
		t.Fatalf("g1 fields: %v %v %v", g.GetWorkflowID(), g.GetName(), g.GetData())
	}
	for _, id := range []string{"n1", "n2"} {
		node, _ := store.NodeFindByID(ctx, id)
		if node == nil || node.GetGroupID() != "g1" {
			t.Fatalf("node %s group_id: %v", id, node)
		}
	}
	// Idempotent.
	if n, err := store.MigrateViewGroups(ctx); err != nil || n != 1 {
		t.Fatalf("rerun: n=%d err=%v", n, err)
	}
}
