package nodestore

import (
	"encoding/json"
	"fmt"
	"log"
	"strings"
)

// This file implements child flow-level export/import: one child flow (flow or
// role:"component" template) moves between workflows as a portable
// envelope. Format decisions:
//
//   - Payload: a versioned child flow envelope, not a Node-RED
//     array. The envelope keeps everything a Node-RED tab would lose:
//     edge ids, view.groups, child flow meta (role/enabled/env) and the
//     component definitions the child flow's instances need. Node-RED
//     interop for single child flows stays available through the
//     whole-workflow export (one tab per child flow); pasting a Node-RED
//     array still creates a new workflow via ImportNodeRED.
//   - Cross-child flow references: nodes keep their link/linkCall/catch
//     references verbatim (an import back into the source workflow
//     still resolves them) and every unresolved reference is recorded
//     in Warnings — keep + warn, the Node-RED "copy as JSON" policy
//     made explicit instead of silent. Component definitions are the
//     exception: they are exported transitively (defs[]), because an
//     instance without its definition cannot deploy at all.
//   - Import semantics: the payload always lands as a NEW child flow —
//     every id (child flow, nodes, edges, groups, defs and their internal
//     nodes/edges) is re-minted and every intra-payload reference is
//     rewritten, so re-importing produces a second copy rather than
//     clobbering the original. Label collisions get an " (imported)"
//     suffix, then a numeric one.

// childFlowExportFormat is the "nodestore" field's marker value.
const childFlowExportFormat = "child_flow"

// childFlowExportVersion is the current envelope version. Importers
// accept any version <= this so the format can evolve.
const childFlowExportVersion = 1

// ChildFlowExport is the portable single-child flow payload.
type ChildFlowExport struct {
	// Format identifies the envelope kind; always "child_flow".
	Format string `json:"nodestore"`
	// Version is the envelope schema version.
	Version int `json:"version"`
	// ChildFlow carries the meta entity {id,label,role,enabled,env}.
	ChildFlow map[string]any `json:"child_flow"`
	// Nodes are the child flow's nodes (backend shape; data.child flow is
	// implied by the envelope and omitted).
	Nodes []NodeInterface `json:"nodes"`
	// Edges are the edges whose endpoints both live in the child flow.
	Edges []EdgeInterface `json:"edges"`
	// Groups are the view.groups entries belonging to the child flow.
	Groups []any `json:"groups,omitempty"`
	// Defs are the type:"component" definition nodes the child flow's
	// component:<id> instances reference, resolved transitively
	// (a definition may itself instantiate another definition).
	Defs []NodeInterface `json:"defs,omitempty"`
	// Warnings lists references that point outside the payload —
	// kept verbatim in the export, so they only resolve when the
	// target workflow already contains the referenced nodes.
	Warnings []string `json:"warnings,omitempty"`
}

// childFlowExportJSON is the wire form of ChildFlowExport — identical field
// layout, but with the concrete nodeJSON/edgeJSON shapes so the
// interface-typed slices marshal and unmarshal faithfully.
type childFlowExportJSON struct {
	Format    string         `json:"nodestore"`
	Version   int            `json:"version"`
	ChildFlow map[string]any `json:"child_flow"`
	Nodes     []nodeJSON     `json:"nodes"`
	Edges     []edgeJSON     `json:"edges"`
	Groups    []any          `json:"groups,omitempty"`
	Defs      []nodeJSON     `json:"defs,omitempty"`
	Warnings  []string       `json:"warnings,omitempty"`
}

// MarshalJSON serializes the interface-typed node/edge slices through
// their persisted JSON shapes.
func (e ChildFlowExport) MarshalJSON() ([]byte, error) {
	wire := childFlowExportJSON{
		Format:    e.Format,
		Version:   e.Version,
		ChildFlow: e.ChildFlow,
		Nodes:     []nodeJSON{},
		Edges:     []edgeJSON{},
		Groups:    e.Groups,
		Warnings:  e.Warnings,
	}
	for _, n := range e.Nodes {
		wire.Nodes = append(wire.Nodes, nodeJSON{
			ID:       n.GetID(),
			Type:     n.GetType(),
			Position: positionJSON{X: n.GetPositionX(), Y: n.GetPositionY()},
			Data:     n.GetData(),
		})
	}
	for _, e2 := range e.Edges {
		wire.Edges = append(wire.Edges, edgeJSON{
			ID:        e2.GetID(),
			Source:    e2.GetSource(),
			SourceKey: e2.GetSourceKey(),
			Target:    e2.GetTarget(),
			TargetKey: e2.GetTargetKey(),
		})
	}
	for _, d := range e.Defs {
		wire.Defs = append(wire.Defs, nodeJSON{
			ID:       d.GetID(),
			Type:     d.GetType(),
			Position: positionJSON{X: d.GetPositionX(), Y: d.GetPositionY()},
			Data:     d.GetData(),
		})
	}
	return json.Marshal(wire)
}

// UnmarshalJSON decodes the wire form back into interface-typed
// domain objects.
func (e *ChildFlowExport) UnmarshalJSON(raw []byte) error {
	var wire childFlowExportJSON
	if err := json.Unmarshal(raw, &wire); err != nil {
		return err
	}

	// Legacy v1 envelopes used nodestore:"module" and a top-level
	// "module" meta object. Treat them as migration input and normalize
	// them into the canonical child_flow shape before import.
	if wire.Format == "module" {
		wire.Format = childFlowExportFormat
	}
	if wire.ChildFlow == nil {
		var legacy struct {
			Module map[string]any `json:"module"`
		}
		if err := json.Unmarshal(raw, &legacy); err != nil {
			return err
		}
		wire.ChildFlow = legacy.Module
	}
	if wire.ChildFlow != nil && wire.ChildFlow["role"] == "subflow" {
		wire.ChildFlow["role"] = "component"
	}
	if len(wire.Groups) > 0 {
		legacyView := map[string]any{"groups": wire.Groups}
		normalizeLegacyViewTerminology(legacyView)
		wire.Groups, _ = legacyView["groups"].([]any)
	}

	e.Format = wire.Format
	e.Version = wire.Version
	e.ChildFlow = wire.ChildFlow
	e.Groups = wire.Groups
	e.Warnings = wire.Warnings
	e.Nodes = []NodeInterface{}
	for _, jn := range wire.Nodes {
		e.Nodes = append(e.Nodes, nodeFromJSON(jn))
	}
	e.Edges = []EdgeInterface{}
	for _, je := range wire.Edges {
		e.Edges = append(e.Edges, edgeFromJSON(je))
	}
	e.Defs = []NodeInterface{}
	for _, jd := range wire.Defs {
		e.Defs = append(e.Defs, nodeFromJSON(jd))
	}
	return nil
}

// Child flow identity helpers.
//
// Under the id-keyed model a node's Data["childFlow"] holds the stable
// child flow id and the child flow entities are persisted as
// workflow view["childFlows"] entries of shape {id, label, role,
// enabled, env}. Documents written before the migration carry the
// child flow name in Data["childFlow"] instead — and in
// view.disabledFlows / view.groups[].childFlow — so every consumer
// resolves references through the entity list, accepting either form.

// flowLabel resolves a child flow reference — a child flow id or a legacy
// child flow name — to the child flow's display label via view["childFlows"].
// Unresolvable references return unchanged, which keeps legacy
// name-valued documents working: their data.child flow already is the
// label.
func flowLabel(w WorkflowInterface, ref string) string {
	if w == nil || w.GetView() == nil || ref == "" {
		return ref
	}
	for _, m := range arrayOf(w.GetView()["childFlows"]) {
		mm, ok := m.(map[string]any)
		if !ok {
			continue
		}
		if stringOf(mm["id"]) == ref {
			if label := stringOf(mm["label"]); label != "" {
				return label
			}
		}
	}
	return ref
}

// flowRefMatches reports whether two stored child flow references point
// at the same child flow. Each reference may be a child flow id or a name;
// both are resolved to labels through view.child flows so mixed old/new
// documents (id on the node, name on the flag, or vice versa) still
// match.
func flowRefMatches(w WorkflowInterface, a, b any) bool {
	ra, rb := stringOf(a), stringOf(b)
	if ra == "" || rb == "" {
		return false
	}
	return ra == rb || flowLabel(w, ra) == flowLabel(w, rb)
}

// flowEntity finds the view.child flows entity matching ref by id or
// label; returns nil when the workflow carries no such entity.
func flowEntity(w WorkflowInterface, ref string) map[string]any {
	if w == nil || w.GetView() == nil || ref == "" {
		return nil
	}
	for _, m := range arrayOf(w.GetView()["childFlows"]) {
		mm, ok := m.(map[string]any)
		if !ok {
			continue
		}
		if stringOf(mm["id"]) == ref || stringOf(mm["label"]) == ref {
			return mm
		}
	}
	return nil
}

// nodeRefKeys are the node Data keys that carry references to other
// node ids: linkOut "links", catch/status/complete "nodes" lists,
// linkCall "link", mqtt "broker" (address values that are not node
// ids pass through unchanged), tcp/mqtt "tls" (tlsConfig refs) and
// websocket in/out "server"/"client" (websocketListener /
// websocketClient config refs).
var nodeRefKeys = []string{"links", "nodes", "link", "broker", "tls", "server", "client"}

// collectIDRefs extracts the node ids a node's Data references through
// nodeRefKeys (ids may be a string, a comma-separated string, or an
// array).
func collectIDRefs(data map[string]any) []string {
	var out []string
	for _, key := range nodeRefKeys {
		raw, ok := data[key]
		if !ok {
			continue
		}
		switch v := raw.(type) {
		case string:
			for _, p := range strings.Split(v, ",") {
				if s := strings.TrimSpace(p); s != "" {
					out = append(out, s)
				}
			}
		case []any:
			for _, item := range v {
				if s, ok := item.(string); ok && s != "" {
					out = append(out, s)
				}
			}
		case []string:
			out = append(out, v...)
		}
	}
	return out
}

// scrubNodeData returns a deep copy of a node's Data safe for export:
// child flow bookkeeping and credential material are removed (same policy
// as ExportNodeRED — secrets never leave the server in exports).
func scrubNodeData(nodeType string, data map[string]any) map[string]any {
	out := map[string]any{}
	for k, v := range data {
		if k == "childFlow" || k == "childFlowName" || k == componentEnvKey ||
			IsCredentialKey(nodeType, k) || IsCredentialSentinel(v) {
			continue
		}
		out[k] = v
	}
	return out
}

// scrubDefData deep-copies a type:"component" definition's Data and
// scrubs credential material inside its internal nodes.
func scrubDefData(data map[string]any) map[string]any {
	raw, _ := json.Marshal(data)
	var out map[string]any
	_ = json.Unmarshal(raw, &out)
	if out == nil {
		out = map[string]any{}
	}
	for _, rawNode := range arrayOf(out["nodes"]) {
		m, ok := rawNode.(map[string]any)
		if !ok {
			continue
		}
		typ := stringOf(m["type"])
		if nd, ok := m["data"].(map[string]any); ok {
			m["data"] = scrubNodeData(typ, nd)
		}
	}
	return out
}

// collectDefIDs returns the closure of component definition ids the
// given nodes reference through "component:<defID>" types — including
// definitions instantiated inside other definitions' internal graphs.
func collectDefIDs(w WorkflowInterface, nodes []NodeInterface) map[string]bool {
	defIndex := map[string]bool{}
	for _, n := range w.GetNodes() {
		if n.GetType() == componentDefNodeType {
			defIndex[n.GetID()] = true
		}
	}
	wanted := map[string]bool{}
	var queue []string
	note := func(nodeType string) {
		if !IsComponentInstance(nodeType) {
			return
		}
		id := strings.TrimPrefix(nodeType, componentTypePrefix)
		if id != "" && !wanted[id] {
			wanted[id] = true
			queue = append(queue, id)
		}
	}
	for _, n := range nodes {
		note(n.GetType())
	}
	for len(queue) > 0 {
		id := queue[0]
		queue = queue[1:]
		if !defIndex[id] {
			continue // instance references a missing definition
		}
		for _, n := range w.GetNodes() {
			if n.GetType() == componentDefNodeType && n.GetID() == id {
				for _, rawInner := range arrayOf(n.GetData()["nodes"]) {
					if m, ok := rawInner.(map[string]any); ok {
						note(stringOf(m["type"]))
					}
				}
			}
		}
	}
	return wanted
}

// ExportChildFlow builds the portable envelope for one child flow of w.
// flowRef is a child flow id or label. Nodes are the workflow nodes
// whose data.child flow resolves to the child flow; edges are those fully
// inside the set; groups and component definitions follow. References
// pointing at nodes outside the child flow are kept verbatim and listed
// in Warnings.
func ExportChildFlow(w WorkflowInterface, flowRef string) (*ChildFlowExport, error) {
	if w == nil {
		return nil, fmt.Errorf("workflow is nil")
	}
	if flowRef == "" {
		return nil, fmt.Errorf("child flow is required")
	}

	var nodes []NodeInterface
	inFlow := map[string]bool{}
	for _, n := range w.GetNodes() {
		if n.GetType() == componentDefNodeType {
			continue
		}
		if flowRefMatches(w, n.GetData()["childFlow"], flowRef) {
			nodes = append(nodes, n)
			inFlow[n.GetID()] = true
		}
	}
	entity := flowEntity(w, flowRef)
	if len(nodes) == 0 && entity == nil {
		return nil, fmt.Errorf("child flow %q not found", flowRef)
	}

	// Child flow meta: prefer the persisted entity, else synthesize from
	// the resolved label so a legacy name-keyed child flow still exports.
	meta := map[string]any{}
	if entity != nil {
		for k, v := range entity {
			meta[k] = v
		}
	}
	if stringOf(meta["id"]) == "" {
		meta["id"] = flowRef
	}
	label := stringOf(meta["label"])
	if label == "" {
		label = flowLabel(w, flowRef)
	}
	meta["label"] = label
	if stringOf(meta["role"]) == "" {
		meta["role"] = "flow"
	}
	if _, has := meta["enabled"]; !has {
		// view.disabledFlows covers child flows whose entity lacks the
		// enabled flag (legacy documents).
		disabled := false
		for _, m := range arrayOf(w.GetView()["disabledFlows"]) {
			if flowRefMatches(w, m, flowRef) {
				disabled = true
			}
		}
		meta["enabled"] = !disabled
	}

	out := &ChildFlowExport{
		Format:    childFlowExportFormat,
		Version:   childFlowExportVersion,
		ChildFlow: meta,
		Nodes:     []NodeInterface{},
		Edges:     []EdgeInterface{},
	}

	for _, n := range nodes {
		out.Nodes = append(out.Nodes, NewNode(n.GetID(), n.GetType()).
			SetPosition(n.GetPositionX(), n.GetPositionY()).
			SetData(scrubNodeData(n.GetType(), n.GetData())))
	}

	// Edges: keep only intra-child flow wires; a wire crossing the child flow
	// boundary is dropped and reported (Node-RED precedent — selected
	// export silently drops out-of-selection wires — but we say so).
	for _, e := range w.GetEdges() {
		switch {
		case inFlow[e.GetSource()] && inFlow[e.GetTarget()]:
			out.Edges = append(out.Edges, e)
		case inFlow[e.GetSource()] || inFlow[e.GetTarget()]:
			out.Warnings = append(out.Warnings, fmt.Sprintf(
				"edge %q (%s -> %s) crosses the child flow boundary and was dropped",
				e.GetID(), e.GetSource(), e.GetTarget()))
		}
	}

	// Groups owned by the child flow (child flow field stripped — the envelope
	// implies it).
	for _, g := range arrayOf(w.GetView()["groups"]) {
		gm, ok := g.(map[string]any)
		if !ok {
			continue
		}
		if !flowRefMatches(w, gm["childFlow"], flowRef) {
			continue
		}
		entry := map[string]any{}
		for k, v := range gm {
			if k == "childFlow" {
				continue
			}
			entry[k] = v
		}
		out.Groups = append(out.Groups, entry)
	}

	// Component definitions the child flow's instances need, transitively.
	wanted := collectDefIDs(w, nodes)
	for _, n := range w.GetNodes() {
		if n.GetType() == componentDefNodeType && wanted[n.GetID()] {
			out.Defs = append(out.Defs, NewNode(n.GetID(), componentDefNodeType).
				SetPosition(n.GetPositionX(), n.GetPositionY()).
				SetData(scrubDefData(n.GetData())))
		}
	}

	out.Warnings = append(out.Warnings, externalRefWarnings(out.Nodes, inFlow)...)
	return out, nil
}

// ExportComponentDef builds a child flow envelope carrying a single component
// definition (plus any definitions it nests). The envelope's child flow
// entity is a role:"component" template named after the definition, so
// importing it produces a navigable child flow plus a working definition.
func ExportComponentDef(w WorkflowInterface, defID string) (*ChildFlowExport, error) {
	if defID == "" {
		return nil, fmt.Errorf("component definition id is required")
	}
	var def NodeInterface
	for _, n := range w.GetNodes() {
		if n.GetType() == componentDefNodeType && n.GetID() == defID {
			def = n
			break
		}
	}
	if def == nil {
		return nil, fmt.Errorf("component definition %q not found", defID)
	}

	out := &ChildFlowExport{
		Format:  childFlowExportFormat,
		Version: childFlowExportVersion,
		ChildFlow: map[string]any{
			"id":      def.GetID(),
			"label":   stringOf(def.GetData()["name"]),
			"role":    "component",
			"enabled": true,
		},
		Nodes: []NodeInterface{},
		Edges: []EdgeInterface{},
	}
	if out.ChildFlow["label"] == "" {
		out.ChildFlow["label"] = "Component"
	}

	// The definition plus the transitive closure of definitions its
	// internal nodes instantiate.
	seed := NewNode(def.GetID(), componentTypePrefix+def.GetID())
	wanted := collectDefIDs(w, []NodeInterface{seed})
	for _, n := range w.GetNodes() {
		if n.GetType() == componentDefNodeType && wanted[n.GetID()] {
			out.Defs = append(out.Defs, NewNode(n.GetID(), componentDefNodeType).
				SetPosition(n.GetPositionX(), n.GetPositionY()).
				SetData(scrubDefData(n.GetData())))
		}
	}
	return out, nil
}

// externalRefWarnings lists node Data references pointing outside the
// exported node set.
func externalRefWarnings(nodes []NodeInterface, inFlow map[string]bool) []string {
	var warnings []string
	for _, n := range nodes {
		for _, ref := range collectIDRefs(n.GetData()) {
			if !inFlow[ref] {
				warnings = append(warnings, fmt.Sprintf(
					"node %q (%s) references external node %q — the reference is kept but will dangle unless the target workflow contains it",
					n.GetID(), n.GetType(), ref))
			}
		}
	}
	return warnings
}

// dedupeFlowLabel returns label, or a suffixed variant when a child flow
// entity already carries that label.
func dedupeFlowLabel(w WorkflowInterface, label string) string {
	taken := map[string]bool{}
	for _, m := range arrayOf(w.GetView()["childFlows"]) {
		if mm, ok := m.(map[string]any); ok {
			taken[stringOf(mm["label"])] = true
		}
	}
	if !taken[label] {
		return label
	}
	candidate := label + " (imported)"
	if !taken[candidate] {
		return candidate
	}
	for i := 2; ; i++ {
		candidate = fmt.Sprintf("%s (imported %d)", label, i)
		if !taken[candidate] {
			return candidate
		}
	}
}

// remapIDRefs rewrites a node-id reference field (string, comma
// separated string, or array) through idMap; unmapped ids pass
// through untouched (the keep-dangling policy).
func remapIDRefs(v any, idMap map[string]string) any {
	switch t := v.(type) {
	case string:
		parts := strings.Split(t, ",")
		for i, p := range parts {
			if mapped, hit := idMap[strings.TrimSpace(p)]; hit {
				parts[i] = mapped
			}
		}
		return strings.Join(parts, ",")
	case []any:
		for i, item := range t {
			if s, ok := item.(string); ok {
				if mapped, hit := idMap[s]; hit {
					t[i] = mapped
				}
			}
		}
		return t
	case []string:
		for i, s := range t {
			if mapped, hit := idMap[s]; hit {
				t[i] = mapped
			}
		}
		return t
	}
	return v
}

// rewriteDefData remaps a definition's internal graph: every internal
// node gets a new id (innerIDs), internal edges and in/out port wires
// follow, inner node-id refs are rewritten, and nested
// "component:<defID>" instances follow the definition id map.
func rewriteDefData(data map[string]any, innerIDs, defIDs map[string]string) map[string]any {
	out := map[string]any{}
	for k, v := range data {
		out[k] = v
	}

	var innerNodes []any
	for _, rawNode := range arrayOf(data["nodes"]) {
		m, ok := rawNode.(map[string]any)
		if !ok {
			continue
		}
		if mapped, hit := innerIDs[stringOf(m["id"])]; hit {
			m["id"] = mapped
		}
		if typ := stringOf(m["type"]); IsComponentInstance(typ) {
			old := strings.TrimPrefix(typ, componentTypePrefix)
			if mapped, hit := defIDs[old]; hit {
				m["type"] = componentTypePrefix + mapped
			}
		}
		if nd, ok := m["data"].(map[string]any); ok {
			for _, key := range nodeRefKeys {
				if rv, ok := nd[key]; ok {
					nd[key] = remapIDRefs(rv, innerIDs)
				}
			}
		}
		innerNodes = append(innerNodes, m)
	}
	out["nodes"] = innerNodes

	var innerEdges []any
	for _, rawEdge := range arrayOf(data["edges"]) {
		m, ok := rawEdge.(map[string]any)
		if !ok {
			continue
		}
		m["id"] = generateID()
		if mapped, hit := innerIDs[stringOf(m["source"])]; hit {
			m["source"] = mapped
		}
		if mapped, hit := innerIDs[stringOf(m["target"])]; hit {
			m["target"] = mapped
		}
		innerEdges = append(innerEdges, m)
	}
	out["edges"] = innerEdges

	// in/out port declarations: wires[].id references internal nodes.
	for _, portKey := range []string{"in", "out"} {
		var ports []any
		for _, rawPort := range arrayOf(data[portKey]) {
			pm, ok := rawPort.(map[string]any)
			if !ok {
				continue
			}
			var wires []any
			for _, rawWire := range arrayOf(pm["wires"]) {
				wm, ok := rawWire.(map[string]any)
				if !ok {
					continue
				}
				if mapped, hit := innerIDs[stringOf(wm["id"])]; hit {
					wm["id"] = mapped
				}
				wires = append(wires, wm)
			}
			pm["wires"] = wires
			ports = append(ports, pm)
		}
		if ports != nil {
			out[portKey] = ports
		}
	}
	return out
}

// ImportChildFlow merges a child flow envelope into w as a NEW child flow. All
// ids are re-minted (child flow, nodes, edges, groups, definitions and
// their internal graphs) and every intra-payload reference is
// rewritten; references pointing outside the payload are kept verbatim
// and reported in the returned warnings when they do not resolve
// against the target workflow. Returns the list of warnings.
func ImportChildFlow(w WorkflowInterface, raw []byte) ([]string, error) {
	var payload ChildFlowExport
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, fmt.Errorf("invalid child flow export JSON: %w", err)
	}
	if payload.Format != childFlowExportFormat {
		return nil, fmt.Errorf("not a child flow export (nodestore: %q)", payload.Format)
	}
	if payload.Version > childFlowExportVersion {
		return nil, fmt.Errorf("child flow export version %d is newer than supported %d", payload.Version, childFlowExportVersion)
	}
	view := w.GetView()
	if view == nil {
		view = map[string]any{}
		w.SetView(view)
	}

	warnings := append([]string{}, payload.Warnings...)
	newFlowID := generateID()
	label := dedupeFlowLabel(w, stringOf(payload.ChildFlow["label"]))
	if label == "" {
		label = dedupeFlowLabel(w, "Imported child flow")
	}
	role := stringOf(payload.ChildFlow["role"])
	if role == "" {
		role = "flow"
	}
	enabled := true
	if en, ok := payload.ChildFlow["enabled"].(bool); ok {
		enabled = en
	}

	// Definitions first: instance types reference their new ids.
	defIDs := map[string]string{}
	for _, d := range payload.Defs {
		defIDs[d.GetID()] = generateID()
	}
	for _, d := range payload.Defs {
		innerIDs := map[string]string{}
		for _, rawNode := range arrayOf(d.GetData()["nodes"]) {
			if m, ok := rawNode.(map[string]any); ok {
				if id := stringOf(m["id"]); id != "" {
					innerIDs[id] = generateID()
				}
			}
		}
		w.AddNode(NewNode(defIDs[d.GetID()], componentDefNodeType).
			SetPosition(d.GetPositionX(), d.GetPositionY()).
			SetData(rewriteDefData(d.GetData(), innerIDs, defIDs)))
	}

	nodeIDs := map[string]string{}
	for _, n := range payload.Nodes {
		nodeIDs[n.GetID()] = generateID()
	}
	existing := map[string]bool{}
	for _, n := range w.GetNodes() {
		existing[n.GetID()] = true
	}

	for _, n := range payload.Nodes {
		data := cloneDataMap(n.GetData())
		for _, key := range nodeRefKeys {
			if rv, ok := data[key]; ok {
				data[key] = remapIDRefs(rv, nodeIDs)
			}
		}
		typ := n.GetType()
		if IsComponentInstance(typ) {
			old := strings.TrimPrefix(typ, componentTypePrefix)
			if mapped, hit := defIDs[old]; hit {
				typ = componentTypePrefix + mapped
			} else {
				warnings = append(warnings, fmt.Sprintf(
					"component instance %q references definition %q, which is not in the payload — it will fail to deploy",
					n.GetID(), old))
			}
		}
		data["childFlow"] = newFlowID
		data["childFlowName"] = label
		w.AddNode(NewNode(nodeIDs[n.GetID()], typ).
			SetPosition(n.GetPositionX(), n.GetPositionY()).
			SetData(data))
		// Report references that resolve neither inside the payload
		// nor against the target workflow — the dangling half of the
		// keep-and-warn policy. Checked on the original (pre-remap)
		// ids: nodeIDs keys are the payload's old ids.
		for _, ref := range collectIDRefs(n.GetData()) {
			if _, internal := nodeIDs[ref]; !internal && !existing[ref] {
				warnings = append(warnings, fmt.Sprintf(
					"node %q (%s) references node %q, which exists neither in the payload nor in this workflow",
					nodeIDs[n.GetID()], typ, ref))
			}
		}
	}

	for _, e := range payload.Edges {
		src, okS := nodeIDs[e.GetSource()]
		dst, okT := nodeIDs[e.GetTarget()]
		if !okS || !okT {
			warnings = append(warnings, fmt.Sprintf(
				"edge %q dropped: endpoint outside the exported node set (%s -> %s)",
				e.GetID(), e.GetSource(), e.GetTarget()))
			continue
		}
		w.AddEdge(NewEdge(generateID(), src, dst).
			SetSourceKey(e.GetSourceKey()).
			SetTargetKey(e.GetTargetKey()))
	}

	// Groups: new ids, child flow field rebound, member ids remapped.
	var groups []any
	for _, g := range payload.Groups {
		gm, ok := g.(map[string]any)
		if !ok {
			continue
		}
		entry := map[string]any{}
		for k, v := range gm {
			entry[k] = v
		}
		entry["id"] = generateID()
		entry["childFlow"] = newFlowID
		var memberIDs []any
		for _, m := range arrayOf(gm["nodeIds"]) {
			if mapped, hit := nodeIDs[stringOf(m)]; hit {
				memberIDs = append(memberIDs, mapped)
			}
		}
		if len(memberIDs) == 0 {
			continue // a group with no surviving members is not imported
		}
		entry["nodeIds"] = memberIDs
		groups = append(groups, entry)
	}
	if len(groups) > 0 {
		view["groups"] = append(arrayOf(view["groups"]), groups...)
	}

	// Register the child flow entity; a disabled import also lands in
	// view.disabledFlows so the engine deploys its nodes inert.
	entity := map[string]any{
		"id":      newFlowID,
		"label":   label,
		"role":    role,
		"enabled": enabled,
	}
	if env := payload.ChildFlow["env"]; env != nil {
		entity["env"] = env
	}
	view["childFlows"] = append(arrayOf(view["childFlows"]), entity)
	if !enabled {
		view["disabledFlows"] = append(arrayOf(view["disabledFlows"]), newFlowID)
	}

	log.Printf("ImportChildFlow: imported child flow %q (%d nodes, %d edges, %d defs) into workflow %q",
		label, len(payload.Nodes), len(payload.Edges), len(payload.Defs), w.GetID())
	return warnings, nil
}
