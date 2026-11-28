package nodestore

import (
	"context"
	"testing"
)

func TestWorkflowCreateFindUpdate(t *testing.T) {
	store := initTestStore(t)
	ctx := context.Background()

	workflow := NewWorkflow("Daily Report").
		SetDescription("sends the daily report").
		SetDeployed(true).
		SetPaused(true)

	node := NewNode("n1", "inject").SetPosition(10, 20)
	node.SetData(map[string]any{"interval": "60"})
	workflow.AddNode(node)
	workflow.AddNode(NewNode("n2", "debug").SetPosition(100, 20))
	workflow.AddEdge(NewEdge("e1", "n1", "n2").SetSourceKey("out").SetTargetKey("in"))

	if err := store.WorkflowCreate(ctx, workflow); err != nil {
		t.Fatal("create:", err)
	}

	found, err := store.WorkflowFindByID(ctx, workflow.GetID())
	if err != nil {
		t.Fatal("find:", err)
	}
	if found == nil {
		t.Fatal("find: workflow not found")
	}
	if found.GetName() != "Daily Report" || !found.GetDeployed() || !found.GetPaused() {
		t.Fatal("name/deployed/paused mismatch")
	}
	if len(found.GetNodes()) != 2 || len(found.GetEdges()) != 1 {
		t.Fatal("nodes/edges count mismatch")
	}
	n1 := found.FindNodeByID("n1")
	if n1 == nil || n1.GetType() != "inject" || n1.GetPositionX() != 10 {
		t.Fatal("node round-trip mismatch")
	}
	if n1.GetData()["interval"] != "60" {
		t.Fatal("node data mismatch")
	}
	edge := found.GetEdges()[0]
	if edge.GetSource() != "n1" || edge.GetTargetKey() != "in" {
		t.Fatal("edge round-trip mismatch")
	}

	byName, err := store.WorkflowFindByName(ctx, "daily report")
	if err != nil || byName == nil {
		t.Fatal("find by name failed")
	}

	found.SetDescription("updated").SetDeployed(false).SetPaused(false)
	if err := store.WorkflowUpdate(ctx, found); err != nil {
		t.Fatal("update:", err)
	}
	reloaded, err := store.WorkflowFindByID(ctx, workflow.GetID())
	if err != nil {
		t.Fatal("reload:", err)
	}
	if reloaded == nil {
		t.Fatal("reload: workflow not found")
	}
	if reloaded.GetDescription() != "updated" || reloaded.GetDeployed() || reloaded.GetPaused() {
		t.Fatal("update round-trip mismatch")
	}

	count, err := store.WorkflowCount(ctx, WorkflowQuery().WithDeployed(false).WithPaused(false))
	if err != nil || count != 1 {
		t.Fatal("count mismatch")
	}
}
