package nodestore

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/dromara/carbon/v2"
)

// nowUTC returns the current UTC time formatted the same way
// entitystore formats its timestamps
func nowUTC() string {
	return carbon.Now(carbon.UTC).ToDateTimeString(carbon.UTC)
}

// parseDatetime parses an entitystore-style datetime string.
// Returns false when the value is empty or unparsable.
func parseDatetime(value string) (time.Time, bool) {
	if value == "" {
		return time.Time{}, false
	}
	t, err := time.Parse(datetimeLayout, value)
	if err == nil {
		return t, true
	}
	// Some drivers (e.g. modernc SQLite) return DATETIME columns in
	// RFC3339 form — accept it too.
	if t, err := time.Parse(time.RFC3339, value); err == nil {
		return t, true
	}
	return time.Time{}, false
}

// formatDatetime formats a time the entitystore way
func formatDatetime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(datetimeLayout)
}

// unifyName normalizes names used for lookups
func unifyName(name string) string {
	return strings.ToLower(strings.TrimSpace(name))
}

// BinaryPayloadWireKey is the reserved single-key object used to encode
// byte slices inside JSON payload trees: {"$binary":{"encoding":"base64","data":"..."}}.
// A plain object with exactly this shape is therefore reserved in durable
// payload storage; ordinary strings (including base64-looking strings saved
// by older versions) remain strings because they carry no type marker.
const BinaryPayloadWireKey = "$binary"

// BinaryPayloadEncodingBase64 is the only supported binary wire encoding.
const BinaryPayloadEncodingBase64 = "base64"

type binaryPayloadJSON struct {
	Encoding string `json:"encoding"`
	Data     string `json:"data"`
}

// MarshalPayloadJSON serializes a payload for durable JSON storage while
// preserving byte identity. Every []byte in the payload tree is encoded as
// the reserved BinaryPayloadWireKey object; plain JSON strings, objects,
// arrays, numbers, booleans and nil keep their ordinary JSON representation.
func MarshalPayloadJSON(payload any) ([]byte, error) {
	return json.Marshal(encodePayloadBinaryValues(payload))
}

// UnmarshalPayloadJSON decodes a payload written by MarshalPayloadJSON.
// Valid binary wire objects decode back to []byte; all ordinary JSON values
// decode with encoding/json's normal types. A malformed reserved binary
// object is an error so corrupt durable rows cannot silently change type.
func UnmarshalPayloadJSON(raw []byte) (any, error) {
	var payload any
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, err
	}
	return decodePayloadBinaryValues(payload)
}

func encodePayloadBinaryValues(value any) any {
	switch v := value.(type) {
	case []byte:
		return binaryPayloadWire(v)
	case map[string]any:
		out := make(map[string]any, len(v))
		for key, item := range v {
			out[key] = encodePayloadBinaryValues(item)
		}
		return out
	case []any:
		out := make([]any, len(v))
		for i, item := range v {
			out[i] = encodePayloadBinaryValues(item)
		}
		return out
	default:
		return encodePayloadCompositeValues(value)
	}
}

func binaryPayloadWire(data []byte) map[string]any {
	return map[string]any{
		BinaryPayloadWireKey: binaryPayloadJSON{
			Encoding: BinaryPayloadEncodingBase64,
			Data:     base64.StdEncoding.EncodeToString(data),
		},
	}
}

// encodePayloadCompositeValues extends the recursive contract beyond the
// dynamically typed JSON shapes to typed string-keyed maps, slices, and
// arrays. Byte slices (including named []byte forms) become binary wire
// objects; other elements recurse so forms such as [][]byte are covered.
func encodePayloadCompositeValues(value any) any {
	if _, ok := value.(json.Marshaler); ok {
		return value
	}
	rv := reflect.ValueOf(value)
	if !rv.IsValid() {
		return value
	}
	switch rv.Kind() {
	case reflect.Slice, reflect.Array:
		if rv.Kind() == reflect.Slice && rv.IsNil() {
			return value
		}
		if rv.Type().Elem().Kind() == reflect.Uint8 {
			data := make([]byte, rv.Len())
			for i := range data {
				data[i] = byte(rv.Index(i).Uint())
			}
			return binaryPayloadWire(data)
		}
		out := make([]any, rv.Len())
		for i := range out {
			out[i] = encodePayloadBinaryValues(rv.Index(i).Interface())
		}
		return out
	case reflect.Map:
		if rv.IsNil() || rv.Type().Key().Kind() != reflect.String {
			return value
		}
		out := make(map[string]any, rv.Len())
		iter := rv.MapRange()
		for iter.Next() {
			out[iter.Key().String()] = encodePayloadBinaryValues(iter.Value().Interface())
		}
		return out
	default:
		return value
	}
}

func decodePayloadBinaryValues(value any) (any, error) {
	switch v := value.(type) {
	case map[string]any:
		if len(v) == 1 {
			if encoded, ok := v[BinaryPayloadWireKey]; ok {
				return decodeBinaryPayloadEnvelope(encoded)
			}
		}
		for key, item := range v {
			decoded, err := decodePayloadBinaryValues(item)
			if err != nil {
				return nil, fmt.Errorf("binary payload key %q: %w", key, err)
			}
			v[key] = decoded
		}
		return v, nil
	case []any:
		for i, item := range v {
			decoded, err := decodePayloadBinaryValues(item)
			if err != nil {
				return nil, fmt.Errorf("binary payload index %d: %w", i, err)
			}
			v[i] = decoded
		}
		return v, nil
	default:
		return value, nil
	}
}

func decodeBinaryPayloadEnvelope(encoded any) (any, error) {
	object, ok := encoded.(map[string]any)
	if !ok || len(object) != 2 {
		return nil, fmt.Errorf("invalid binary payload envelope")
	}
	encoding, encodingOK := object["encoding"].(string)
	data, dataOK := object["data"].(string)
	if !encodingOK || !dataOK || encoding != BinaryPayloadEncodingBase64 {
		return nil, fmt.Errorf("invalid binary payload envelope")
	}
	decoded, err := base64.StdEncoding.DecodeString(data)
	if err != nil {
		return nil, fmt.Errorf("invalid binary payload data: %w", err)
	}
	return decoded, nil
}
