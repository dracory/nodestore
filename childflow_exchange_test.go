package nodestore

import (
	"encoding/json"
	"strings"
	"testing"
)

// requireWellFormedDef asserts that a type:"component" definition's
// internal graph is intact: every internal node parses through
// parseComponentInnerNode and every internal edge decodes with endpoints
// among the internal node ids. It stands in for flowrunner's
// ExpandComponents check (nodestore has no engine), proving the imported
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
		inner, err := parseComponentInnerNode(mustJSON(m))
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

// findDef returns the first type:"component" definition node of w.
func findDef(w WorkflowInterface, id string) NodeInterface {
	for _, n := range w.GetNodes() {
		if n.GetType() == componentDefNodeType && (id == "" || n.GetID() == id) {
			return n
		}
	}
	return nil
}

// Child flow export/import round-trip: a child flow exported from workflow A
// and imported into workflow B must arrive with every id re-minted,
// internal references rewritten, and its component definitions intact
// enough that a deploy-time expansion could instantiate them.
func TestChildFlowExportImportRoundTrip(t *testing.T) {
	src := NewWorkflow("source").SetID("wf-src")
	src.SetNodes([]NodeInterface{
		NewNode("def1", "component").SetData(map[string]any{
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
		NewNode("n1", "inject").SetData(map[string]any{"name": "go", "childFlow": "mod-a"}),
		NewNode("n2", "component:def1").SetData(map[string]any{"name": "inst", "childFlow": "mod-a"}),
		NewNode("n3", "debug").SetData(map[string]any{"name": "out", "childFlow": "mod-a"}),
		NewNode("n4", "linkOut").SetData(map[string]any{"name": "lo", "childFlow": "mod-a", "links": []any{"ext-1"}}),
		// Other-childFlow + unassigned nodes must not export.
		NewNode("m1", "debug").SetData(map[string]any{"name": "other", "childFlow": "mod-b"}),
		NewNode("m2", "linkIn").SetData(map[string]any{"name": "li", "childFlow": "mod-b"}),
	})
	src.SetEdges([]EdgeInterface{
		NewEdge("e1", "n1", "n2").SetSourceKey("output_1").SetTargetKey("input_1"),
		NewEdge("e2", "n2", "n3").SetSourceKey("output_1").SetTargetKey("input_1"),
		NewEdge("e3", "n3", "m1").SetSourceKey("output_1").SetTargetKey("input_1"),
	})
	src.SetView(map[string]any{
		"childFlows": []any{
			map[string]any{"id": "mod-a", "label": "Alpha", "role": "flow", "enabled": true},
			map[string]any{"id": "mod-b", "label": "Beta", "role": "flow", "enabled": true},
		},
		"groups": []any{
			map[string]any{"id": "g1", "childFlow": "mod-a", "name": "grp", "nodeIds": []any{"n1", "n3"}},
		},
	})

	exp, err := ExportChildFlow(src, "mod-a")
	if err != nil {
		t.Fatalf("ExportChildFlow: %v", err)
	}
	if exp.Format != "child_flow" || exp.Version != 1 {
		t.Fatalf("bad envelope header: %+v", exp)
	}
	if got := len(exp.Nodes); got != 4 {
		t.Fatalf("expected 4 exported nodes, got %d", got)
	}
	for _, n := range exp.Nodes {
		if _, ok := n.GetData()["childFlow"]; ok {
			t.Fatalf("exported node %s still carries data.childFlow", n.GetID())
		}
	}
	if got := len(exp.Edges); got != 2 {
		t.Fatalf("expected 2 intra-child-flow edges, got %d", got)
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
		NewNode("x1", "debug").SetData(map[string]any{"childFlow": "home"}),
	})
	dst.SetView(map[string]any{
		"childFlows": []any{
			map[string]any{"id": "home", "label": "Alpha", "role": "flow", "enabled": true},
		},
	})
	warnings, err := ImportChildFlow(dst, raw)
	if err != nil {
		t.Fatalf("ImportChildFlow: %v", err)
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

	// The child flow entity must be registered with a fresh id and a
	// deduped label ("Alpha" is taken in the destination workflow).
	var entity map[string]any
	for _, m := range arrayOf(dst.GetView()["childFlows"]) {
		mm := m.(map[string]any)
		if mm["id"] != "home" {
			entity = mm
		}
	}
	if entity == nil {
		t.Fatal("imported child flow entity missing from view.childFlows")
	}
	newID := entity["id"].(string)
	if newID == "mod-a" {
		t.Fatal("child flow id must be re-minted on import")
	}
	if entity["label"] == "Alpha" {
		t.Fatal("label collision must produce a deduped label")
	}

	// Imported nodes: fresh ids, data.childFlow = new child flow id.
	var imported []NodeInterface
	for _, n := range dst.GetNodes() {
		if n.GetData()["childFlow"] == newID {
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
		if n.GetData()["childFlowName"] != entity["label"] {
			t.Fatalf("childFlowName %v does not match deduped label %v", n.GetData()["childFlowName"], entity["label"])
		}
		if IsComponentInstance(n.GetType()) {
			instID = n.GetID()
			newDefID := strings.TrimPrefix(n.GetType(), componentTypePrefix)
			if newDefID == "def1" {
				t.Fatal("instance type must point at the re-minted definition id")
			}
			if findDef(dst, newDefID) == nil {
				t.Fatalf("instance references missing definition %q", newDefID)
			}
		}
	}
	if instID == "" {
		t.Fatal("component instance missing from import")
	}

	// Imported edges: exactly the two intra-child-flow ones, remapped.
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
	if g["childFlow"] != newID {
		t.Fatalf("group childFlow not rebound: %v", g["childFlow"])
	}
	for _, m := range arrayOf(g["nodeIds"]) {
		if !importedIDs[stringOf(m)] {
			t.Fatalf("group member %v not remapped", m)
		}
	}

	// The imported definition must be structurally intact — every
	// internal node decodable and every internal edge endpoint valid.
	for _, n := range dst.GetNodes() {
		if n.GetType() == componentDefNodeType {
			requireWellFormedDef(t, n)
		}
	}
}

// Importing into the SOURCE workflow produces a second copy whose
// external refs still resolve (they pointed at nodes that exist here),
// so no dangling warnings besides the ones carried by the export.
func TestChildFlowImportIntoSameWorkflow(t *testing.T) {
	src := NewWorkflow("source").SetID("wf-src")
	src.SetNodes([]NodeInterface{
		NewNode("a1", "linkIn").SetData(map[string]any{"childFlow": "mod-x"}),
		NewNode("a2", "linkOut").SetData(map[string]any{"childFlow": "mod-x", "links": "a1"}),
		NewNode("b1", "debug").SetData(map[string]any{"childFlow": "mod-y"}),
	})
	src.SetView(map[string]any{
		"childFlows": []any{
			map[string]any{"id": "mod-x", "label": "X"},
			map[string]any{"id": "mod-y", "label": "Y"},
		},
	})
	exp, err := ExportChildFlow(src, "mod-x")
	if err != nil {
		t.Fatalf("ExportChildFlow: %v", err)
	}
	if len(exp.Warnings) != 0 {
		t.Fatalf("internal linkOut ref should not warn, got %v", exp.Warnings)
	}
	raw, _ := json.Marshal(exp)
	warnings, err := ImportChildFlow(src, raw)
	if err != nil {
		t.Fatalf("ImportChildFlow: %v", err)
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

// A definition-only export (ExportComponentDef) imports as a working
// definition: its internal graph stays well-formed and an instance can
// be created against the re-minted id.
func TestComponentDefExportImport(t *testing.T) {
	src := NewWorkflow("").SetID("wf-src")
	src.SetNodes([]NodeInterface{
		NewNode("def9", "component").SetData(map[string]any{
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
	exp, err := ExportComponentDef(src, "def9")
	if err != nil {
		t.Fatalf("ExportComponentDef: %v", err)
	}
	if exp.ChildFlow["role"] != "component" {
		t.Fatalf("def export role should be component, got %v", exp.ChildFlow["role"])
	}
	raw, _ := json.Marshal(exp)

	dst := NewWorkflow("").SetID("wf-dst")
	if _, err := ImportChildFlow(dst, raw); err != nil {
		t.Fatalf("ImportChildFlow: %v", err)
	}
	var newDefID string
	for _, n := range dst.GetNodes() {
		if n.GetType() == componentDefNodeType {
			newDefID = n.GetID()
			requireWellFormedDef(t, n)
		}
	}
	if newDefID == "" || newDefID == "def9" {
		t.Fatalf("expected a re-minted definition, got %q", newDefID)
	}
	// An instance of the imported definition references it correctly.
	dst.AddNode(NewNode("inst1", "component:"+newDefID).SetData(map[string]any{}))
	instance := nodeByID(dst, "inst1")
	if instance == nil {
		t.Fatal("imported instance missing")
	}
	def := findDef(dst, strings.TrimPrefix(instance.GetType(), componentTypePrefix))
	if def == nil {
		t.Fatal("instance references missing definition")
	}
}

// Bad payloads are rejected with clear errors.
func TestImportChildFlowRejectsBadPayload(t *testing.T) {
	w := NewWorkflow("")
	if _, err := ImportChildFlow(w, []byte(`[1,2,3]`)); err == nil {
		t.Fatal("expected error for non-envelope JSON")
	}
	if _, err := ImportChildFlow(w, []byte(`{"nodestore":"childFlow","version":99}`)); err == nil {
		t.Fatal("expected error for newer version")
	}
	if _, err := ExportChildFlow(w, "nope"); err == nil {
		t.Fatal("expected error for unknown child flow")
	}
}
