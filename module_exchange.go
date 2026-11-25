package nodestore

import (
	"encoding/json"
	"fmt"
	"log"
	"strings"
)

// This file implements module-level export/import: one module (flow or
// role:"subflow" template) moves between workflows as a portable
// envelope. Format decisions:
//
//   - Payload: a versioned module envelope, not a Node-RED
//     array. The envelope keeps everything a Node-RED tab would lose:
//     edge ids, view.groups, module meta (role/enabled/env) and the
//     subflow definitions the module's instances need. Node-RED
//     interop for single modules stays available through the
//     whole-workflow export (one tab per module); pasting a Node-RED
//     array still creates a new workflow via ImportNodeRED.
//   - Cross-module references: nodes keep their link/linkCall/catch
//     references verbatim (an import back into the source workflow
//     still resolves them) and every unresolved reference is recorded
//     in Warnings — keep + warn, the Node-RED "copy as JSON" policy
//     made explicit instead of silent. Subflow definitions are the
//     exception: they are exported transitively (defs[]), because an
//     instance without its definition cannot deploy at all.
//   - Import semantics: the payload always lands as a NEW module —
//     every id (module, nodes, edges, groups, defs and their internal
//     nodes/edges) is re-minted and every intra-payload reference is
//     rewritten, so re-importing produces a second copy rather than
//     clobbering the original. Label collisions get an " (imported)"
//     suffix, then a numeric one.

// moduleExportFormat is the "nodestore" field's marker value.
const moduleExportFormat = "module"

// moduleExportVersion is the current envelope version. Importers
// accept any version <= this so the format can evolve.
const moduleExportVersion = 1

// ModuleExport is the portable single-module payload.
type ModuleExport struct {
	// Format identifies the envelope kind; always "module".
	Format string `json:"nodestore"`
	// Version is the envelope schema version.
	Version int `json:"version"`
	// Module carries the meta entity {id,label,role,enabled,env}.
	Module map[string]any `json:"module"`
	// Nodes are the module's nodes (backend shape; data.module is
	// implied by the envelope and omitted).
	Nodes []NodeInterface `json:"nodes"`
	// Edges are the edges whose endpoints both live in the module.
	Edges []EdgeInterface `json:"edges"`
	// Groups are the view.groups entries belonging to the module.
	Groups []any `json:"groups,omitempty"`
	// Defs are the type:"subflow" definition nodes the module's
	// subflow:<id> instances reference, resolved transitively
	// (a definition may itself instantiate another definition).
	Defs []NodeInterface `json:"defs,omitempty"`
	// Warnings lists references that point outside the payload —
	// kept verbatim in the export, so they only resolve when the
	// target workflow already contains the referenced nodes.
	Warnings []string `json:"warnings,omitempty"`
}

// moduleExportJSON is the wire form of ModuleExport — identical field
// layout, but with the concrete nodeJSON/edgeJSON shapes so the
// interface-typed slices marshal and unmarshal faithfully.
type moduleExportJSON struct {
	Format   string         `json:"nodestore"`
	Version  int            `json:"version"`
	Module   map[string]any `json:"module"`
	Nodes    []nodeJSON     `json:"nodes"`
	Edges    []edgeJSON     `json:"edges"`
	Groups   []any          `json:"groups,omitempty"`
	Defs     []nodeJSON     `json:"defs,omitempty"`
	Warnings []string       `json:"warnings,omitempty"`
}

// MarshalJSON serializes the interface-typed node/edge slices through
// their persisted JSON shapes.
func (e ModuleExport) MarshalJSON() ([]byte, error) {
	wire := moduleExportJSON{
		Format:   e.Format,
		Version:  e.Version,
		Module:   e.Module,
		Nodes:    []nodeJSON{},
		Edges:    []edgeJSON{},
		Groups:   e.Groups,
		Warnings: e.Warnings,
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
func (e *ModuleExport) UnmarshalJSON(raw []byte) error {
	var wire moduleExportJSON
	if err := json.Unmarshal(raw, &wire); err != nil {
		return err
	}
	e.Format = wire.Format
	e.Version = wire.Version
	e.Module = wire.Module
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

// Module identity helpers.
//
// Under the id-keyed model a node's Data["module"] holds the stable
// module id and the module entities are persisted as
// workflow view["modules"] entries of shape {id, label, role,
// enabled, env}. Documents written before the migration carry the
// module name in Data["module"] instead — and in
// view.disabledModules / view.groups[].module — so every consumer
// resolves references through the entity list, accepting either form.

// moduleLabel resolves a module reference — a module id or a legacy
// module name — to the module's display label via view["modules"].
// Unresolvable references return unchanged, which keeps legacy
// name-valued documents working: their data.module already is the
// label.
func moduleLabel(w WorkflowInterface, ref string) string {
	if w == nil || w.GetView() == nil || ref == "" {
		return ref
	}
	for _, m := range arrayOf(w.GetView()["modules"]) {
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

// moduleRefMatches reports whether two stored module references point
// at the same module. Each reference may be a module id or a name;
// both are resolved to labels through view.modules so mixed old/new
// documents (id on the node, name on the flag, or vice versa) still
// match.
func moduleRefMatches(w WorkflowInterface, a, b any) bool {
	ra, rb := stringOf(a), stringOf(b)
	if ra == "" || rb == "" {
		return false
	}
	return ra == rb || moduleLabel(w, ra) == moduleLabel(w, rb)
}

// moduleEntity finds the view.modules entity matching ref by id or
// label; returns nil when the workflow carries no such entity.
func moduleEntity(w WorkflowInterface, ref string) map[string]any {
	if w == nil || w.GetView() == nil || ref == "" {
		return nil
	}
	for _, m := range arrayOf(w.GetView()["modules"]) {
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
// node ids: linkOut "links", catch/status/complete "nodes" lists and
// linkCall "link".
var nodeRefKeys = []string{"links", "nodes", "link"}

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
// module bookkeeping and credential material are removed (same policy
// as ExportNodeRED — secrets never leave the server in exports).
func scrubNodeData(nodeType string, data map[string]any) map[string]any {
	out := map[string]any{}
	for k, v := range data {
		if k == "module" || k == "moduleName" || k == subflowEnvKey ||
			IsCredentialKey(nodeType, k) || IsCredentialSentinel(v) {
			continue
		}
		out[k] = v
	}
	return out
}

// scrubDefData deep-copies a type:"subflow" definition's Data and
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

// collectDefIDs returns the closure of subflow definition ids the
// given nodes reference through "subflow:<defID>" types — including
// definitions instantiated inside other definitions' internal graphs.
func collectDefIDs(w WorkflowInterface, nodes []NodeInterface) map[string]bool {
	defIndex := map[string]bool{}
	for _, n := range w.GetNodes() {
		if n.GetType() == subflowDefNodeType {
			defIndex[n.GetID()] = true
		}
	}
	wanted := map[string]bool{}
	var queue []string
	note := func(nodeType string) {
		if !IsSubflowInstance(nodeType) {
			return
		}
		id := strings.TrimPrefix(nodeType, subflowTypePrefix)
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
			if n.GetType() == subflowDefNodeType && n.GetID() == id {
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

// ExportModule builds the portable envelope for one module of w.
// moduleRef is a module id or label. Nodes are the workflow nodes
// whose data.module resolves to the module; edges are those fully
// inside the set; groups and subflow definitions follow. References
// pointing at nodes outside the module are kept verbatim and listed
// in Warnings.
func ExportModule(w WorkflowInterface, moduleRef string) (*ModuleExport, error) {
	if w == nil {
		return nil, fmt.Errorf("workflow is nil")
	}
	if moduleRef == "" {
		return nil, fmt.Errorf("module is required")
	}

	var nodes []NodeInterface
	inModule := map[string]bool{}
	for _, n := range w.GetNodes() {
		if n.GetType() == subflowDefNodeType {
			continue
		}
		if moduleRefMatches(w, n.GetData()["module"], moduleRef) {
			nodes = append(nodes, n)
			inModule[n.GetID()] = true
		}
	}
	entity := moduleEntity(w, moduleRef)
	if len(nodes) == 0 && entity == nil {
		return nil, fmt.Errorf("module %q not found", moduleRef)
	}

	// Module meta: prefer the persisted entity, else synthesize from
	// the resolved label so a legacy name-keyed module still exports.
	meta := map[string]any{}
	if entity != nil {
		for k, v := range entity {
			meta[k] = v
		}
	}
	if stringOf(meta["id"]) == "" {
		meta["id"] = moduleRef
	}
	label := stringOf(meta["label"])
	if label == "" {
		label = moduleLabel(w, moduleRef)
	}
	meta["label"] = label
	if stringOf(meta["role"]) == "" {
		meta["role"] = "flow"
	}
	if _, has := meta["enabled"]; !has {
		// view.disabledModules covers modules whose entity lacks the
		// enabled flag (legacy documents).
		disabled := false
		for _, m := range arrayOf(w.GetView()["disabledModules"]) {
			if moduleRefMatches(w, m, moduleRef) {
				disabled = true
			}
		}
		meta["enabled"] = !disabled
	}

	out := &ModuleExport{
		Format:  moduleExportFormat,
		Version: moduleExportVersion,
		Module:  meta,
		Nodes:   []NodeInterface{},
		Edges:   []EdgeInterface{},
	}

	for _, n := range nodes {
		out.Nodes = append(out.Nodes, NewNode(n.GetID(), n.GetType()).
			SetPosition(n.GetPositionX(), n.GetPositionY()).
			SetData(scrubNodeData(n.GetType(), n.GetData())))
	}

	// Edges: keep only intra-module wires; a wire crossing the module
	// boundary is dropped and reported (Node-RED precedent — selected
	// export silently drops out-of-selection wires — but we say so).
	for _, e := range w.GetEdges() {
		switch {
		case inModule[e.GetSource()] && inModule[e.GetTarget()]:
			out.Edges = append(out.Edges, e)
		case inModule[e.GetSource()] || inModule[e.GetTarget()]:
			out.Warnings = append(out.Warnings, fmt.Sprintf(
				"edge %q (%s -> %s) crosses the module boundary and was dropped",
				e.GetID(), e.GetSource(), e.GetTarget()))
		}
	}

	// Groups owned by the module (module field stripped — the envelope
	// implies it).
	for _, g := range arrayOf(w.GetView()["groups"]) {
		gm, ok := g.(map[string]any)
		if !ok {
			continue
		}
		if !moduleRefMatches(w, gm["module"], moduleRef) {
			continue
		}
		entry := map[string]any{}
		for k, v := range gm {
			if k == "module" {
				continue
			}
			entry[k] = v
		}
		out.Groups = append(out.Groups, entry)
	}

	// Subflow definitions the module's instances need, transitively.
	wanted := collectDefIDs(w, nodes)
	for _, n := range w.GetNodes() {
		if n.GetType() == subflowDefNodeType && wanted[n.GetID()] {
			out.Defs = append(out.Defs, NewNode(n.GetID(), subflowDefNodeType).
				SetPosition(n.GetPositionX(), n.GetPositionY()).
				SetData(scrubDefData(n.GetData())))
		}
	}

	out.Warnings = append(out.Warnings, externalRefWarnings(out.Nodes, inModule)...)
	return out, nil
}

// ExportSubflowDef builds a module envelope carrying a single subflow
// definition (plus any definitions it nests). The envelope's module
// entity is a role:"subflow" template named after the definition, so
// importing it produces a navigable module plus a working definition.
func ExportSubflowDef(w WorkflowInterface, defID string) (*ModuleExport, error) {
	if defID == "" {
		return nil, fmt.Errorf("subflow definition id is required")
	}
	var def NodeInterface
	for _, n := range w.GetNodes() {
		if n.GetType() == subflowDefNodeType && n.GetID() == defID {
			def = n
			break
		}
	}
	if def == nil {
		return nil, fmt.Errorf("subflow definition %q not found", defID)
	}

	out := &ModuleExport{
		Format:  moduleExportFormat,
		Version: moduleExportVersion,
		Module: map[string]any{
			"id":      def.GetID(),
			"label":   stringOf(def.GetData()["name"]),
			"role":    "subflow",
			"enabled": true,
		},
		Nodes: []NodeInterface{},
		Edges: []EdgeInterface{},
	}
	if out.Module["label"] == "" {
		out.Module["label"] = "Subflow"
	}

	// The definition plus the transitive closure of definitions its
	// internal nodes instantiate.
	seed := NewNode(def.GetID(), subflowTypePrefix+def.GetID())
	wanted := collectDefIDs(w, []NodeInterface{seed})
	for _, n := range w.GetNodes() {
		if n.GetType() == subflowDefNodeType && wanted[n.GetID()] {
			out.Defs = append(out.Defs, NewNode(n.GetID(), subflowDefNodeType).
				SetPosition(n.GetPositionX(), n.GetPositionY()).
				SetData(scrubDefData(n.GetData())))
		}
	}
	return out, nil
}

// externalRefWarnings lists node Data references pointing outside the
// exported node set.
func externalRefWarnings(nodes []NodeInterface, inModule map[string]bool) []string {
	var warnings []string
	for _, n := range nodes {
		for _, ref := range collectIDRefs(n.GetData()) {
			if !inModule[ref] {
				warnings = append(warnings, fmt.Sprintf(
					"node %q (%s) references external node %q — the reference is kept but will dangle unless the target workflow contains it",
					n.GetID(), n.GetType(), ref))
			}
		}
	}
	return warnings
}

// dedupeModuleLabel returns label, or a suffixed variant when a module
// entity already carries that label.
func dedupeModuleLabel(w WorkflowInterface, label string) string {
	taken := map[string]bool{}
	for _, m := range arrayOf(w.GetView()["modules"]) {
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
// "subflow:<defID>" instances follow the definition id map.
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
		if typ := stringOf(m["type"]); IsSubflowInstance(typ) {
			old := strings.TrimPrefix(typ, subflowTypePrefix)
			if mapped, hit := defIDs[old]; hit {
				m["type"] = subflowTypePrefix + mapped
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

// ImportModule merges a module envelope into w as a NEW module. All
// ids are re-minted (module, nodes, edges, groups, definitions and
// their internal graphs) and every intra-payload reference is
// rewritten; references pointing outside the payload are kept verbatim
// and reported in the returned warnings when they do not resolve
// against the target workflow. Returns the list of warnings.
func ImportModule(w WorkflowInterface, raw []byte) ([]string, error) {
	var payload ModuleExport
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, fmt.Errorf("invalid module export JSON: %w", err)
	}
	if payload.Format != moduleExportFormat {
		return nil, fmt.Errorf("not a module export (nodestore: %q)", payload.Format)
	}
	if payload.Version > moduleExportVersion {
		return nil, fmt.Errorf("module export version %d is newer than supported %d", payload.Version, moduleExportVersion)
	}
	view := w.GetView()
	if view == nil {
		view = map[string]any{}
		w.SetView(view)
	}

	warnings := append([]string{}, payload.Warnings...)
	newModuleID := generateID()
	label := dedupeModuleLabel(w, stringOf(payload.Module["label"]))
	if label == "" {
		label = dedupeModuleLabel(w, "Imported module")
	}
	role := stringOf(payload.Module["role"])
	if role == "" {
		role = "flow"
	}
	enabled := true
	if en, ok := payload.Module["enabled"].(bool); ok {
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
		w.AddNode(NewNode(defIDs[d.GetID()], subflowDefNodeType).
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
		if IsSubflowInstance(typ) {
			old := strings.TrimPrefix(typ, subflowTypePrefix)
			if mapped, hit := defIDs[old]; hit {
				typ = subflowTypePrefix + mapped
			} else {
				warnings = append(warnings, fmt.Sprintf(
					"subflow instance %q references definition %q, which is not in the payload — it will fail to deploy",
					n.GetID(), old))
			}
		}
		data["module"] = newModuleID
		data["moduleName"] = label
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

	// Groups: new ids, module field rebound, member ids remapped.
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
		entry["module"] = newModuleID
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

	// Register the module entity; a disabled import also lands in
	// view.disabledModules so the engine deploys its nodes inert.
	entity := map[string]any{
		"id":      newModuleID,
		"label":   label,
		"role":    role,
		"enabled": enabled,
	}
	if env := payload.Module["env"]; env != nil {
		entity["env"] = env
	}
	view["modules"] = append(arrayOf(view["modules"]), entity)
	if !enabled {
		view["disabledModules"] = append(arrayOf(view["disabledModules"]), newModuleID)
	}

	log.Printf("ImportModule: imported module %q (%d nodes, %d edges, %d defs) into workflow %q",
		label, len(payload.Nodes), len(payload.Edges), len(payload.Defs), w.GetID())
	return warnings, nil
}
