package nodestore

import (
	"context"
	"testing"
)

func TestMessageCreateFindUpdate(t *testing.T) {
	store := initTestStore(t)
	ctx := context.Background()

	message := NewMessage().
		SetPayload(map[string]any{"temp": 21}).
		SetMetadata(map[string]any{"msgId": "abc"})
	message.SetError("upstream failed")

	if err := store.MessageCreate(ctx, message); err != nil {
		t.Fatal("create:", err)
	}

	found, err := store.MessageFindByID(ctx, message.GetID())
	if err != nil {
		t.Fatal("find:", err)
	}
	if found == nil {
		t.Fatal("find: message not found")
	}
	payload, ok := found.GetPayload().(map[string]any)
	if !ok || payload["temp"] != float64(21) {
		t.Fatal("payload round-trip mismatch")
	}
	if found.GetMetadata()["msgId"] != "abc" {
		t.Fatal("metadata mismatch")
	}
	if found.GetError() == nil || *found.GetError() != "upstream failed" {
		t.Fatal("error mismatch")
	}

	found.ClearError()
	if err := store.MessageUpdate(ctx, found); err != nil {
		t.Fatal("update:", err)
	}
	reloaded, _ := store.MessageFindByID(ctx, message.GetID())
	if reloaded.GetError() != nil {
		t.Fatal("error should be cleared")
	}
}
