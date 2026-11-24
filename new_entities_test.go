package nodestore

import (
	"context"
	"testing"
)

// TestCredentialCRUD covers the credential entity lifecycle: create,
// find, list-by-workflow, the deploy map helper, update and delete.
func TestCredentialCRUD(t *testing.T) {
	store := initTestStore(t)
	ctx := context.Background()

	c := NewCredential("wf1", "n1").SetValues(map[string]string{
		"username": "bob",
		"password": "hunter2",
	})
	if err := store.CredentialCreate(ctx, c); err != nil {
		t.Fatalf("create: %v", err)
	}
	if c.GetID() == "" {
		t.Fatal("expected entity-assigned ID")
	}

	got, err := store.CredentialFindByID(ctx, c.GetID())
	if err != nil || got == nil {
		t.Fatalf("find: %v nil=%v", err, got == nil)
	}
	if got.GetWorkflowID() != "wf1" || got.GetNodeID() != "n1" {
		t.Fatalf("wrong owner: %v %v", got.GetWorkflowID(), got.GetNodeID())
	}
	if got.GetValue("password") != "hunter2" {
		t.Fatalf("wrong value: %v", got.GetValue("password"))
	}

	byNode, err := store.CredentialFindByNode(ctx, "wf1", "n1")
	if err != nil || byNode == nil {
		t.Fatalf("find by node: %v nil=%v", err, byNode == nil)
	}

	got.SetValue("password", "new-secret").SetValue("cleared", "")
	if err := store.CredentialUpdate(ctx, got); err != nil {
		t.Fatalf("update: %v", err)
	}
	m, err := store.CredentialMapForWorkflow(ctx, "wf1")
	if err != nil {
		t.Fatalf("map: %v", err)
	}
	if m["n1"]["password"] != "new-secret" {
		t.Fatalf("map value: %v", m["n1"])
	}
	if _, ok := m["n1"]["cleared"]; ok {
		t.Fatal("cleared key must be absent")
	}

	// Replace drops entities missing from the new map and adds new ones.
	if err := store.CredentialReplaceForWorkflow(ctx, "wf1", map[string]map[string]string{
		"n2": {"token": "t"},
	}); err != nil {
		t.Fatalf("replace: %v", err)
	}
	list, err := store.CredentialList(ctx, CredentialQuery().WithWorkflowID("wf1"))
	if err != nil || len(list) != 1 || list[0].GetNodeID() != "n2" {
		t.Fatalf("replace result: %v %v", err, list)
	}

	if err := store.CredentialDeleteByWorkflowID(ctx, "wf1"); err != nil {
		t.Fatalf("delete by workflow: %v", err)
	}
	n, _ := store.CredentialCount(ctx, CredentialQuery().WithWorkflowID("wf1"))
	if n != 0 {
		t.Fatalf("expected 0 credentials, got %d", n)
	}
}

// TestDeployRecordCRUD covers append, newest-first listing and the
// retention prune.
func TestDeployRecordCRUD(t *testing.T) {
	store := initTestStore(t)
	ctx := context.Background()

	for i := 0; i < DeployRecordRetention+5; i++ {
		r := NewDeployRecord("wf1", "local", `{"i":`+string(rune('0'+i%10))+`}`)
		if err := store.DeployRecordCreate(ctx, r); err != nil {
			t.Fatalf("create %d: %v", i, err)
		}
	}
	list, err := store.DeployRecordList(ctx, DeployRecordQuery().WithWorkflowID("wf1"))
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != DeployRecordRetention {
		t.Fatalf("retention: expected %d records, got %d", DeployRecordRetention, len(list))
	}
	// Newest first.
	for i := 1; i < len(list); i++ {
		if list[i-1].GetCreatedAt().Before(list[i].GetCreatedAt()) {
			t.Fatal("expected newest-first order")
		}
	}
	rec := list[0]
	if rec.GetActor() != "local" || rec.GetWorkflowID() != "wf1" {
		t.Fatalf("record fields: %v %v", rec.GetActor(), rec.GetWorkflowID())
	}
	if err := store.DeployRecordDeleteByWorkflowID(ctx, "wf1"); err != nil {
		t.Fatalf("delete by workflow: %v", err)
	}
	n, _ := store.DeployRecordCount(ctx, DeployRecordQuery().WithWorkflowID("wf1"))
	if n != 0 {
		t.Fatalf("expected 0 records, got %d", n)
	}
}

// TestNodeGroupCRUD covers node_group entities plus group membership
// carried as group_id on the node entity.
func TestNodeGroupCRUD(t *testing.T) {
	store := initTestStore(t)
	ctx := context.Background()

	wf := NewWorkflow("w").SetID("wf1")
	node := NewNode("n1", "inject").SetWorkflowID("wf1").SetGroupID("g1")
	wf.SetNodes([]NodeInterface{node})
	if err := store.WorkflowCreate(ctx, wf); err != nil {
		t.Fatalf("workflow create: %v", err)
	}
	persisted, err := store.NodeFindByID(ctx, "n1")
	if err != nil || persisted == nil {
		t.Fatalf("node find: %v nil=%v", err, persisted == nil)
	}
	if persisted.GetGroupID() != "g1" {
		t.Fatalf("group_id not persisted: %q", persisted.GetGroupID())
	}

	g := NewNodeGroup("wf1", "Inputs").SetID("g1").SetData(map[string]any{
		"x": 10.0, "y": 20.0, "w": 100.0, "h": 50.0,
	})
	if err := store.NodeGroupCreate(ctx, g); err != nil {
		t.Fatalf("create: %v", err)
	}
	got, err := store.NodeGroupFindByID(ctx, "g1")
	if err != nil || got == nil {
		t.Fatalf("find: %v nil=%v", err, got == nil)
	}
	if got.GetName() != "Inputs" || got.GetData()["w"] != 100.0 {
		t.Fatalf("group fields: %v %v", got.GetName(), got.GetData())
	}
	list, err := store.NodeGroupList(ctx, NodeGroupQuery().WithWorkflowID("wf1"))
	if err != nil || len(list) != 1 {
		t.Fatalf("list: %v %v", err, list)
	}
	got.SetName("Renamed")
	if err := store.NodeGroupUpdate(ctx, got); err != nil {
		t.Fatalf("update: %v", err)
	}
	if err := store.NodeGroupDeleteByID(ctx, "g1"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if gone, _ := store.NodeGroupFindByID(ctx, "g1"); gone != nil {
		t.Fatal("expected group deleted")
	}
}

// TestWorkflowGroupCRUD covers sidebar workflow_group entities.
func TestWorkflowGroupCRUD(t *testing.T) {
	store := initTestStore(t)
	ctx := context.Background()

	g := NewWorkflowGroup("Prod").SetID("wg1").
		SetParentID("").SetWorkflowIDs([]string{"wf1", "wf2"})
	if err := store.WorkflowGroupCreate(ctx, g); err != nil {
		t.Fatalf("create: %v", err)
	}
	child := NewWorkflowGroup("EU").SetID("wg2").SetParentID("wg1")
	if err := store.WorkflowGroupCreate(ctx, child); err != nil {
		t.Fatalf("create child: %v", err)
	}

	got, err := store.WorkflowGroupFindByID(ctx, "wg1")
	if err != nil || got == nil {
		t.Fatalf("find: %v nil=%v", err, got == nil)
	}
	if got.GetName() != "Prod" || len(got.GetWorkflowIDs()) != 2 {
		t.Fatalf("group fields: %v %v", got.GetName(), got.GetWorkflowIDs())
	}
	kids, err := store.WorkflowGroupList(ctx, WorkflowGroupQuery().WithParentID("wg1"))
	if err != nil || len(kids) != 1 || kids[0].GetID() != "wg2" {
		t.Fatalf("children: %v %v", err, kids)
	}
	got.SetWorkflowIDs([]string{"wf3"})
	if err := store.WorkflowGroupUpdate(ctx, got); err != nil {
		t.Fatalf("update: %v", err)
	}
	if err := store.WorkflowGroupDeleteByID(ctx, "wg2"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	n, _ := store.WorkflowGroupCount(ctx, WorkflowGroupQuery())
	if n != 1 {
		t.Fatalf("expected 1 group, got %d", n)
	}
}
