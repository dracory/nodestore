package nodestore

import (
	"database/sql"
	"testing"

	"github.com/dracory/entitystore"
	_ "modernc.org/sqlite"
)

func initTestStore(t *testing.T) *Store {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	entityStore, err := entitystore.NewStore(entitystore.NewStoreOptions{
		DB:                 db,
		EntityTableName:    "entity",
		AttributeTableName: "attribute",
		AutomigrateEnabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	store, err := NewStore(NewStoreOptions{EntityStore: entityStore})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Errorf("close db: %v", err)
		}
	})
	return store
}
