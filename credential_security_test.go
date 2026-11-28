package nodestore

import (
	"context"
	"database/sql"
	"strings"
	"testing"

	"github.com/dracory/entitystore"
	_ "modernc.org/sqlite"
)

func newTestStoreWithCipher(t *testing.T, cipher CredentialCipher) *Store {
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
		t.Fatalf("entitystore: %v", err)
	}
	st, err := NewStore(NewStoreOptions{EntityStore: entityStore, CredentialCipher: cipher})
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Errorf("close db: %v", err)
		}
	})
	return st
}

func TestCredentialCipherRoundTrip(t *testing.T) {
	c, err := NewAESCredentialCipher([]byte("0123456789abcdef0123456789abcdef"))
	if err != nil {
		t.Fatalf("cipher: %v", err)
	}
	sealed, err := c.Encrypt([]byte(`{"password":"hunter2"}`))
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	if !strings.HasPrefix(sealed, "v1:") {
		t.Fatalf("sealed value missing v1 prefix: %q", sealed)
	}
	back, err := c.Decrypt(sealed)
	if err != nil {
		t.Fatalf("decrypt: %v", err)
	}
	if string(back) != `{"password":"hunter2"}` {
		t.Fatalf("round trip mismatch: %s", back)
	}
	// Legacy plaintext passes through unchanged.
	plain, err := c.Decrypt(`{"password":"old"}`)
	if err != nil || string(plain) != `{"password":"old"}` {
		t.Fatalf("legacy passthrough failed: %q %v", plain, err)
	}
	// Wrong key fails closed.
	other, err := NewAESCredentialCipher([]byte("abcdef0123456789abcdef0123456789"))
	if err != nil {
		t.Fatalf("wrong-key cipher construction: %v", err)
	}
	if _, err := other.Decrypt(sealed); err == nil {
		t.Fatal("decrypt with wrong key succeeded")
	}
	if _, err := NewAESCredentialCipher([]byte("short")); err == nil {
		t.Fatal("short key accepted")
	}
}

func TestCredentialEncryptedAtRest(t *testing.T) {
	cipher, err := NewAESCredentialCipher([]byte("0123456789abcdef0123456789abcdef"))
	if err != nil {
		t.Fatalf("cipher: %v", err)
	}
	st := newTestStoreWithCipher(t, cipher)
	ctx := context.Background()

	cred := NewCredential("wf1", "n1").SetValue("password", "hunter2")
	if err := st.CredentialCreate(ctx, cred); err != nil {
		t.Fatalf("create: %v", err)
	}

	// Raw attribute on disk must NOT contain the plaintext secret.
	attrs, err := st.entityStore.EntityAttributeList(ctx, cred.GetID())
	if err != nil {
		t.Fatalf("attrs: %v", err)
	}
	var raw string
	for _, a := range attrs {
		if a.GetKey() == ATTRIBUTE_DATA {
			raw = a.GetValue()
		}
	}
	if strings.Contains(raw, "hunter2") {
		t.Fatalf("plaintext secret persisted: %q", raw)
	}
	if !strings.HasPrefix(raw, "v1:") {
		t.Fatalf("expected sealed v1 value, got %q", raw)
	}

	// Read path opens it transparently.
	got, err := st.CredentialFindByID(ctx, cred.GetID())
	if err != nil {
		t.Fatalf("find: %v", err)
	}
	if got == nil {
		t.Fatal("find: credential not found")
	}
	if got.GetValue("password") != "hunter2" {
		t.Fatalf("decrypted value mismatch: %v", got.GetValues())
	}
}

func TestPlaintextCredentialRejectedOnWrite(t *testing.T) {
	st := newTestStoreWithCipher(t, nil)
	ctx := context.Background()

	bad := NewNode("n1", "mqttOut").SetData(map[string]any{"password": "hunter2"})
	if err := st.NodeCreate(ctx, bad); err == nil {
		t.Fatal("NodeCreate accepted a plaintext credential")
	}

	w := NewWorkflow("wf").SetID("wf-guard")
	w.AddNode(NewNode("n2", "httpRequest").SetData(map[string]any{"password": "x"}))
	if err := st.WorkflowCreate(ctx, w); err == nil {
		t.Fatal("WorkflowCreate accepted a plaintext credential")
	}

	rec := NewDeployRecord("wf-guard", "tester",
		`{"nodes":[{"id":"n","type":"mqttOut","data":{"password":"x"}}]}`)
	if err := st.DeployRecordCreate(ctx, rec); err == nil {
		t.Fatal("DeployRecordCreate accepted a flow snapshot with plaintext credentials")
	}

	// Sentinel values remain legal.
	ok := NewNode("n3", "mqttOut").SetData(map[string]any{"password": CredentialSentinel})
	if err := st.NodeCreate(ctx, ok); err != nil {
		t.Fatalf("sentinel credential rejected: %v", err)
	}
}
