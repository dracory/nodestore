package nodestore

import (
	"bytes"
	"context"
	"reflect"
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

func TestMarshalPayloadJSONBinaryWireShape(t *testing.T) {
	raw, err := MarshalPayloadJSON([]byte{0, 1, 0xff})
	if err != nil {
		t.Fatal("marshal:", err)
	}
	want := `{"$binary":{"encoding":"base64","data":"AAH/"}}`
	if string(raw) != want {
		t.Fatalf("binary wire shape = %s, want %s", raw, want)
	}
	decoded, err := UnmarshalPayloadJSON(raw)
	if err != nil {
		t.Fatal("unmarshal:", err)
	}
	bytesValue, ok := decoded.([]byte)
	if !ok {
		t.Fatalf("decoded payload is %T, want []byte", decoded)
	}
	if !bytes.Equal(bytesValue, []byte{0, 1, 0xff}) {
		t.Fatalf("decoded bytes = %v", bytesValue)
	}
}

func TestMessageBinaryPayloadDurableRoundTrip(t *testing.T) {
	store := initTestStore(t)
	ctx := context.Background()
	original := []byte{0, 1, 2, 0xff, 'h', 'i'}
	message := NewMessage().SetPayload(original)
	if err := store.MessageCreate(ctx, message); err != nil {
		t.Fatal("create:", err)
	}

	found, err := store.MessageFindByID(ctx, message.GetID())
	if err != nil {
		t.Fatal("find:", err)
	}
	payload, ok := found.GetPayload().([]byte)
	if !ok {
		t.Fatalf("payload is %T, want []byte", found.GetPayload())
	}
	if !bytes.Equal(payload, original) {
		t.Fatalf("payload bytes = %v, want %v", payload, original)
	}
}

func TestMessageNestedBinaryPayloadDurableRoundTrip(t *testing.T) {
	store := initTestStore(t)
	ctx := context.Background()
	message := NewMessage().SetPayload(map[string]any{
		"file":  []byte{0, 0xff, 'p', 'd', 'f'},
		"parts": []any{[]byte{1, 2}, "text"},
	})
	if err := store.MessageCreate(ctx, message); err != nil {
		t.Fatal("create:", err)
	}

	found, err := store.MessageFindByID(ctx, message.GetID())
	if err != nil {
		t.Fatal("find:", err)
	}
	payload, ok := found.GetPayload().(map[string]any)
	if !ok {
		t.Fatalf("payload is %T, want map", found.GetPayload())
	}
	fileBytes, ok := payload["file"].([]byte)
	if !ok || !bytes.Equal(fileBytes, []byte{0, 0xff, 'p', 'd', 'f'}) {
		t.Fatalf("nested file payload = %#v (%T)", payload["file"], payload["file"])
	}
	parts, ok := payload["parts"].([]any)
	if !ok || len(parts) != 2 {
		t.Fatalf("parts payload = %#v", payload["parts"])
	}
	partBytes, ok := parts[0].([]byte)
	if !ok || !bytes.Equal(partBytes, []byte{1, 2}) {
		t.Fatalf("nested part payload = %#v (%T)", parts[0], parts[0])
	}
	if parts[1] != "text" {
		t.Fatalf("plain string part changed: %#v", parts[1])
	}
}

func TestMessageScalarPayloadRoundTripsRemainUnchanged(t *testing.T) {
	store := initTestStore(t)
	ctx := context.Background()
	tests := []struct {
		name    string
		payload any
		want    any
	}{
		{name: "plain base64-looking string stays a string", payload: "AAH/", want: "AAH/"},
		{name: "object", payload: map[string]any{"a": 1}, want: map[string]any{"a": float64(1)}},
		{name: "array", payload: []any{"x", 2}, want: []any{"x", float64(2)}},
		{name: "number", payload: 3.5, want: float64(3.5)},
		{name: "boolean", payload: true, want: true},
		{name: "nil", payload: nil, want: nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			message := NewMessage().SetPayload(tt.payload)
			if err := store.MessageCreate(ctx, message); err != nil {
				t.Fatal("create:", err)
			}
			found, err := store.MessageFindByID(ctx, message.GetID())
			if err != nil {
				t.Fatal("find:", err)
			}
			if !reflect.DeepEqual(found.GetPayload(), tt.want) {
				t.Fatalf("payload = %#v (%T), want %#v (%T)", found.GetPayload(), found.GetPayload(), tt.want, tt.want)
			}
		})
	}
}
