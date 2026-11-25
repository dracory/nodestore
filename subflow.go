package nodestore

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"
)

// Shared constants and scalar helpers used by the Node-RED and module
// import/export converters (ported from flowrunner, adapted to the
// interface-first domain model).

// subflowTypePrefix marks a workflow node as a subflow instance.
const subflowTypePrefix = "subflow:"

// subflowDefNodeType is the node type carrying a subflow definition.
const subflowDefNodeType = "subflow"

// subflowEnvKey is the node Data key carrying the resolved env overlay.
const subflowEnvKey = "_env"

// IsSubflowInstance reports whether nodeType names a subflow instance
// ("subflow:<defId>").
func IsSubflowInstance(nodeType string) bool {
	return strings.HasPrefix(nodeType, subflowTypePrefix)
}

// outputPortIndex maps an edge SourceKey ("output_1", "", "output_N")
// to a zero-based index into the definition's out array.
func outputPortIndex(sourceKey string) int {
	if sourceKey == "" {
		return 0
	}
	var n int
	if _, err := fmt.Sscanf(sourceKey, "output_%d", &n); err == nil && n >= 1 {
		return n - 1
	}
	return 0
}

// cloneDataMap deep-copies a node Data map via JSON so every instance
// owns its config (and its _env overlay) independently.
func cloneDataMap(data map[string]any) map[string]any {
	out := map[string]any{}
	if len(data) == 0 {
		return out
	}
	if raw, err := json.Marshal(data); err == nil {
		if json.Unmarshal(raw, &out) == nil {
			return out
		}
	}
	for k, v := range data {
		out[k] = v
	}
	return out
}

func stringOf(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

func floatOf(v any) float64 {
	if f, ok := v.(float64); ok {
		return f
	}
	return 0
}

// truthyFlag interprets a decoded-JSON flag value: boolean true, or
// the strings "true"/"1"/"yes" — Node-RED writes "d": true but
// hand-edited flows may carry other spellings.
func truthyFlag(v any) bool {
	switch t := v.(type) {
	case bool:
		return t
	case string:
		switch strings.ToLower(strings.TrimSpace(t)) {
		case "true", "1", "yes":
			return true
		}
	}
	return false
}

// CredentialSentinel is the marker stored in node.Data in place of a
// real credential value (the same convention Node-RED's editor uses,
// "__PWRD__"). Stored workflows and editor payloads carry the sentinel
// where a secret exists; the actual value lives in server-side
// credential storage and is merged back into node.Data at deploy time.
// Exports must strip sentinel values and credential keys entirely.
const CredentialSentinel = "__PWRD__"

// credentialKeys lists the config keys treated as credentials per node
// type. The node-types schema marks the same keys
// "format": "credential" — keep the two lists in sync. Node packs
// extend it via RegisterCredentialKeys.
var credentialKeys = struct {
	sync.RWMutex
	m map[string][]string
}{m: map[string][]string{
	"httpRequest": {"username", "password"},
	"mqttIn":      {"username", "password"},
	"mqttOut":     {"username", "password"},
	"mqttBroker":  {"user", "password"},
	"mailIn":      {"username", "password"},
	"mailOut":     {"username", "password"},
	"queue":       {"password"},
	"fsIn":        {"password"},
	"fsOut":       {"password"},
}}

// RegisterCredentialKeys marks config keys of a node type as
// credentials — the same treatment built-in types get: the editor
// masks them, storage moves them out of the flow document behind the
// CredentialSentinel marker, and exports strip them. Re-calling
// replaces the key list for the type.
func RegisterCredentialKeys(typeID string, keys []string) {
	credentialKeys.Lock()
	defer credentialKeys.Unlock()
	credentialKeys.m[typeID] = append([]string(nil), keys...)
}

// CredentialKeys returns the config keys of a node type that are
// treated as credentials (masked in the editor, stored outside the
// exported flow document, stripped from exports).
func CredentialKeys(nodeType string) []string {
	credentialKeys.RLock()
	defer credentialKeys.RUnlock()
	return credentialKeys.m[nodeType]
}

// IsCredentialKey reports whether key is a credential field on the
// given node type.
func IsCredentialKey(nodeType, key string) bool {
	for _, k := range CredentialKeys(nodeType) {
		if k == key {
			return true
		}
	}
	return false
}

// IsCredentialSentinel reports whether v is the sentinel string left
// in place of a stored credential value.
func IsCredentialSentinel(v any) bool {
	s, ok := v.(string)
	return ok && s == CredentialSentinel
}
