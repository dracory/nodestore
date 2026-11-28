package nodestore

import (
	"strings"
	"testing"
)

func TestExportDOTStructuralOutput(t *testing.T) {
	wf := NewWorkflow("Test Flow").SetID("wf-test").SetNodes([]NodeInterface{
		NewNode("a", "inject").SetData(map[string]any{"name": "Alpha"}),
		NewNode("b", "debug").SetData(map[string]any{}),
	}).SetEdges([]EdgeInterface{
		NewEdge("e1", "a", "b").SetSourceKey("output_1").SetTargetKey("input_1"),
	})

	got := ExportDOT(wf, nil)
	want := "digraph \"wf-test\" {\n" +
		"  label=\"Test Flow\";\n" +
		"  labelloc=\"t\";\n" +
		"  rankdir=\"LR\";\n" +
		"  node [shape=\"box\"];\n" +
		"\n" +
		"  \"a\" [label=\"Alpha\"];\n" +
		"  \"b\" [label=\"b\"];\n" +
		"\n" +
		"  \"a\":\"output_1\" -> \"b\":\"input_1\";\n" +
		"}\n"
	if got != want {
		t.Fatalf("DOT mismatch\nwant:\n%s\ngot:\n%s", want, got)
	}
}

func TestExportDOTDeterministicOrderingAndNoMutation(t *testing.T) {
	nodes := []NodeInterface{
		NewNode("z", "junction").SetData(map[string]any{}),
		NewNode("a", "junction").SetData(map[string]any{}),
		NewNode("m", "junction").SetData(map[string]any{}),
	}
	edges := []EdgeInterface{
		NewEdge("e2", "z", "a"),
		NewEdge("e1", "a", "m"),
		NewEdge("e3", "a", "z"),
	}
	scrambled := NewWorkflow("Order").SetID("order").SetNodes(nodes).SetEdges(edges)
	first := ExportDOT(scrambled, nil)
	second := ExportDOT(scrambled, nil)
	if first != second {
		t.Fatal("repeated export differs")
	}
	if got := scrambled.GetNodes(); got[0].GetID() != "z" || got[1].GetID() != "a" || got[2].GetID() != "m" {
		t.Fatalf("ExportDOT mutated node order: %v", got)
	}
	if got := scrambled.GetEdges(); got[0].GetID() != "e2" || got[1].GetID() != "e1" || got[2].GetID() != "e3" {
		t.Fatalf("ExportDOT mutated edge order: %v", got)
	}

	aNode := strings.Index(first, "\"a\" [label=")
	mNode := strings.Index(first, "\"m\" [label=")
	zNode := strings.Index(first, "\"z\" [label=")
	if !(aNode >= 0 && aNode < mNode && mNode < zNode) {
		t.Fatalf("nodes not sorted by ID:\n%s", first)
	}
	e1 := strings.Index(first, "\"a\" -> \"m\";")
	e3 := strings.Index(first, "\"a\" -> \"z\";")
	e2 := strings.Index(first, "\"z\" -> \"a\";")
	if !(e1 >= 0 && e1 < e3 && e3 < e2) {
		t.Fatalf("edges not sorted deterministically:\n%s", first)
	}
}

func TestExportDOTEscapesLabelsAndIDs(t *testing.T) {
	wf := NewWorkflow("Escapes \"Flow\"\\Name\nLine").SetID("wf\"\\\n").SetNodes([]NodeInterface{
		NewNode("n\"\\\n", "junction").SetData(map[string]any{"name": "Quote \" Backslash \\ Newline\nEnd"}),
	})

	got := ExportDOT(wf, nil)
	for _, want := range []string{
		`label="Escapes \"Flow\"\\Name\nLine";`,
		`digraph "wf\"\\\n"`,
		`"n\"\\\n" [label="Quote \" Backslash \\ Newline\nEnd"];`,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("escaped fragment %q missing from:\n%s", want, got)
		}
	}
}

func TestExportDOTStatusOverlayColors(t *testing.T) {
	wf := NewWorkflow("Statuses").SetID("statuses").SetNodes([]NodeInterface{
		NewNode("running", "junction").SetData(map[string]any{}),
		NewNode("ok", "junction").SetData(map[string]any{}),
		NewNode("error", "junction").SetData(map[string]any{}),
		NewNode("paused", "junction").SetData(map[string]any{}),
		NewNode("complete", "junction").SetData(map[string]any{}),
		NewNode("failed", "junction").SetData(map[string]any{}),
		NewNode("unknown", "junction").SetData(map[string]any{}),
		NewNode("absent", "junction").SetData(map[string]any{}),
	})
	got := ExportDOT(wf, map[string]string{
		"running":  NodeStatusRunning,
		"ok":       NodeStatusOK,
		"error":    NodeStatusError,
		"paused":   "paused",
		"complete": "complete",
		"failed":   "failed",
		"unknown":  "something-else",
	})
	expect := map[string]string{
		"running":  "lightblue",
		"ok":       "lightgreen",
		"error":    "lightcoral",
		"paused":   "yellow",
		"complete": "lightgreen",
		"failed":   "lightcoral",
		"unknown":  "lightgray",
		"absent":   "lightgray",
	}
	for id, color := range expect {
		fragment := `"` + id + `" [label="` + id + `", fillcolor="` + color + `", style="filled"];`
		if !strings.Contains(got, fragment) {
			t.Fatalf("status fragment %q missing from:\n%s", fragment, got)
		}
	}
	distinct := map[string]bool{
		expect["running"]:  true,
		expect["complete"]: true,
		expect["failed"]:   true,
		expect["paused"]:   true,
	}
	if len(distinct) != 4 {
		t.Fatalf("running/complete/failed/paused colors are not distinct: %v", distinct)
	}
}

func TestExportDOTEmptyWorkflow(t *testing.T) {
	got := ExportDOT(NewWorkflow("Empty").SetID("empty"), nil)
	want := "digraph \"empty\" {\n" +
		"  label=\"Empty\";\n" +
		"  labelloc=\"t\";\n" +
		"  rankdir=\"LR\";\n" +
		"  node [shape=\"box\"];\n" +
		"}\n"
	if got != want {
		t.Fatalf("empty workflow DOT mismatch\nwant:\n%s\ngot:\n%s", want, got)
	}
}

func TestExportDOTSkipsEdgesReferencingMissingNodes(t *testing.T) {
	wf := NewWorkflow("Missing").SetID("missing").SetNodes([]NodeInterface{
		NewNode("a", "junction").SetData(map[string]any{}),
		NewNode("b", "junction").SetData(map[string]any{}),
	}).SetEdges([]EdgeInterface{
		NewEdge("bad-source", "missing", "b"),
		NewEdge("good", "a", "b"),
		NewEdge("bad-target", "a", "missing"),
	})
	got := ExportDOT(wf, nil)
	if !strings.Contains(got, "\"a\" -> \"b\";") {
		t.Fatalf("valid edge missing:\n%s", got)
	}
	if strings.Contains(got, "missing\" ->") || strings.Contains(got, "-> \"missing") {
		t.Fatalf("edge referencing a missing node was exported:\n%s", got)
	}
}
