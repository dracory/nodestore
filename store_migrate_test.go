package nodestore

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/dracory/entitystore"
)

// TestMigrateWorkflowBlobAttributesLongIDs covers the legacy-blob
// failure seen on real data: pre-entity stores wrote synthetic
// composite edge ids (~100 chars) that exceed the 40-char entity ID
// column, so EntityCreate rejected them on every startup. The
// migration must derive a deterministic short id, keep edge endpoints
// resolving, and stay idempotent on re-run.
func TestMigrateWorkflowBlobAttributesLongIDs(t *testing.T) {
	store := initTestStore(t)
	ctx := context.Background()

	longEdgeID := "edge-Home-01a20b10-52c9-782b-bac7-5fe6cb9cca66-output_1-01a20b10-817b-74a4-9a92-8faa03a93b6e-input_1"
	if len(longEdgeID) <= maxEntityIDLength {
		t.Fatalf("fixture edge id must exceed %d chars, got %d", maxEntityIDLength, len(longEdgeID))
	}
	longNodeID := "node-01a20b10-52c9-782b-bac7-5fe6cb9cca66-extra-suffix"
	if len(longNodeID) <= maxEntityIDLength {
		t.Fatalf("fixture node id must exceed %d chars, got %d", maxEntityIDLength, len(longNodeID))
	}
	shortNodeID := shortEntityID("node", longNodeID)
	shortEdgeID := shortEntityID("edge", longEdgeID)
	if len(shortNodeID) > maxEntityIDLength || len(shortEdgeID) > maxEntityIDLength {
		t.Fatalf("short ids must fit the column: %q %q", shortNodeID, shortEdgeID)
	}

	// Seed a workflow record the way the pre-entity store wrote it:
	// one workflow entity, nodes/edges as JSON attribute blobs.
	entity := entitystore.NewEntity()
	entity.SetID("legacy-wf-long-ids")
	entity.SetType(EntityTypeWorkflow)
	entity.SetHandle("legacy-wf-long-ids")
	if err := store.entityStore.EntityCreate(ctx, entity); err != nil {
		t.Fatalf("seed entity: %v", err)
	}
	nodesBlob, _ := json.Marshal([]nodeJSON{
		{ID: longNodeID, Type: "inject", Position: positionJSON{X: 1, Y: 2},
			Data: map[string]any{"links": []any{longNodeID}}},
		{ID: "n2", Type: "debug", Position: positionJSON{X: 3, Y: 4}},
	})
	edgesBlob, _ := json.Marshal([]edgeJSON{
		{ID: longEdgeID, Source: longNodeID, SourceKey: "output_1", Target: "n2", TargetKey: "input_1"},
	})
	if err := store.entityStore.AttributesSet(ctx, "legacy-wf-long-ids", map[string]string{
		ATTRIBUTE_NAME:  "legacy-wf-long-ids",
		ATTRIBUTE_NODES: string(nodesBlob),
		ATTRIBUTE_EDGES: string(edgesBlob),
	}); err != nil {
		t.Fatalf("seed attrs: %v", err)
	}

	if n, err := store.MigrateWorkflowBlobAttributes(ctx); err != nil || n != 1 {
		t.Fatalf("blob migration: n=%d err=%v", n, err)
	}

	// Edge entity persisted under the deterministic short id.
	edge, err := store.EdgeFindByID(ctx, shortEdgeID)
	if err != nil || edge == nil {
		t.Fatalf("edge find by short id: %v nil=%v", err, edge == nil)
	}
	if edge.GetSource() != shortNodeID || edge.GetTarget() != "n2" {
		t.Fatalf("edge endpoints not remapped: %q -> %q", edge.GetSource(), edge.GetTarget())
	}
	if edge.GetSourceKey() != "output_1" || edge.GetTargetKey() != "input_1" {
		t.Fatalf("edge port keys lost: %q %q", edge.GetSourceKey(), edge.GetTargetKey())
	}
	if edge.GetWorkflowID() != "legacy-wf-long-ids" {
		t.Fatalf("edge workflow link: %q", edge.GetWorkflowID())
	}

	// Over-length node id persisted shortened, data refs remapped.
	node, err := store.NodeFindByID(ctx, shortNodeID)
	if err != nil || node == nil {
		t.Fatalf("node find by short id: %v nil=%v", err, node == nil)
	}
	links, _ := node.GetData()["links"].([]any)
	if len(links) != 1 || links[0] != shortNodeID {
		t.Fatalf("node data refs not remapped: %v", node.GetData()["links"])
	}

	// Re-run is a no-op: nothing migrated again, no duplicates.
	if n, err := store.MigrateWorkflowBlobAttributes(ctx); err != nil || n != 0 {
		t.Fatalf("rerun: n=%d err=%v", n, err)
	}
	edges, err := store.EdgeList(ctx, EdgeQuery().WithWorkflowID("legacy-wf-long-ids"))
	if err != nil || len(edges) != 1 {
		t.Fatalf("edge count after rerun: %d err=%v", len(edges), err)
	}
	nodes, err := store.NodeList(ctx, NodeQuery().WithWorkflowID("legacy-wf-long-ids"))
	if err != nil || len(nodes) != 2 {
		t.Fatalf("node count after rerun: %d err=%v", len(nodes), err)
	}

	// The loaded workflow graph resolves source/target as before.
	wf, err := store.WorkflowFindByID(ctx, "legacy-wf-long-ids")
	if err != nil || wf == nil {
		t.Fatalf("workflow reload: %v", err)
	}
	if len(wf.GetEdges()) != 1 || wf.GetEdges()[0].GetTarget() != "n2" {
		t.Fatalf("reloaded edges: %+v", wf.GetEdges())
	}
}
