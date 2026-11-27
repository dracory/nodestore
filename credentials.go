package nodestore

import (
	"encoding/json"
	"fmt"
)

// plaintextCredentialKey returns the first credential-typed config key
// in data that holds a real (non-sentinel, non-empty) string value.
// Such values must never reach storage — they belong in credential
// entities behind the sentinel marker.
func plaintextCredentialKey(nodeType string, data map[string]any) string {
	for _, k := range CredentialKeys(nodeType) {
		s, isStr := data[k].(string)
		if isStr && s != "" && s != CredentialSentinel {
			return k
		}
	}
	return ""
}

// validateNodeCredentialSentinels rejects a node whose data carries a
// plaintext credential value (including inner nodes of component
// definitions). Enforced on every write so secrets can never be
// persisted in node documents by mistake.
func validateNodeCredentialSentinels(n NodeInterface) error {
	if n == nil {
		return nil
	}
	if key := plaintextCredentialKey(n.GetType(), n.GetData()); key != "" {
		return fmt.Errorf("node %q (%s): credential key %q holds a plaintext value — persist secrets via Credential entities, not node data", n.GetID(), n.GetType(), key)
	}
	if n.GetType() == componentDefNodeType {
		for _, m := range innerNodeData(n.GetData()) {
			inner, _ := m["data"].(map[string]any)
			if key := plaintextCredentialKey(stringOf(m["type"]), inner); key != "" {
				return fmt.Errorf("component definition %q: inner node %q (%s) credential key %q holds a plaintext value", n.GetID(), stringOf(m["id"]), stringOf(m["type"]), key)
			}
		}
	}
	return nil
}

// validateWorkflowCredentialSentinels applies the node check to every
// node of a workflow.
func validateWorkflowCredentialSentinels(w WorkflowInterface) error {
	for _, n := range w.GetNodes() {
		if err := validateNodeCredentialSentinels(n); err != nil {
			return err
		}
	}
	return nil
}

// validateFlowJSONCredentials scans a raw flow document (deploy
// record snapshot — backend {type, data} or Node-RED flat shape) for
// plaintext credential values. Returns the offending key/type.
func validateFlowJSONCredentials(raw string) error {
	var doc any
	if err := json.Unmarshal([]byte(raw), &doc); err != nil {
		return nil // not parseable as JSON — nothing to scan
	}
	if key, typ, found := scanJSONForPlaintextCredential(doc); found {
		return fmt.Errorf("flow document: node type %q carries credential key %q with a plaintext value — snapshot must contain sentinel markers only", typ, key)
	}
	return nil
}

// scanJSONForPlaintextCredential walks decoded JSON looking for node
// objects — anything with a string "type" — and checks their
// credential keys both inline (Node-RED flat shape) and under "data"
// (backend/inner-node shape). Recurses into nested structures so
// component definition graphs are covered.
func scanJSONForPlaintextCredential(v any) (key, nodeType string, found bool) {
	switch t := v.(type) {
	case map[string]any:
		typ := stringOf(t["type"])
		if key := plaintextCredentialKey(typ, t); key != "" {
			return key, typ, true
		}
		if d, ok := t["data"].(map[string]any); ok {
			if key := plaintextCredentialKey(typ, d); key != "" {
				return key, typ, true
			}
		}
		for _, v2 := range t {
			if key, typ, found := scanJSONForPlaintextCredential(v2); found {
				return key, typ, true
			}
		}
	case []any:
		for _, item := range t {
			if key, typ, found := scanJSONForPlaintextCredential(item); found {
				return key, typ, true
			}
		}
	}
	return "", "", false
}

// credentials.go ports flowadmin/storage/credentials.go onto the
// nodestore domain types: credential values live outside the workflow
// document as `credential` entities — node.Data keeps only the
// CredentialSentinel marker — and are merged back at deploy time via
// ApplyWorkflowCredentials.

// extractCredentialFields moves credential values out of a single
// node-data map and returns the stored credential map for that node
// (nil when nothing is stored). Semantics per key:
//
//   - real string value      → stored, data gets the sentinel
//   - sentinel + stored old  → old value kept, data keeps sentinel
//   - sentinel, nothing old  → key dropped
//   - ""                     → explicit clear, stored value dropped
//   - key absent + stored old→ old value kept, sentinel re-attached
func extractCredentialFields(nodeType string, data map[string]any, existing map[string]string) map[string]string {
	keys := CredentialKeys(nodeType)
	if len(keys) == 0 || data == nil {
		return nil
	}
	var out map[string]string
	for _, k := range keys {
		v, present := data[k]
		s, isStr := v.(string)
		switch {
		case !present:
			if old := existing[k]; old != "" {
				if out == nil {
					out = map[string]string{}
				}
				out[k] = old
				data[k] = CredentialSentinel
			}
		case !isStr, s == "":
			delete(data, k)
		case s == CredentialSentinel:
			if old := existing[k]; old != "" {
				if out == nil {
					out = map[string]string{}
				}
				out[k] = old
			} else {
				delete(data, k)
			}
		default:
			if out == nil {
				out = map[string]string{}
			}
			out[k] = s
			data[k] = CredentialSentinel
		}
	}
	return out
}

// innerNodeData returns the data maps of a component definition's inner
// nodes (stored in Data["nodes"] as {id, type, data, ...} objects).
func innerNodeData(data map[string]any) []map[string]any {
	rawNodes, _ := data["nodes"].([]any)
	var out []map[string]any
	for _, raw := range rawNodes {
		m, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		inner, _ := m["data"].(map[string]any)
		if inner == nil {
			continue
		}
		out = append(out, m)
	}
	return out
}

// ExtractWorkflowCredentials moves credential values out of every node
// in the workflow into a nodeID → key → value map, leaving sentinel
// markers in node.Data. existing is the previously stored credential
// map (nil on create) used to preserve values the editor sent back as
// sentinels or omitted entirely. Persist the result via
// CredentialReplaceForWorkflow.
func ExtractWorkflowCredentials(w WorkflowInterface, existing map[string]map[string]string) map[string]map[string]string {
	var creds map[string]map[string]string
	put := func(nodeID string, c map[string]string) {
		if c == nil {
			return
		}
		if creds == nil {
			creds = map[string]map[string]string{}
		}
		creds[nodeID] = c
	}
	for _, n := range w.GetNodes() {
		data := n.GetData()
		if data == nil {
			continue
		}
		put(n.GetID(), extractCredentialFields(n.GetType(), data, existing[n.GetID()]))
		// Component definitions carry inner nodes inside Data["nodes"];
		// their credentials live in the same store keyed by inner id.
		if n.GetType() == componentDefNodeType {
			for _, m := range innerNodeData(data) {
				id, _ := m["id"].(string)
				typ, _ := m["type"].(string)
				inner, _ := m["data"].(map[string]any)
				if id == "" {
					continue
				}
				put(id, extractCredentialFields(typ, inner, existing[id]))
			}
		}
	}
	return creds
}

// applyCredentialFields writes stored credential values back into a
// node-data map and drops sentinel markers that have no stored value.
func applyCredentialFields(nodeType string, data map[string]any, stored map[string]string) {
	for _, k := range CredentialKeys(nodeType) {
		if v := stored[k]; v != "" {
			data[k] = v
			continue
		}
		if IsCredentialSentinel(data[k]) {
			delete(data, k)
		}
	}
}

// DeepCloneWorkflow returns a workflow carrying fresh node objects
// with deep-copied data maps (JSON round-trip — node data is always
// JSON-typed). The deploy path clones before credential injection so
// real secrets never land in the caller's stored document — the
// engine's own cloneWorkflow is shallow (nodes shared).
func DeepCloneWorkflow(w WorkflowInterface) WorkflowInterface {
	c := NewWorkflow(w.GetName()).
		SetID(w.GetID()).
		SetDescription(w.GetDescription()).
		SetDeployed(w.GetDeployed()).
		SetPaused(w.GetPaused()).
		SetCreatedAt(w.GetCreatedAt()).
		SetUpdatedAt(w.GetUpdatedAt())
	c.SetView(w.GetView())
	for _, n := range w.GetNodes() {
		data := map[string]any{}
		if raw, err := json.Marshal(n.GetData()); err == nil {
			_ = json.Unmarshal(raw, &data)
		}
		c.AddNode(NewNode(n.GetID(), n.GetType()).
			SetPosition(n.GetPositionX(), n.GetPositionY()).
			SetGroupID(n.GetGroupID()).
			SetData(data))
	}
	for _, e := range w.GetEdges() {
		c.AddEdge(NewEdge(e.GetID(), e.GetSource(), e.GetTarget()).
			SetWorkflowID(e.GetWorkflowID()).
			SetSourceKey(e.GetSourceKey()).
			SetTargetKey(e.GetTargetKey()))
	}
	return c
}

// ApplyWorkflowCredentials merges stored credentials into node.Data so
// a deploy hands executors the real values while storage keeps only
// sentinel markers. Mutates the workflow in place — callers deploy a
// clone (the engine deploy path clones before expansion, so stored
// entities are never tainted).
func ApplyWorkflowCredentials(w WorkflowInterface, creds map[string]map[string]string) {
	for _, n := range w.GetNodes() {
		data := n.GetData()
		if data == nil {
			continue
		}
		applyCredentialFields(n.GetType(), data, creds[n.GetID()])
		if n.GetType() == componentDefNodeType {
			for _, m := range innerNodeData(data) {
				id, _ := m["id"].(string)
				typ, _ := m["type"].(string)
				inner, _ := m["data"].(map[string]any)
				applyCredentialFields(typ, inner, creds[id])
			}
		}
	}
}
