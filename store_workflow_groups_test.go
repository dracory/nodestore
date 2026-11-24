package nodestore

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/dracory/entitystore"
)

// TestWorkflowGroupsEntityCutover covers the P5 group cutover end to
// end at the store level: a workflow saved with view.groups gets
// node_group entities + stamped node group_id, the stored view blob
// drops the "groups" key, and reads synthesize the same wire shape
// back. An update that drops one group and renames the other converges
// the entity set — entities are the authoritative read AND write path.
func TestWorkflowGroupsEntityCutover(t *testing.T) {
	store := initTestStore(t)
	ctx := context.Background()

	wf := NewWorkflow("w").SetID("wf1")
	wf.SetNodes([]NodeInterface{
		NewNode("n1", "inject"),
		NewNode("n2", "debug"),
		NewNode("n3", "debug"),
	})
	wf.SetEdges([]EdgeInterface{NewEdge("e1", "n1", "n2")})
	wf.SetView(map[string]any{
		"theme": "dark",
		"groups": []any{
			map[string]any{
				"id": "g1", "name": "Inputs",
				"x": 1.0, "y": 2.0, "w": 3.0, "h": 4.0,
				"nodeIds": []any{"n1", "n2"},
			},
			map[string]any{
				"id": "g2", "name": "Outputs", "module": "Home",
				"nodeIds": []any{"n3"},
			},
		},
	})
	if err := store.WorkflowCreate(ctx, wf); err != nil {
		t.Fatalf("workflow create: %v", err)
	}

	// Write path: entities exist, membership stamped, blob stripped.
	groups, err := store.NodeGroupList(ctx, NodeGroupQuery().WithWorkflowID("wf1"))
	if err != nil || len(groups) != 2 {
		t.Fatalf("expected 2 node_group entities, got %d err=%v", len(groups), err)
	}
	for _, tc := range []struct{ nodeID, groupID string }{
		{"n1", "g1"}, {"n2", "g1"}, {"n3", "g2"},
	} {
		n, _ := store.NodeFindByID(ctx, tc.nodeID)
		if n == nil || n.GetGroupID() != tc.groupID {
			t.Fatalf("node %s group_id=%q want %q", tc.nodeID, n.GetGroupID(), tc.groupID)
		}
	}
	attrs, err := store.entityAttributes(ctx, "wf1")
	if err != nil {
		t.Fatalf("attrs: %v", err)
	}
	var storedView map[string]any
	if err := json.Unmarshal([]byte(attrs[ATTRIBUTE_VIEW]), &storedView); err != nil {
		t.Fatalf("stored view: %v", err)
	}
	if _, has := storedView["groups"]; has {
		t.Fatal("stored view must not keep a groups key after P5")
	}
	if storedView["theme"] != "dark" {
		t.Fatalf("sibling view keys must survive, got %v", storedView)
	}

	// Read path: view.groups is re-synthesized from the entities.
	loaded, err := store.WorkflowFindByID(ctx, "wf1")
	if err != nil || loaded == nil {
		t.Fatalf("reload: %v", err)
	}
	synth := parseViewGroups(loaded.GetView()["groups"])
	if len(synth) != 2 {
		t.Fatalf("expected 2 synthesized groups, got %d", len(synth))
	}
	if synth[0].id != "g1" || synth[0].name != "Inputs" || synth[0].data["w"] != 3.0 {
		t.Fatalf("g1 round-trip: %+v", synth[0])
	}
	if len(synth[0].nodeIDs) != 2 || synth[0].nodeIDs[0] != "n1" {
		t.Fatalf("g1 members: %v", synth[0].nodeIDs)
	}
	if synth[1].data["module"] != "Home" {
		t.Fatalf("g2 data round-trip: %v", synth[1].data)
	}

	// Update: drop g2, rename g1, move n3 into g1 — all through the
	// same view.groups write the editor sends.
	loaded.SetView(map[string]any{
		"theme": "dark",
		"groups": []any{
			map[string]any{
				"id": "g1", "name": "Renamed",
				"nodeIds": []any{"n1", "n3"},
			},
		},
	})
	if err := store.WorkflowUpdate(ctx, loaded); err != nil {
		t.Fatalf("workflow update: %v", err)
	}
	groups, _ = store.NodeGroupList(ctx, NodeGroupQuery().WithWorkflowID("wf1"))
	if len(groups) != 1 || groups[0].GetName() != "Renamed" {
		t.Fatalf("converged groups: %d %v", len(groups), groups)
	}
	n2, _ := store.NodeFindByID(ctx, "n2")
	if n2.GetGroupID() != "" {
		t.Fatalf("removed member must be cleared, got %q", n2.GetGroupID())
	}
	n3, _ := store.NodeFindByID(ctx, "n3")
	if n3.GetGroupID() != "g1" {
		t.Fatalf("moved member must be re-stamped, got %q", n3.GetGroupID())
	}

	// A view without a "groups" key leaves group state alone
	// (deploy-flag flips etc. must not clobber entities).
	loaded2, _ := store.WorkflowFindByID(ctx, "wf1")
	loaded2.SetView(map[string]any{"theme": "light"})
	if err := store.WorkflowUpdate(ctx, loaded2); err != nil {
		t.Fatalf("update without groups: %v", err)
	}
	groups, _ = store.NodeGroupList(ctx, NodeGroupQuery().WithWorkflowID("wf1"))
	if len(groups) != 1 {
		t.Fatalf("groups must survive a groups-less update, got %d", len(groups))
	}
}

// TestMigrationPipelineFromLegacyBlobs is the P5 acceptance fixture:
// a legacy workflow record — ATTRIBUTE_NODES/ATTRIBUTE_EDGES blobs and
// a view.groups array inside the view blob, exactly as a flowadmin-era
// DB carries them — is migrated by the same startup sequence
// nodeadmin.New runs. Afterwards the graph loads entirely as entities
// (nodes, edges, node_group, stamped membership) and a re-run is a
// no-op.
func TestMigrationPipelineFromLegacyBlobs(t *testing.T) {
	store := initTestStore(t)
	ctx := context.Background()

	// Seed the record the way the pre-entity store wrote it: one
	// workflow entity, nodes/edges/groups as JSON attribute blobs.
	entity := entitystore.NewEntity()
	entity.SetID("legacy-wf")
	entity.SetType(EntityTypeWorkflow)
	entity.SetHandle("legacy-wf")
	if err := store.entityStore.EntityCreate(ctx, entity); err != nil {
		t.Fatalf("seed entity: %v", err)
	}
	nodesBlob, _ := json.Marshal([]nodeJSON{
		{ID: "n1", Type: "inject", Position: positionJSON{X: 10, Y: 20}},
		{ID: "n2", Type: "debug", Position: positionJSON{X: 30, Y: 40}},
	})
	edgesBlob, _ := json.Marshal([]edgeJSON{
		{ID: "e1", Source: "n1", Target: "n2"},
	})
	viewBlob, _ := json.Marshal(map[string]any{
		"groups": []any{
			map[string]any{"id": "g1", "name": "Main", "nodeIds": []any{"n1"}},
		},
	})
	if err := store.entityStore.AttributesSet(ctx, "legacy-wf", map[string]string{
		ATTRIBUTE_NAME:     "legacy-wf",
		ATTRIBUTE_NODES:    string(nodesBlob),
		ATTRIBUTE_EDGES:    string(edgesBlob),
		ATTRIBUTE_VIEW:     string(viewBlob),
		ATTRIBUTE_DEPLOYED: "false",
	}); err != nil {
		t.Fatalf("seed attrs: %v", err)
	}

	// Same order nodeadmin.New runs them.
	if n, err := store.MigrateWorkflowBlobAttributes(ctx); err != nil || n != 1 {
		t.Fatalf("blob migration: n=%d err=%v", n, err)
	}
	if n, err := store.MigrateViewGroups(ctx); err != nil || n != 1 {
		t.Fatalf("groups migration: n=%d err=%v", n, err)
	}

	// Graph loads as entities — children, edge, group, membership.
	wf, err := store.WorkflowFindByID(ctx, "legacy-wf")
	if err != nil || wf == nil {
		t.Fatalf("reload: %v", err)
	}
	if len(wf.GetNodes()) != 2 || len(wf.GetEdges()) != 1 {
		t.Fatalf("children: %d nodes %d edges", len(wf.GetNodes()), len(wf.GetEdges()))
	}
	if wf.GetEdges()[0].GetSource() != "n1" || wf.GetEdges()[0].GetTarget() != "n2" {
		t.Fatalf("edge integrity: %+v", wf.GetEdges()[0])
	}
	groups, _ := store.NodeGroupList(ctx, NodeGroupQuery().WithWorkflowID("legacy-wf"))
	if len(groups) != 1 || groups[0].GetID() != "g1" {
		t.Fatalf("node_group entity: %v", groups)
	}
	n1, _ := store.NodeFindByID(ctx, "n1")
	if n1.GetGroupID() != "g1" {
		t.Fatalf("membership: %q", n1.GetGroupID())
	}
	synth := parseViewGroups(wf.GetView()["groups"])
	if len(synth) != 1 || len(synth[0].nodeIDs) != 1 || synth[0].nodeIDs[0] != "n1" {
		t.Fatalf("synthesized groups: %+v", synth)
	}
	attrs, _ := store.entityAttributes(ctx, "legacy-wf")
	if _, has := attrs[ATTRIBUTE_NODES]; has {
		t.Fatal("blob nodes attribute must be stripped")
	}
	var storedView map[string]any
	_ = json.Unmarshal([]byte(attrs[ATTRIBUTE_VIEW]), &storedView)
	if _, has := storedView["groups"]; has {
		t.Fatal("blob view.groups must be stripped")
	}

	// Re-run is a no-op: nothing new is created and nothing breaks.
	if n, err := store.MigrateWorkflowBlobAttributes(ctx); err != nil || n != 0 {
		t.Fatalf("blob rerun: n=%d err=%v", n, err)
	}
	if _, err := store.MigrateViewGroups(ctx); err != nil {
		t.Fatalf("groups rerun: %v", err)
	}
	if c, _ := store.NodeGroupCount(ctx, NodeGroupQuery()); c != 1 {
		t.Fatalf("rerun duplicated groups: %d", c)
	}
	wf2, _ := store.WorkflowFindByID(ctx, "legacy-wf")
	if len(wf2.GetNodes()) != 2 || len(wf2.GetEdges()) != 1 {
		t.Fatal("rerun corrupted the graph")
	}
}
