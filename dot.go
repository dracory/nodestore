package nodestore

import (
	"sort"
	"strings"
)

// This file exports workflow graphs as DOT (Graphviz) text. It is a
// pure format transformation of the stored domain model, like the
// Node-RED JSON importer/exporter: no I/O, no graphviz dependency and
// no runtime-engine dependency. Consumers supply any runtime status
// overlay they want rendered.

// DOT status colors. The canonical noderun node statuses are
// NodeStatusRunning/NodeStatusOK/NodeStatusError; complete/completed
// and failed are accepted as overlay aliases so consumers with the
// broader lifecycle vocabulary can use the same exporter.
const (
	dotColorRunning  = "lightblue"
	dotColorComplete = "lightgreen"
	dotColorFailed   = "lightcoral"
	dotColorPaused   = "yellow"
	dotColorDefault  = "lightgray"
)

// ExportDOT returns workflow as deterministic DOT graph text. Nodes are
// sorted by ID and edges by source/source port/target/target port/ID, so
// exporting the same workflow always produces the same text.
//
// statuses is an optional overlay keyed by node ID. When it is nil the
// graph is purely structural. When it is non-nil, every node receives a
// fill color: running is blue, ok/complete/completed are green,
// error/failed are red, paused is yellow, and absent or unrecognized
// statuses use the default gray fill.
//
// Edges whose source or target node is absent from the workflow are
// skipped. DOT creates implicit nodes for otherwise-unknown endpoints,
// which would silently invent graph structure the stored workflow does
// not contain.
func ExportDOT(workflow WorkflowInterface, statuses map[string]string) string {
	if workflow == nil {
		return "digraph \"workflow\" {\n  rankdir=\"LR\";\n}\n"
	}

	nodes := append([]NodeInterface(nil), workflow.GetNodes()...)
	sort.Slice(nodes, func(i, j int) bool {
		return nodes[i].GetID() < nodes[j].GetID()
	})
	edges := append([]EdgeInterface(nil), workflow.GetEdges()...)
	sort.Slice(edges, func(i, j int) bool {
		a, b := edges[i], edges[j]
		if a.GetSource() != b.GetSource() {
			return a.GetSource() < b.GetSource()
		}
		if a.GetSourceKey() != b.GetSourceKey() {
			return a.GetSourceKey() < b.GetSourceKey()
		}
		if a.GetTarget() != b.GetTarget() {
			return a.GetTarget() < b.GetTarget()
		}
		if a.GetTargetKey() != b.GetTargetKey() {
			return a.GetTargetKey() < b.GetTargetKey()
		}
		return a.GetID() < b.GetID()
	})

	graphID := workflow.GetID()
	if graphID == "" {
		graphID = "workflow"
	}
	var out strings.Builder
	out.WriteString("digraph ")
	out.WriteString(dotQuote(graphID))
	out.WriteString(" {\n")
	if workflow.GetName() != "" {
		out.WriteString("  label=")
		out.WriteString(dotQuote(workflow.GetName()))
		out.WriteString(";\n  labelloc=\"t\";\n")
	}
	out.WriteString("  rankdir=\"LR\";\n")
	out.WriteString("  node [shape=\"box\"];\n")

	known := make(map[string]bool, len(nodes))
	for _, node := range nodes {
		known[node.GetID()] = true
	}
	for i, node := range nodes {
		if i == 0 {
			out.WriteString("\n")
		}
		out.WriteString("  ")
		out.WriteString(dotQuote(node.GetID()))
		out.WriteString(" [label=")
		out.WriteString(dotQuote(dotNodeLabel(node)))
		if statuses != nil {
			out.WriteString(", fillcolor=")
			out.WriteString(dotQuote(dotStatusColor(statuses[node.GetID()])))
			out.WriteString(", style=\"filled\"")
		}
		out.WriteString("];\n")
	}

	firstEdge := true
	for _, edge := range edges {
		if !known[edge.GetSource()] || !known[edge.GetTarget()] {
			continue
		}
		if firstEdge {
			out.WriteString("\n")
			firstEdge = false
		}
		out.WriteString("  ")
		out.WriteString(dotEndpoint(edge.GetSource(), edge.GetSourceKey()))
		out.WriteString(" -> ")
		out.WriteString(dotEndpoint(edge.GetTarget(), edge.GetTargetKey()))
		out.WriteString(";\n")
	}
	out.WriteString("}\n")
	return out.String()
}

// dotNodeLabel prefers the user-facing node name stored in Data and
// falls back to the stable node ID. The type is the final fallback for
// malformed nodes with neither name nor ID.
func dotNodeLabel(node NodeInterface) string {
	if node == nil {
		return ""
	}
	if name, ok := node.GetData()["name"].(string); ok && name != "" {
		return name
	}
	if node.GetID() != "" {
		return node.GetID()
	}
	return node.GetType()
}

// dotStatusColor maps the overlay status vocabulary onto node fills.
func dotStatusColor(status string) string {
	switch status {
	case NodeStatusRunning:
		return dotColorRunning
	case NodeStatusOK, "complete", "completed":
		return dotColorComplete
	case NodeStatusError, "failed":
		return dotColorFailed
	case "paused":
		return dotColorPaused
	default:
		return dotColorDefault
	}
}

// dotEndpoint renders a node reference with an optional port. Both the
// node ID and port are quoted DOT IDs, so arbitrary stored identifiers
// remain valid and unambiguous.
func dotEndpoint(nodeID, port string) string {
	if port == "" {
		return dotQuote(nodeID)
	}
	return dotQuote(nodeID) + ":" + dotQuote(port)
}

// dotQuote quotes one DOT ID/label string. Graphviz treats backslash as
// the escape character inside quoted strings; escape it first, then
// quotes and line terminators.
func dotQuote(value string) string {
	value = strings.ReplaceAll(value, `\`, `\\`)
	value = strings.ReplaceAll(value, `"`, `\"`)
	value = strings.ReplaceAll(value, "\r", `\r`)
	value = strings.ReplaceAll(value, "\n", `\n`)
	return `"` + value + `"`
}
