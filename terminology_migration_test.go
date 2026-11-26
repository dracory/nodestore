package nodestore

import (
	"encoding/json"
	"testing"

	"github.com/dracory/entitystore"
)

func TestLegacyNodeTerminologyNormalizesOnRead(t *testing.T) {
	entity := entitystore.NewEntity()
	entity.SetType(EntityTypeNode)
	data, _ := json.Marshal(map[string]any{
		"module":     "legacy-flow",
		"moduleName": "Legacy Flow",
	})
	node, err := nodeFromEntity(entity, map[string]string{
		ATTRIBUTE_TYPE: "subflow:legacy-component",
		ATTRIBUTE_DATA: string(data),
	})
	if err != nil {
		t.Fatal(err)
	}
	if node.GetType() != "component:legacy-component" {
		t.Fatalf("type = %q", node.GetType())
	}
	if node.GetData()["childFlow"] != "legacy-flow" {
		t.Fatalf("childFlow = %v", node.GetData()["childFlow"])
	}
	if node.GetData()["childFlowName"] != "Legacy Flow" {
		t.Fatalf("childFlowName = %v", node.GetData()["childFlowName"])
	}
	if _, exists := node.GetData()["module"]; exists {
		t.Fatal("legacy module key still present")
	}
}

func TestLegacyViewTerminologyNormalizesOnRead(t *testing.T) {
	entity := entitystore.NewEntity()
	entity.SetType(EntityTypeWorkflow)
	view, _ := json.Marshal(map[string]any{
		"modules": []any{
			map[string]any{"id": "m1", "label": "Legacy", "role": "subflow", "enabled": true},
		},
		"disabledModules": []any{"m1"},
		"treeGroups": []any{
			map[string]any{"id": "folder", "modules": []any{"m1"}},
		},
		"groups": []any{
			map[string]any{"id": "group", "module": "m1"},
		},
	})
	workflow, err := workflowFromEntity(entity, map[string]string{ATTRIBUTE_VIEW: string(view)})
	if err != nil {
		t.Fatal(err)
	}
	got := workflow.GetView()
	if _, exists := got["modules"]; exists {
		t.Fatal("legacy modules key still present")
	}
	if _, exists := got["disabledModules"]; exists {
		t.Fatal("legacy disabledModules key still present")
	}
	flows := arrayOf(got["childFlows"])
	if len(flows) != 1 {
		t.Fatalf("childFlows = %v", flows)
	}
	entityMap := flows[0].(map[string]any)
	if entityMap["role"] != "component" {
		t.Fatalf("role = %v", entityMap["role"])
	}
	if len(arrayOf(got["disabledFlows"])) != 1 {
		t.Fatalf("disabledFlows = %v", got["disabledFlows"])
	}
	folder := arrayOf(got["treeGroups"])[0].(map[string]any)
	if len(arrayOf(folder["childFlows"])) != 1 {
		t.Fatalf("tree group childFlows = %v", folder)
	}
	group := arrayOf(got["groups"])[0].(map[string]any)
	if group["childFlow"] != "m1" {
		t.Fatalf("group childFlow = %v", group)
	}
}

func TestLegacyDefinitionInternalsNormalizeOnRead(t *testing.T) {
	data := map[string]any{
		"nodes": []any{
			map[string]any{"id": "in", "type": "subflowIn", "data": map[string]any{}},
			map[string]any{"id": "instance", "type": "subflow:inner", "data": map[string]any{"module": "nested"}},
		},
	}
	normalizeLegacyDataTerminology(data)
	nodes := arrayOf(data["nodes"])
	if nodes[0].(map[string]any)["type"] != "componentIn" {
		t.Fatalf("port type = %v", nodes[0])
	}
	inner := nodes[1].(map[string]any)
	if inner["type"] != "component:inner" {
		t.Fatalf("instance type = %v", inner["type"])
	}
	innerData := inner["data"].(map[string]any)
	if innerData["childFlow"] != "nested" {
		t.Fatalf("nested childFlow = %v", innerData)
	}
}

func TestLegacyChildFlowEnvelopeNormalizesOnDecode(t *testing.T) {
	raw, _ := json.Marshal(map[string]any{
		"nodestore": "module",
		"version":   1,
		"module":    map[string]any{"id": "m1", "label": "Legacy", "role": "subflow", "enabled": true},
		"nodes": []any{
			map[string]any{"id": "n1", "type": "subflow:def1", "position": map[string]any{"x": 0, "y": 0}, "data": map[string]any{"module": "m1"}},
		},
		"edges": []any{},
		"groups": []any{
			map[string]any{"id": "g1", "module": "m1", "nodeIds": []any{"n1"}},
		},
		"defs": []any{
			map[string]any{"id": "def1", "type": "subflow", "position": map[string]any{"x": 0, "y": 0}, "data": map[string]any{"nodes": []any{map[string]any{"id": "in", "type": "subflowIn", "data": map[string]any{}}}}},
		},
	})
	var export ChildFlowExport
	if err := json.Unmarshal(raw, &export); err != nil {
		t.Fatal(err)
	}
	if export.Format != childFlowExportFormat {
		t.Fatalf("format = %q", export.Format)
	}
	if export.ChildFlow["role"] != "component" {
		t.Fatalf("role = %v", export.ChildFlow["role"])
	}
	if export.Nodes[0].GetType() != "component:def1" {
		t.Fatalf("node type = %q", export.Nodes[0].GetType())
	}
	if export.Nodes[0].GetData()["childFlow"] != "m1" {
		t.Fatalf("node childFlow = %v", export.Nodes[0].GetData())
	}
	if export.Defs[0].GetType() != componentDefNodeType {
		t.Fatalf("def type = %q", export.Defs[0].GetType())
	}
	group := export.Groups[0].(map[string]any)
	if group["childFlow"] != "m1" {
		t.Fatalf("group = %v", group)
	}
}
