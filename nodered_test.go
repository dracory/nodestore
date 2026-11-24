package nodestore

import (
	"encoding/json"
	"testing"
)

// nodeByID returns the node with the given id or nil.
func nodeByID(w WorkflowInterface, id string) NodeInterface {
	for _, n := range w.GetNodes() {
		if n.GetID() == id {
			return n
		}
	}
	return nil
}

// edgesFrom returns the outgoing edges of a node as
// sourceKey -> []targetID.
func edgesFrom(w WorkflowInterface, id string) map[string][]string {
	out := map[string][]string{}
	for _, e := range w.GetEdges() {
		if e.GetSource() == id {
			out[e.GetSourceKey()] = append(out[e.GetSourceKey()], e.GetTarget())
		}
	}
	return out
}

// TestNodeREDTypeMapping verifies the Node-RED -> GoFlow type-name
// table: every core type with a GoFlow equivalent lands mapped, a
// "link call" gets its links array folded into GoFlow's "link" field,
// and an unknown type keeps its raw spelling.
func TestNodeREDTypeMapping(t *testing.T) {
	flowJSON := `[
		{"id":"tab1","type":"tab","label":"Main"},
		{"id":"nHttpIn","type":"http in","z":"tab1","url":"/a","wires":[[]]},
		{"id":"nHttpRes","type":"http response","z":"tab1","wires":[[]]},
		{"id":"nHttpReq","type":"http request","z":"tab1","method":"GET","wires":[[]]},
		{"id":"nLinkIn","type":"link in","z":"tab1","wires":[[]]},
		{"id":"nLinkOut","type":"link out","z":"tab1","links":["nLinkIn"],"mode":"link","wires":[[]]},
		{"id":"nLinkCall","type":"link call","z":"tab1","links":["nLinkIn"],"linkType":"static","wires":[[]]},
		{"id":"nMqttIn","type":"mqtt in","z":"tab1","topic":"t","wires":[[]]},
		{"id":"nMqttOut","type":"mqtt out","z":"tab1","topic":"t","wires":[[]]},
		{"id":"nTcpIn","type":"tcp in","z":"tab1","wires":[[]]},
		{"id":"nTcpOut","type":"tcp out","z":"tab1","wires":[[]]},
		{"id":"nUdpIn","type":"udp in","z":"tab1","wires":[[]]},
		{"id":"nUdpOut","type":"udp out","z":"tab1","wires":[[]]},
		{"id":"nWsIn","type":"websocket in","z":"tab1","wires":[[]]},
		{"id":"nWsOut","type":"websocket out","z":"tab1","wires":[[]]},
		{"id":"nFileIn","type":"file in","z":"tab1","filename":"/tmp/x","wires":[[]]},
		{"id":"nFile","type":"file","z":"tab1","filename":"/tmp/y","wires":[[]]},
		{"id":"nUnknown","type":"fancy-widget","z":"tab1","conf":1,"wires":[[]]}
	]`

	w, err := ImportNodeRED([]byte(flowJSON))
	if err != nil {
		t.Fatalf("import failed: %v", err)
	}

	want := map[string]string{
		"nHttpIn":   "httpTrigger",
		"nHttpRes":  "httpResponse",
		"nHttpReq":  "httpRequest",
		"nLinkIn":   "linkIn",
		"nLinkOut":  "linkOut",
		"nLinkCall": "linkCall",
		"nMqttIn":   "mqttIn",
		"nMqttOut":  "mqttOut",
		"nTcpIn":    "tcpIn",
		"nTcpOut":   "tcpOut",
		"nUdpIn":    "udpIn",
		"nUdpOut":   "udpOut",
		"nWsIn":     "websocketIn",
		"nWsOut":    "websocketOut",
		"nFileIn":   "fileIn",
		"nFile":     "file",
		"nUnknown":  "fancy-widget",
	}
	for id, wantType := range want {
		n := nodeByID(w, id)
		if n == nil {
			t.Fatalf("node %s missing after import", id)
		}
		if n.GetType() != wantType {
			t.Errorf("node %s: type = %q, want %q", id, n.GetType(), wantType)
		}
	}

	// "link call": Node-RED links[0] becomes GoFlow's link field.
	call := nodeByID(w, "nLinkCall")
	if stringOf(call.GetData()["link"]) != "nLinkIn" {
		t.Errorf("linkCall data.link = %v, want nLinkIn", call.GetData()["link"])
	}
	if _, still := call.GetData()["links"]; still {
		t.Errorf("linkCall data still carries Node-RED links array: %v", call.GetData()["links"])
	}

	// The unknown node keeps its config verbatim.
	unk := nodeByID(w, "nUnknown")
	if unk.GetData()["conf"] != float64(1) {
		t.Errorf("unknown node lost config: %v", unk.GetData())
	}
}

// TestNodeREDExportTypeMapping verifies the reverse table: GoFlow
// executor types export back to Node-RED spellings and the linkCall
// link field becomes a links array again.
func TestNodeREDExportTypeMapping(t *testing.T) {
	w := NewWorkflow("Export map").SetID("w1")
	w.SetNodes([]NodeInterface{
		NewNode("a", "httpTrigger").SetData(map[string]any{"url": "/a"}),
		NewNode("b", "linkCall").SetData(map[string]any{"link": "c", "linkType": "static"}),
		NewNode("c", "linkIn").SetData(map[string]any{}),
		NewNode("d", "fileIn").SetData(map[string]any{"filename": "/tmp/x"}),
		NewNode("e", "mqttOut").SetData(map[string]any{"topic": "t"}),
	})
	raw, err := ExportNodeRED(w)
	if err != nil {
		t.Fatalf("export failed: %v", err)
	}
	var items []map[string]any
	if err := json.Unmarshal(raw, &items); err != nil {
		t.Fatalf("export produced invalid JSON: %v", err)
	}
	got := map[string]map[string]any{}
	for _, it := range items {
		got[stringOf(it["id"])] = it
	}
	wantType := map[string]string{
		"a": "http in",
		"b": "link call",
		"c": "link in",
		"d": "file in",
		"e": "mqtt out",
	}
	for id, wt := range wantType {
		if stringOf(got[id]["type"]) != wt {
			t.Errorf("node %s exported type %q, want %q", id, got[id]["type"], wt)
		}
	}
	links := arrayOf(got["b"]["links"])
	if len(links) != 1 || stringOf(links[0]) != "c" {
		t.Errorf("linkCall exported links = %v, want [c]", got["b"]["links"])
	}
	if _, still := got["b"]["link"]; still {
		t.Errorf("linkCall export still carries GoFlow link field")
	}
}

// TestNodeREDMultiTabSwitch exercises a real-style multi-tab export:
// node z -> module mapping and a 2-output switch whose wires[i]
// become output_{i+1} fan-out edges.
func TestNodeREDMultiTabSwitch(t *testing.T) {
	flowJSON := `[
		{"id":"tabA","type":"tab","label":"First"},
		{"id":"tabB","type":"tab","label":"Second"},
		{"id":"sw","type":"switch","z":"tabA","x":10,"y":10,
		 "rules":[{"t":"eq","v":"a"},{"t":"eq","v":"b"}],
		 "wires":[["dbgA"],["dbgB","dbgC"]]},
		{"id":"dbgA","type":"debug","z":"tabA","wires":[[]]},
		{"id":"dbgB","type":"debug","z":"tabB","wires":[[]]},
		{"id":"dbgC","type":"debug","z":"tabB","wires":[[]]},
		{"id":"inj","type":"inject","z":"tabB","d":true,"payload":"x","wires":[["sw"]]}
	]`

	w, err := ImportNodeRED([]byte(flowJSON))
	if err != nil {
		t.Fatalf("import failed: %v", err)
	}

	// z -> module mapping
	wantModule := map[string]string{
		"sw": "First", "dbgA": "First",
		"dbgB": "Second", "dbgC": "Second", "inj": "Second",
	}
	for id, mod := range wantModule {
		n := nodeByID(w, id)
		if n == nil {
			t.Fatalf("node %s missing", id)
		}
		if stringOf(n.GetData()["module"]) != mod {
			t.Errorf("node %s module = %q, want %q", id, n.GetData()["module"], mod)
		}
	}

	// wires[i] -> output_{i+1} fan-out, including two targets on one port
	sw := edgesFrom(w, "sw")
	if len(sw["output_1"]) != 1 || sw["output_1"][0] != "dbgA" {
		t.Errorf("switch output_1 wires = %v, want [dbgA]", sw["output_1"])
	}
	if len(sw["output_2"]) != 2 || sw["output_2"][0] != "dbgB" || sw["output_2"][1] != "dbgC" {
		t.Errorf("switch output_2 wires = %v, want [dbgB dbgC]", sw["output_2"])
	}

	// Disabled flag preserved verbatim in data.
	inj := nodeByID(w, "inj")
	if inj.GetData()["d"] != true {
		t.Errorf("disabled flag not preserved: d = %v", inj.GetData()["d"])
	}
}

// TestNodeREDLinkNodesWiring imports a Node-RED link in/out pair and
// proves the converter produces the mapped linkOut/linkIn types and
// the downstream wiring the runtime relies on (the flowrunner
// original additionally executed the flow end to end; nodestore has
// no engine, so this test asserts the converted graph shape).
func TestNodeREDLinkNodesWiring(t *testing.T) {
	flowJSON := `[
		{"id":"tab1","type":"tab","label":"Main"},
		{"id":"inj","type":"inject","z":"tab1","payload":"via-link","payloadType":"str","once":true,"wires":[["lout"]]},
		{"id":"lout","type":"link out","z":"tab1","links":["lin"],"mode":"link","wires":[[]]},
		{"id":"lin","type":"link in","z":"tab1","wires":[["dbg"]]},
		{"id":"dbg","type":"debug","z":"tab1","active":true,"wires":[[]]}
	]`

	w, err := ImportNodeRED([]byte(flowJSON))
	if err != nil {
		t.Fatalf("import failed: %v", err)
	}
	if nodeByID(w, "lout").GetType() != "linkOut" || nodeByID(w, "lin").GetType() != "linkIn" {
		t.Fatalf("link types not mapped: %+v", w.GetNodes())
	}
	if got := arrayOf(nodeByID(w, "lout").GetData()["links"]); len(got) != 1 || stringOf(got[0]) != "lin" {
		t.Fatalf("linkOut links not preserved: %v", nodeByID(w, "lout").GetData())
	}
	if got := edgesFrom(w, "lin")["output_1"]; len(got) != 1 || got[0] != "dbg" {
		t.Fatalf("linkIn downstream wiring missing: %v", edgesFrom(w, "lin"))
	}
}

// TestNodeREDTypeMappingRoundTrip proves an imported Node-RED flow
// exports back to the original type spellings.
func TestNodeREDTypeMappingRoundTrip(t *testing.T) {
	flowJSON := `[
		{"id":"tab1","type":"tab","label":"Main"},
		{"id":"h","type":"http in","z":"tab1","url":"/x","wires":[["r"]]},
		{"id":"r","type":"http response","z":"tab1","wires":[[]]},
		{"id":"m","type":"mqtt out","z":"tab1","topic":"t","wires":[[]]}
	]`
	w, err := ImportNodeRED([]byte(flowJSON))
	if err != nil {
		t.Fatalf("import failed: %v", err)
	}
	raw, err := ExportNodeRED(w)
	if err != nil {
		t.Fatalf("export failed: %v", err)
	}
	var items []map[string]any
	if err := json.Unmarshal(raw, &items); err != nil {
		t.Fatalf("bad export JSON: %v", err)
	}
	got := map[string]string{}
	for _, it := range items {
		got[stringOf(it["id"])] = stringOf(it["type"])
	}
	for id, wt := range map[string]string{"h": "http in", "r": "http response", "m": "mqtt out"} {
		if got[id] != wt {
			t.Errorf("round-trip type for %s = %q, want %q", id, got[id], wt)
		}
	}
}

// TestNodeREDEnvRoundTrip proves flow env on tab objects imports into
// workflow view["env"] and exports back onto emitted tabs (Node-RED
// parity: tab env is exported).
func TestNodeREDEnvRoundTrip(t *testing.T) {
	flowJSON := `[
		{"id":"tab1","type":"tab","label":"Main",
		 "env":[{"name":"SHARED","type":"str","value":"one"}]},
		{"id":"inj","type":"inject","z":"tab1","payload":"x","wires":[["dbg"]]},
		{"id":"dbg","type":"debug","z":"tab1","wires":[[]]}
	]`

	w, err := ImportNodeRED([]byte(flowJSON))
	if err != nil {
		t.Fatalf("import failed: %v", err)
	}
	env, _ := w.GetView()["env"].([]any)
	if len(env) != 1 {
		t.Fatalf("expected 1 env entry in view.env, got %v", w.GetView()["env"])
	}
	m, _ := env[0].(map[string]any)
	if m["name"] != "SHARED" || m["value"] != "one" {
		t.Fatalf("unexpected env entry: %v", m)
	}

	out, err := ExportNodeRED(w)
	if err != nil {
		t.Fatalf("export failed: %v", err)
	}
	var items []map[string]any
	if err := json.Unmarshal(out, &items); err != nil {
		t.Fatalf("export produced invalid JSON: %v", err)
	}
	var tab map[string]any
	for _, it := range items {
		if stringOf(it["type"]) == "tab" {
			tab = it
		}
	}
	if tab == nil {
		t.Fatal("export produced no tab object")
	}
	tabEnv, _ := tab["env"].([]any)
	if len(tabEnv) != 1 {
		t.Fatalf("expected env on exported tab, got %v", tab["env"])
	}
	em, _ := tabEnv[0].(map[string]any)
	if em["name"] != "SHARED" || em["value"] != "one" {
		t.Fatalf("exported tab env mismatch: %v", em)
	}
}
