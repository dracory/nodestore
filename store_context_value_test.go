package nodestore

import (
	"context"
	"database/sql"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/dracory/entitystore"
	_ "modernc.org/sqlite"
)

func initFileTestStore(t *testing.T, dbPath string) (*Store, *sql.DB) {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+filepath.ToSlash(dbPath)+"?_pragma=busy_timeout(5000)")
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	entityStore, err := entitystore.NewStore(entitystore.NewStoreOptions{
		DB:                 db,
		EntityTableName:    "node_entity",
		AttributeTableName: "node_attribute",
		AutomigrateEnabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	store, err := NewStore(NewStoreOptions{EntityStore: entityStore})
	if err != nil {
		t.Fatal(err)
	}
	return store, db
}

func TestContextValueRoundTripKeysAndDelete(t *testing.T) {
	store := initTestStore(t)
	ctx := context.Background()

	if err := store.ContextValueSet(ctx, "wf1", "flow", "wf1", "alpha", `"a"`); err != nil {
		t.Fatalf("set flow: %v", err)
	}
	if err := store.ContextValueSet(ctx, "wf1", "node", "wf1/n1", "beta", `{"x":1}`); err != nil {
		t.Fatalf("set node: %v", err)
	}
	if err := store.ContextValueSet(ctx, "wf2", "flow", "wf2", "alpha", `"other"`); err != nil {
		t.Fatalf("set other flow: %v", err)
	}
	if err := store.ContextValueSet(ctx, "", "global", "", "shared", `"g"`); err != nil {
		t.Fatalf("set global: %v", err)
	}

	value, ok, err := store.ContextValueGet(ctx, "flow", "wf1", "alpha")
	if err != nil || !ok || value != `"a"` {
		t.Fatalf("flow get: value=%q ok=%v err=%v", value, ok, err)
	}
	value, ok, err = store.ContextValueGet(ctx, "flow", "wf2", "alpha")
	if err != nil || !ok || value != `"other"` {
		t.Fatalf("scope isolation failed: value=%q ok=%v err=%v", value, ok, err)
	}

	keys, err := store.ContextValueKeys(ctx, "flow", "wf1")
	if err != nil || !reflect.DeepEqual(keys, []string{"alpha"}) {
		t.Fatalf("flow keys: %v err=%v", keys, err)
	}
	keys, err = store.ContextValueKeys(ctx, "node", "wf1/n1")
	if err != nil || !reflect.DeepEqual(keys, []string{"beta"}) {
		t.Fatalf("node keys: %v err=%v", keys, err)
	}

	if err := store.ContextValueSet(ctx, "wf1", "flow", "wf1", "alpha", `"new"`); err != nil {
		t.Fatalf("update flow: %v", err)
	}
	value, ok, err = store.ContextValueGet(ctx, "flow", "wf1", "alpha")
	if err != nil || !ok || value != `"new"` {
		t.Fatalf("updated get: value=%q ok=%v err=%v", value, ok, err)
	}

	if err := store.ContextValueDelete(ctx, "flow", "wf1", "alpha"); err != nil {
		t.Fatalf("delete key: %v", err)
	}
	if _, ok, err = store.ContextValueGet(ctx, "flow", "wf1", "alpha"); err != nil || ok {
		t.Fatalf("deleted key still present: ok=%v err=%v", ok, err)
	}
	if _, ok, err = store.ContextValueGet(ctx, "flow", "wf2", "alpha"); err != nil || !ok {
		t.Fatalf("other workflow key was deleted: ok=%v err=%v", ok, err)
	}

	if err := store.ContextValueDeleteScope(ctx, "node", "wf1/n1"); err != nil {
		t.Fatalf("delete scope: %v", err)
	}
	keys, err = store.ContextValueKeys(ctx, "node", "wf1/n1")
	if err != nil || len(keys) != 0 {
		t.Fatalf("node scope not empty: %v err=%v", keys, err)
	}
	if _, ok, err = store.ContextValueGet(ctx, "global", "", "shared"); err != nil || !ok {
		t.Fatalf("global key was deleted: ok=%v err=%v", ok, err)
	}
}

func TestContextValuePersistenceAcrossReopen(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "context.db")
	ctx := context.Background()

	store1, db1 := initFileTestStore(t, dbPath)
	if err := store1.ContextValueSet(ctx, "wf1", "flow", "wf1", "number", `41`); err != nil {
		t.Fatalf("set flow: %v", err)
	}
	if err := store1.ContextValueSet(ctx, "", "global", "", "boot", `"yes"`); err != nil {
		t.Fatalf("set global: %v", err)
	}
	if err := db1.Close(); err != nil {
		t.Fatalf("close first db: %v", err)
	}

	store2, db2 := initFileTestStore(t, dbPath)
	defer func() { _ = db2.Close() }()
	value, ok, err := store2.ContextValueGet(ctx, "flow", "wf1", "number")
	if err != nil || !ok || value != `41` {
		t.Fatalf("flow value did not survive reopen: value=%q ok=%v err=%v", value, ok, err)
	}
	value, ok, err = store2.ContextValueGet(ctx, "global", "", "boot")
	if err != nil || !ok || value != `"yes"` {
		t.Fatalf("global value did not survive reopen: value=%q ok=%v err=%v", value, ok, err)
	}
}

func TestWorkflowDeleteCascadesContextValues(t *testing.T) {
	store, db := initFileTestStore(t, filepath.Join(t.TempDir(), "context-delete.db"))
	defer func() { _ = db.Close() }()
	ctx := context.Background()

	workflow := NewWorkflow("delete me").SetID("wf-del")
	if err := store.WorkflowCreate(ctx, workflow); err != nil {
		t.Fatalf("create workflow: %v", err)
	}
	if err := store.ContextValueSet(ctx, "wf-del", "flow", "wf-del", "flow-key", `"f"`); err != nil {
		t.Fatalf("set flow: %v", err)
	}
	if err := store.ContextValueSet(ctx, "wf-del", "node", "wf-del/n1", "node-key", `"n"`); err != nil {
		t.Fatalf("set node: %v", err)
	}
	if err := store.ContextValueSet(ctx, "wf-other", "flow", "wf-other", "other-key", `"o"`); err != nil {
		t.Fatalf("set other: %v", err)
	}
	if err := store.ContextValueSet(ctx, "", "global", "", "global-key", `"g"`); err != nil {
		t.Fatalf("set global: %v", err)
	}

	if err := store.WorkflowDeleteByID(ctx, "wf-del"); err != nil {
		t.Fatalf("delete workflow: %v", err)
	}
	if keys, err := store.ContextValueKeys(ctx, "flow", "wf-del"); err != nil || len(keys) != 0 {
		t.Fatalf("flow context survived delete: %v err=%v", keys, err)
	}
	if keys, err := store.ContextValueKeys(ctx, "node", "wf-del/n1"); err != nil || len(keys) != 0 {
		t.Fatalf("node context survived delete: %v err=%v", keys, err)
	}
	if _, ok, err := store.ContextValueGet(ctx, "flow", "wf-other", "other-key"); err != nil || !ok {
		t.Fatalf("other workflow context deleted: ok=%v err=%v", ok, err)
	}
	if _, ok, err := store.ContextValueGet(ctx, "global", "", "global-key"); err != nil || !ok {
		t.Fatalf("global context deleted: ok=%v err=%v", ok, err)
	}
}
