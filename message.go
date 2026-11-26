package nodestore

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/dracory/entitystore"
)

// == INTERFACE ===============================================================

// MessageInterface is the interface form of flowrunner.Message — the
// standardized envelope for data flowing between nodes, persisted as an
// entity (type EntityTypeMessage) so the runner has durable messages.
type MessageInterface interface {
	GetID() string
	SetID(id string) MessageInterface

	// GetPayload returns the message payload (typically
	// map[string]any or []byte after JSON round-trip)
	GetPayload() any
	SetPayload(payload any) MessageInterface

	// GetMetadata returns the message metadata
	GetMetadata() map[string]any
	SetMetadata(metadata map[string]any) MessageInterface

	// GetError returns the error message if an upstream node failed
	GetError() *string
	SetError(err string) MessageInterface
	ClearError() MessageInterface

	// GetTimestamp returns the timestamp when the message was created/last modified
	GetTimestamp() time.Time
	SetTimestamp(timestamp time.Time) MessageInterface

	// GetCreatedAt returns the creation timestamp
	GetCreatedAt() time.Time
	SetCreatedAt(createdAt time.Time) MessageInterface

	// GetUpdatedAt returns the last update timestamp
	GetUpdatedAt() time.Time
	SetUpdatedAt(updatedAt time.Time) MessageInterface
}

// == IMPLEMENTATION ==========================================================

// messageManagedAttributes lists the attribute keys a message owns —
// including the legacy ATTRIBUTE_TIMESTAMP so updates clean it up.
var messageManagedAttributes = map[string]bool{
	ATTRIBUTE_PAYLOAD:   true,
	ATTRIBUTE_METADATA:  true,
	ATTRIBUTE_ERROR:     true,
	ATTRIBUTE_TIMESTAMP: true, // legacy, cleaned up on update
}

type messageImplementation struct {
	entity   entitystore.EntityInterface
	payload  any
	metadata map[string]any
	err      *string
}

// NewMessage creates a new message
func NewMessage() MessageInterface {
	entity := entitystore.NewEntity()
	entity.SetType(EntityTypeMessage)
	return &messageImplementation{
		entity:   entity,
		metadata: map[string]any{},
	}
}

// messageFromEntity rebuilds a message from a persisted entity + attributes
func messageFromEntity(entity entitystore.EntityInterface, attrs map[string]string) (MessageInterface, error) {
	m := &messageImplementation{entity: entity, metadata: map[string]any{}}

	if raw := attrs[ATTRIBUTE_PAYLOAD]; raw != "" {
		payload, err := UnmarshalPayloadJSON([]byte(raw))
		if err != nil {
			return nil, err
		}
		m.payload = payload
	}
	if raw := attrs[ATTRIBUTE_METADATA]; raw != "" {
		metadata, err := UnmarshalPayloadJSON([]byte(raw))
		if err != nil {
			return nil, err
		}
		metadataMap, ok := metadata.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("message metadata is not a JSON object")
		}
		m.metadata = metadataMap
	}
	if raw := attrs[ATTRIBUTE_ERROR]; raw != "" {
		m.err = &raw
	}
	// Legacy fallback: pre-change records stored a separate timestamp
	// attribute that duplicated (and could diverge from) created_at.
	// Fold it onto the entity timestamp so old records read correctly.
	if t, ok := parseDatetime(attrs[ATTRIBUTE_TIMESTAMP]); ok {
		m.entity.SetCreatedAt(formatDatetime(t))
	}
	return m, nil
}

func (m *messageImplementation) attributes() (map[string]string, error) {
	payload, err := MarshalPayloadJSON(m.payload)
	if err != nil {
		return nil, err
	}
	metadata, err := MarshalPayloadJSON(m.metadata)
	if err != nil {
		return nil, err
	}
	attrs := map[string]string{
		ATTRIBUTE_PAYLOAD:  string(payload),
		ATTRIBUTE_METADATA: string(metadata),
	}
	if m.err != nil {
		attrs[ATTRIBUTE_ERROR] = *m.err
	}
	return attrs, nil
}

func (m *messageImplementation) entity_() entitystore.EntityInterface {
	return m.entity
}

// MarshalJSON renders the wire/debug shape of a message
// ({"id","payload","metadata","error","timestamp"}) — used by debug
// complete=true, run-journal records and tests that inspect the event
// log. The struct itself has unexported fields so encoding/json needs
// this hook.
func (m *messageImplementation) MarshalJSON() ([]byte, error) {
	return json.Marshal(map[string]any{
		"id":        m.GetID(),
		"payload":   m.payload,
		"metadata":  m.metadata,
		"error":     m.err,
		"timestamp": m.GetTimestamp(),
	})
}

func (m *messageImplementation) GetID() string                    { return m.entity.GetID() }
func (m *messageImplementation) SetID(id string) MessageInterface { m.entity.SetID(id); return m }

func (m *messageImplementation) GetPayload() any { return m.payload }
func (m *messageImplementation) SetPayload(payload any) MessageInterface {
	m.payload = payload
	return m
}

func (m *messageImplementation) GetMetadata() map[string]any { return m.metadata }
func (m *messageImplementation) SetMetadata(metadata map[string]any) MessageInterface {
	if metadata == nil {
		metadata = map[string]any{}
	}
	m.metadata = metadata
	return m
}

func (m *messageImplementation) GetError() *string { return m.err }
func (m *messageImplementation) SetError(err string) MessageInterface {
	m.err = &err
	return m
}
func (m *messageImplementation) ClearError() MessageInterface {
	m.err = nil
	return m
}

// GetTimestamp returns the message timestamp — the entity's created_at
// (message creation time IS the entity creation time; no separate field).
func (m *messageImplementation) GetTimestamp() time.Time { return m.GetCreatedAt() }
func (m *messageImplementation) SetTimestamp(timestamp time.Time) MessageInterface {
	return m.SetCreatedAt(timestamp)
}

func (m *messageImplementation) GetCreatedAt() time.Time {
	t, _ := parseDatetime(m.entity.GetCreatedAt())
	return t
}
func (m *messageImplementation) SetCreatedAt(createdAt time.Time) MessageInterface {
	m.entity.SetCreatedAt(formatDatetime(createdAt))
	return m
}

func (m *messageImplementation) GetUpdatedAt() time.Time {
	t, _ := parseDatetime(m.entity.GetUpdatedAt())
	return t
}
func (m *messageImplementation) SetUpdatedAt(updatedAt time.Time) MessageInterface {
	m.entity.SetUpdatedAt(formatDatetime(updatedAt))
	return m
}
