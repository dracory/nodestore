package nodestore

import (
	"context"
	"testing"

	"github.com/dracory/entitystore"
)

// TestWorkflowChildrenEntities verifies a workflow with 3 nodes + 2
// edges persists as 1 workflow entity + 3 node entities + 2 edge
// entities linked via workflow_id.
func TestWorkflowChildrenEntities(t *testing.T) {
	store := initTestStore(t)
	ctx := context.Background()

	workflow := NewWorkflow("graph")
	workflow.AddNode(NewNode("n1", "inject").SetPosition(0, 0))
	workflow.AddNode(NewNode("n2", "function").SetPosition(10, 0))
	workflow.AddNode(NewNode("n3", "debug").SetPosition(20, 0))
	workflow.AddEdge(NewEdge("e1", "n1", "n2"))
	workflow.AddEdge(NewEdge("e2", "n2", "n3"))

	if err := store.WorkflowCreate(ctx, workflow); err != nil {
		t.Fatal("create:", err)
	}

	countEntities := func(entityType string) int64 {
		count, err := store.GetEntityStore().EntityCount(ctx,
			entitystore.EntityQuery().WithEntityType(entityType))
		if err != nil {
			t.Fatal("entity count:", err)
		}
		return count
	}
	if got := countEntities(EntityTypeWorkflow); got != 1 {
		t.Fatal("expected 1 workflow entity, got", got)
	}
	if got := countEntities(EntityTypeNode); got != 3 {
		t.Fatal("expected 3 node entities, got", got)
	}
	if got := countEntities(EntityTypeEdge); got != 2 {
		t.Fatal("expected 2 edge entities, got", got)
	}

	// Children are linked to the parent workflow.
	nodes, err := store.NodeList(ctx, NodeQuery().WithWorkflowID(workflow.GetID()))
	if err != nil || len(nodes) != 3 {
		t.Fatal("NodeList(WithWorkflowID):", err, len(nodes))
	}
	for _, n := range nodes {
		if n.GetWorkflowID() != workflow.GetID() {
			t.Fatal("node workflow_id mismatch")
		}
	}
	edges, err := store.EdgeList(ctx, EdgeQuery().WithWorkflowID(workflow.GetID()))
	if err != nil || len(edges) != 2 {
		t.Fatal("EdgeList(WithWorkflowID):", err, len(edges))
	}

	// Reloaded workflow has children eagerly populated.
	found, err := store.WorkflowFindByID(ctx, workflow.GetID())
	if err != nil || found == nil {
		t.Fatal("find:", err)
	}
	if len(found.GetNodes()) != 3 || len(found.GetEdges()) != 2 {
		t.Fatal("reloaded children count mismatch")
	}
}

// TestNodeEdgeCRUD covers standalone node/edge entity operations.
func TestNodeEdgeCRUD(t *testing.T) {
	store := initTestStore(t)
	ctx := context.Background()

	node := NewNode("n1", "inject").SetWorkflowID("wf1").SetPosition(5, 7)
	node.SetData(map[string]any{"k": "v"})
	if err := store.NodeCreate(ctx, node); err != nil {
		t.Fatal("node create:", err)
	}
	found, err := store.NodeFindByID(ctx, "n1")
	if err != nil || found == nil {
		t.Fatal("node find:", err)
	}
	if found.GetType() != "inject" || found.GetPositionX() != 5 || found.GetPositionY() != 7 {
		t.Fatal("node round-trip mismatch")
	}
	if found.GetData()["k"] != "v" || found.GetWorkflowID() != "wf1" {
		t.Fatal("node data/workflow mismatch")
	}
	if found.GetCreatedAt().IsZero() {
		t.Fatal("node created_at should come from the entity")
	}

	found.SetPosition(50, 70).SetType("debug")
	if err := store.NodeUpdate(ctx, found); err != nil {
		t.Fatal("node update:", err)
	}
	reloaded, _ := store.NodeFindByID(ctx, "n1")
	if reloaded.GetPositionX() != 50 || reloaded.GetType() != "debug" {
		t.Fatal("node update round-trip mismatch")
	}

	edge := NewEdge("e1", "n1", "n2").SetWorkflowID("wf1").SetSourceKey("out").SetTargetKey("in")
	if err := store.EdgeCreate(ctx, edge); err != nil {
		t.Fatal("edge create:", err)
	}
	foundEdge, err := store.EdgeFindByID(ctx, "e1")
	if err != nil || foundEdge == nil {
		t.Fatal("edge find:", err)
	}
	if foundEdge.GetSource() != "n1" || foundEdge.GetTarget() != "n2" ||
		foundEdge.GetSourceKey() != "out" || foundEdge.GetTargetKey() != "in" ||
		foundEdge.GetWorkflowID() != "wf1" {
		t.Fatal("edge round-trip mismatch")
	}

	count, err := store.NodeCount(ctx, NodeQuery().WithWorkflowID("wf1"))
	if err != nil || count != 1 {
		t.Fatal("node count:", err, count)
	}
	count, err = store.EdgeCount(ctx, EdgeQuery().WithWorkflowID("wf1"))
	if err != nil || count != 1 {
		t.Fatal("edge count:", err, count)
	}

	if err := store.NodeDeleteByID(ctx, "n1"); err != nil {
		t.Fatal("node delete:", err)
	}
	if gone, _ := store.NodeFindByID(ctx, "n1"); gone != nil {
		t.Fatal("node should be deleted")
	}
	if err := store.EdgeDeleteByID(ctx, "e1"); err != nil {
		t.Fatal("edge delete:", err)
	}
}

// TestWorkflowChildSync verifies WorkflowUpdate creates new, updates
// changed and deletes removed children — and that a node update
// persists independently of its siblings.
func TestWorkflowChildSync(t *testing.T) {
	store := initTestStore(t)
	ctx := context.Background()

	workflow := NewWorkflow("sync")
	workflow.AddNode(NewNode("n1", "inject"))
	workflow.AddNode(NewNode("n2", "debug"))
	if err := store.WorkflowCreate(ctx, workflow); err != nil {
		t.Fatal("create:", err)
	}

	found, _ := store.WorkflowFindByID(ctx, workflow.GetID())
	// Remove n2, move n1, add n3.
	n1 := found.FindNodeByID("n1")
	n1.SetPosition(99, 99)
	found.SetNodes([]NodeInterface{n1, NewNode("n3", "function")})
	if err := store.WorkflowUpdate(ctx, found); err != nil {
		t.Fatal("update:", err)
	}

	nodes, _ := store.NodeList(ctx, NodeQuery().WithWorkflowID(workflow.GetID()))
	if len(nodes) != 2 {
		t.Fatal("expected 2 nodes after sync, got", len(nodes))
	}
	ids := map[string]bool{}
	for _, n := range nodes {
		ids[n.GetID()] = true
	}
	if !ids["n1"] || !ids["n3"] || ids["n2"] {
		t.Fatal("child sync mismatch:", ids)
	}
	reloadedN1, _ := store.NodeFindByID(ctx, "n1")
	if reloadedN1.GetPositionX() != 99 {
		t.Fatal("n1 position update did not persist independently")
	}
	if gone, _ := store.NodeFindByID(ctx, "n2"); gone != nil {
		t.Fatal("n2 should have been deleted")
	}

	// Independent node update does not touch siblings.
	n3, _ := store.NodeFindByID(ctx, "n3")
	n3.SetData(map[string]any{"x": 1})
	if err := store.NodeUpdate(ctx, n3); err != nil {
		t.Fatal("node update:", err)
	}
	reloadedN1b, _ := store.NodeFindByID(ctx, "n1")
	if reloadedN1b.GetPositionX() != 99 || len(reloadedN1b.GetData()) != 0 {
		t.Fatal("sibling node was affected by independent update")
	}

	// Deleting the workflow cascades to children.
	if err := store.WorkflowDeleteByID(ctx, workflow.GetID()); err != nil {
		t.Fatal("workflow delete:", err)
	}
	remaining, _ := store.NodeList(ctx, NodeQuery().WithWorkflowID(workflow.GetID()))
	if len(remaining) != 0 {
		t.Fatal("workflow children should be cascade-deleted")
	}
}

// TestNodeQueryFilters exercises limit/offset/sort/id filters.
func TestNodeQueryFilters(t *testing.T) {
	store := initTestStore(t)
	ctx := context.Background()
	for _, id := range []string{"a", "b", "c"} {
		if err := store.NodeCreate(ctx, NewNode(id, "t").SetWorkflowID("w")); err != nil {
			t.Fatal(err)
		}
	}
	list, err := store.NodeList(ctx, NodeQuery().WithWorkflowID("w").WithLimit(2))
	if err != nil || len(list) != 2 {
		t.Fatal("limit:", err, len(list))
	}
	list, err = store.NodeList(ctx, NodeQuery().WithWorkflowID("w").WithOffset(2))
	if err != nil || len(list) != 1 {
		t.Fatal("offset:", err, len(list))
	}
	list, err = store.NodeList(ctx, NodeQuery().WithWorkflowID("w").WithID("b"))
	if err != nil || len(list) != 1 || list[0].GetID() != "b" {
		t.Fatal("id filter:", err)
	}
	count, err := store.NodeCount(ctx, NodeQuery().WithWorkflowID("w"))
	if err != nil || count != 3 {
		t.Fatal("count:", err, count)
	}
}
