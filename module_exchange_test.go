package nodestore

import (
	"encoding/json"
	"strings"
	"testing"
)

// requireWellFormedDef asserts that a type:"subflow" definition's
// internal graph is intact: every internal node parses through
// parseSubflowInnerNode and every internal edge decodes with endpoints
// among the internal node ids. It stands in for flowrunner's
// ExpandSubflows check (nodestore has no engine), proving the imported
// definition is deployable in shape.
func requireWellFormedDef(t *testing.T, n NodeInterface) {
	t.Helper()
	data := n.GetData()
	innerIDs := map[string]bool{}
	for _, rawNode := range arrayOf(data["nodes"]) {
		m, ok := rawNode.(map[string]any)
		if !ok {
			t.Fatalf("definition %q carries a non-map internal node: %v", n.GetID(), rawNode)
		}
		inner, err := parseSubflowInnerNode(mustJSON(m))
		if err != nil {
			t.Fatalf("definition %q internal node failed to parse: %v", n.GetID(), err)
		}
		innerIDs[inner.GetID()] = true
	}
	if len(innerIDs) == 0 {
		t.Fatalf("definition %q carries no internal nodes", n.GetID())
	}
	for _, rawEdge := range arrayOf(data["edges"]) {
		var je edgeJSON
		if err := json.Unmarshal(mustJSON(rawEdge), &je); err != nil {
			t.Fatalf("definition %q internal edge failed to decode: %v", n.GetID(), err)
		}
		if !innerIDs[je.Source] || !innerIDs[je.Target] {
			t.Fatalf("definition %q internal edge %q references unknown endpoint (%s -> %s)",
				n.GetID(), je.ID, je.Source, je.Target)
		}
	}
}

// findDef returns the first type:"subflow" definition node of w.
func findDef(w WorkflowInterface, id string) NodeInterface {
	for _, n := range w.GetNodes() {
		if n.GetType() == subflowDefNodeType && (id == "" || n.GetID() == id) {
			return n
		}
	}
	return nil
}

// Module export/import round-trip: a module exported from workflow A
// and imported into workflow B must arrive with every id re-minted,
// internal references rewritten, and its subflow definitions intact
// enough that a deploy-time expansion could instantiate them.
func TestModuleExportImportRoundTrip(t *testing.T) {
	src := NewWorkflow("source").SetID("wf-src")
	src.SetNodes([]NodeInterface{
		NewNode("def1", "subflow").SetData(map[string]any{
			"name": "Pass",
			"in":   []any{map[string]any{"x": 0.0, "y": 0.0, "wires": []any{map[string]any{"id": "inner1"}}}},
			"out":  []any{map[string]any{"x": 0.0, "y": 0.0, "wires": []any{map[string]any{"id": "inner1", "port": 0.0}}}},
			"env":  []any{},
			"nodes": []any{map[string]any{
				"id": "inner1", "type": "junction",
				"position": map[string]any{"x": 10.0, "y": 10.0},
				"data":     map[string]any{"name": "j"},
			}},
			"edges": []any{},
		}),
		NewNode("n1", "inject").SetData(map[string]any{"name": "go", "module": "mod-a"}),
		NewNode("n2", "subflow:def1").SetData(map[string]any{"name": "inst", "module": "mod-a"}),
		NewNode("n3", "debug").SetData(map[string]any{"name": "out", "module": "mod-a"}),
		NewNode("n4", "linkOut").SetData(map[string]any{"name": "lo", "module": "mod-a", "links": []any{"ext-1"}}),
		// Other-module + unmoduled nodes must not export.
		NewNode("m1", "debug").SetData(map[string]any{"name": "other", "module": "mod-b"}),
		NewNode("m2", "linkIn").SetData(map[string]any{"name": "li", "module": "mod-b"}),
	})
	src.SetEdges([]EdgeInterface{
		NewEdge("e1", "n1", "n2").SetSourceKey("output_1").SetTargetKey("input_1"),
		NewEdge("e2", "n2", "n3").SetSourceKey("output_1").SetTargetKey("input_1"),
		NewEdge("e3", "n3", "m1").SetSourceKey("output_1").SetTargetKey("input_1"),
	})
	src.SetView(map[string]any{
		"modules": []any{
			map[string]any{"id": "mod-a", "label": "Alpha", "role": "flow", "enabled": true},
			map[string]any{"id": "mod-b", "label": "Beta", "role": "flow", "enabled": true},
		},
		"groups": []any{
			map[string]any{"id": "g1", "module": "mod-a", "name": "grp", "nodeIds": []any{"n1", "n3"}},
		},
	})

	exp, err := ExportModule(src, "mod-a")
	if err != nil {
		t.Fatalf("ExportModule: %v", err)
	}
	if exp.Format != "module" || exp.Version != 1 {
		t.Fatalf("bad envelope header: %+v", exp)
	}
	if got := len(exp.Nodes); got != 4 {
		t.Fatalf("expected 4 exported nodes, got %d", got)
	}
	for _, n := range exp.Nodes {
		if _, ok := n.GetData()["module"]; ok {
			t.Fatalf("exported node %s still carries data.module", n.GetID())
		}
	}
	if got := len(exp.Edges); got != 2 {
		t.Fatalf("expected 2 intra-module edges, got %d", got)
	}
	if got := len(exp.Defs); got != 1 || exp.Defs[0].GetID() != "def1" {
		t.Fatalf("expected def1 exported transitively, got %+v", exp.Defs)
	}
	if len(exp.Groups) != 1 {
		t.Fatalf("expected 1 group, got %d", len(exp.Groups))
	}
	// Boundary edge drop + external linkOut ref must be warned.
	var sawEdge, sawRef bool
	for _, warn := range exp.Warnings {
		if strings.Contains(warn, "e3") {
			sawEdge = true
		}
		if strings.Contains(warn, "ext-1") {
			sawRef = true
		}
	}
	if !sawEdge || !sawRef {
		t.Fatalf("expected boundary-edge and external-ref warnings, got %v", exp.Warnings)
	}

	raw, _ := json.Marshal(exp)

	// Import into a different workflow.
	dst := NewWorkflow("dest").SetID("wf-dst")
	dst.SetNodes([]NodeInterface{
		NewNode("x1", "debug").SetData(map[string]any{"module": "home"}),
	})
	dst.SetView(map[string]any{
		"modules": []any{
			map[string]any{"id": "home", "label": "Alpha", "role": "flow", "enabled": true},
		},
	})
	warnings, err := ImportModule(dst, raw)
	if err != nil {
		t.Fatalf("ImportModule: %v", err)
	}
	// The external linkOut target must be reported dangling.
	var sawDangling bool
	for _, warn := range warnings {
		if strings.Contains(warn, "ext-1") {
			sawDangling = true
		}
	}
	if !sawDangling {
		t.Fatalf("expected dangling-ref warning, got %v", warnings)
	}

	// The module entity must be registered with a fresh id and a
	// deduped label ("Alpha" is taken in the destination workflow).
	var entity map[string]any
	for _, m := range arrayOf(dst.GetView()["modules"]) {
		mm := m.(map[string]any)
		if mm["id"] != "home" {
			entity = mm
		}
	}
	if entity == nil {
		t.Fatal("imported module entity missing from view.modules")
	}
	newID := entity["id"].(string)
	if newID == "mod-a" {
		t.Fatal("module id must be re-minted on import")
	}
	if entity["label"] == "Alpha" {
		t.Fatal("label collision must produce a deduped label")
	}

	// Imported nodes: fresh ids, data.module = new module id.
	var imported []NodeInterface
	for _, n := range dst.GetNodes() {
		if n.GetData()["module"] == newID {
			imported = append(imported, n)
		}
	}
	if len(imported) != 4 {
		t.Fatalf("expected 4 imported nodes, got %d", len(imported))
	}
	oldIDs := map[string]bool{"n1": true, "n2": true, "n3": true, "n4": true}
	var instID string
	for _, n := range imported {
		if oldIDs[n.GetID()] {
			t.Fatalf("node id %s was not re-minted", n.GetID())
		}
		if n.GetData()["moduleName"] != entity["label"] {
			t.Fatalf("moduleName %v does not match deduped label %v", n.GetData()["moduleName"], entity["label"])
		}
		if IsSubflowInstance(n.GetType()) {
			instID = n.GetID()
			newDefID := strings.TrimPrefix(n.GetType(), subflowTypePrefix)
			if newDefID == "def1" {
				t.Fatal("instance type must point at the re-minted definition id")
			}
			if findDef(dst, newDefID) == nil {
				t.Fatalf("instance references missing definition %q", newDefID)
			}
		}
	}
	if instID == "" {
		t.Fatal("subflow instance missing from import")
	}

	// Imported edges: exactly the two intra-module ones, remapped.
	importedIDs := map[string]bool{}
	for _, n := range imported {
		importedIDs[n.GetID()] = true
	}
	var importedEdges int
	for _, e := range dst.GetEdges() {
		if importedIDs[e.GetSource()] && importedIDs[e.GetTarget()] {
			importedEdges++
		}
	}
	if importedEdges != 2 {
		t.Fatalf("expected 2 imported edges, got %d", importedEdges)
	}

	// Group imported with remapped members.
	var groups = arrayOf(dst.GetView()["groups"])
	if len(groups) != 1 {
		t.Fatalf("expected 1 imported group, got %d", len(groups))
	}
	g := groups[0].(map[string]any)
	if g["module"] != newID {
		t.Fatalf("group module not rebound: %v", g["module"])
	}
	for _, m := range arrayOf(g["nodeIds"]) {
		if !importedIDs[stringOf(m)] {
			t.Fatalf("group member %v not remapped", m)
		}
	}

	// The imported definition must be structurally intact — every
	// internal node decodable and every internal edge endpoint valid.
	for _, n := range dst.GetNodes() {
		if n.GetType() == subflowDefNodeType {
			requireWellFormedDef(t, n)
		}
	}
}

// Importing into the SOURCE workflow produces a second copy whose
// external refs still resolve (they pointed at nodes that exist here),
// so no dangling warnings besides the ones carried by the export.
func TestModuleImportIntoSameWorkflow(t *testing.T) {
	src := NewWorkflow("source").SetID("wf-src")
	src.SetNodes([]NodeInterface{
		NewNode("a1", "linkIn").SetData(map[string]any{"module": "mod-x"}),
		NewNode("a2", "linkOut").SetData(map[string]any{"module": "mod-x", "links": "a1"}),
		NewNode("b1", "debug").SetData(map[string]any{"module": "mod-y"}),
	})
	src.SetView(map[string]any{
		"modules": []any{
			map[string]any{"id": "mod-x", "label": "X"},
			map[string]any{"id": "mod-y", "label": "Y"},
		},
	})
	exp, err := ExportModule(src, "mod-x")
	if err != nil {
		t.Fatalf("ExportModule: %v", err)
	}
	if len(exp.Warnings) != 0 {
		t.Fatalf("internal linkOut ref should not warn, got %v", exp.Warnings)
	}
	raw, _ := json.Marshal(exp)
	warnings, err := ImportModule(src, raw)
	if err != nil {
		t.Fatalf("ImportModule: %v", err)
	}
	if len(warnings) != 0 {
		t.Fatalf("re-import into source must not warn, got %v", warnings)
	}
	// The copy's linkOut must point at the COPY's linkIn, not the original.
	var copyOut NodeInterface
	for _, n := range src.GetNodes() {
		if n.GetType() == "linkOut" && n.GetID() != "a2" {
			copyOut = n
		}
	}
	if copyOut == nil {
		t.Fatal("linkOut copy missing")
	}
	target := stringOf(copyOut.GetData()["links"])
	if target == "a1" {
		t.Fatal("copy's linkOut still targets the original linkIn")
	}
	var found bool
	for _, n := range src.GetNodes() {
		if n.GetID() == target && n.GetType() == "linkIn" {
			found = true
		}
	}
	if !found {
		t.Fatalf("copy's linkOut targets %q, not the copied linkIn", target)
	}
}

// A definition-only export (ExportSubflowDef) imports as a working
// definition: its internal graph stays well-formed and an instance can
// be created against the re-minted id.
func TestSubflowDefExportImport(t *testing.T) {
	src := NewWorkflow("").SetID("wf-src")
	src.SetNodes([]NodeInterface{
		NewNode("def9", "subflow").SetData(map[string]any{
			"name": "Echo",
			"in":   []any{map[string]any{"wires": []any{map[string]any{"id": "i1"}}}},
			"out":  []any{map[string]any{"wires": []any{map[string]any{"id": "i1", "port": 0.0}}}},
			"nodes": []any{map[string]any{
				"id": "i1", "type": "junction",
				"position": map[string]any{"x": 1.0, "y": 1.0},
				"data":     map[string]any{},
			}},
			"edges": []any{},
		}),
	})
	exp, err := ExportSubflowDef(src, "def9")
	if err != nil {
		t.Fatalf("ExportSubflowDef: %v", err)
	}
	if exp.Module["role"] != "subflow" {
		t.Fatalf("def export role should be subflow, got %v", exp.Module["role"])
	}
	raw, _ := json.Marshal(exp)

	dst := NewWorkflow("").SetID("wf-dst")
	if _, err := ImportModule(dst, raw); err != nil {
		t.Fatalf("ImportModule: %v", err)
	}
	var newDefID string
	for _, n := range dst.GetNodes() {
		if n.GetType() == subflowDefNodeType {
			newDefID = n.GetID()
			requireWellFormedDef(t, n)
		}
	}
	if newDefID == "" || newDefID == "def9" {
		t.Fatalf("expected a re-minted definition, got %q", newDefID)
	}
	// An instance of the imported definition references it correctly.
	dst.AddNode(NewNode("inst1", "subflow:"+newDefID).SetData(map[string]any{}))
	def := findDef(dst, strings.TrimPrefix(nodeByID(dst, "inst1").GetType(), subflowTypePrefix))
	if def == nil {
		t.Fatal("instance references missing definition")
	}
}

// Bad payloads are rejected with clear errors.
func TestImportModuleRejectsBadPayload(t *testing.T) {
	w := NewWorkflow("")
	if _, err := ImportModule(w, []byte(`[1,2,3]`)); err == nil {
		t.Fatal("expected error for non-envelope JSON")
	}
	if _, err := ImportModule(w, []byte(`{"nodestore":"module","version":99}`)); err == nil {
		t.Fatal("expected error for newer version")
	}
	if _, err := ExportModule(w, "nope"); err == nil {
		t.Fatal("expected error for unknown module")
	}
}
