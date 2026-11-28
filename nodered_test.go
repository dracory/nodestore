package nodestore

import (
	"encoding/json"
	"strings"
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

// nodeByType returns the first node of the given type or nil —
// imports re-mint ids, so tests locate nodes by type, not source id.
func nodeByType(w WorkflowInterface, nodeType string) NodeInterface {
	for _, n := range w.GetNodes() {
		if n.GetType() == nodeType {
			return n
		}
	}
	return nil
}

// nodesByType returns all nodes of the given type.
func nodesByType(w WorkflowInterface, nodeType string) []NodeInterface {
	var out []NodeInterface
	for _, n := range w.GetNodes() {
		if n.GetType() == nodeType {
			out = append(out, n)
		}
	}
	return out
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

// TestNodeREDTypeMapping verifies the Node-RED -> engine type-name
// table: every core type with an engine equivalent lands mapped, a
// "link call" gets its links array folded into the engine's "link" field,
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

	// Every imported type is unique in this fixture, so nodes are
	// located by their mapped type (ids are re-minted on import).
	wantTypes := []string{
		"httpTrigger", "httpResponse", "httpRequest",
		"linkIn", "linkOut", "linkCall",
		"mqttIn", "mqttOut",
		"tcpIn", "tcpOut",
		"udpIn", "udpOut",
		"websocketIn", "websocketOut",
		"fileIn", "file",
		"fancy-widget",
	}
	for _, wantType := range wantTypes {
		if nodeByType(w, wantType) == nil {
			t.Errorf("node of type %s missing after import", wantType)
		}
	}

	// "link call": Node-RED links[0] becomes the engine's link field,
	// remapped to the linkIn node's NEW id.
	call := nodeByType(w, "linkCall")
	linkIn := nodeByType(w, "linkIn")
	if call == nil || linkIn == nil {
		t.Fatalf("link nodes missing after import")
	}
	if stringOf(call.GetData()["link"]) != linkIn.GetID() {
		t.Errorf("linkCall data.link = %v, want %v (remapped linkIn id)", call.GetData()["link"], linkIn.GetID())
	}
	if _, still := call.GetData()["links"]; still {
		t.Errorf("linkCall data still carries Node-RED links array: %v", call.GetData()["links"])
	}

	// The unknown node keeps its config verbatim.
	unk := nodeByType(w, "fancy-widget")
	if unk == nil {
		t.Fatal("unknown node missing after import")
	}
	if unk.GetData()["conf"] != float64(1) {
		t.Errorf("unknown node lost config: %v", unk.GetData())
	}
}

// TestNodeREDExportTypeMapping verifies the reverse table: engine
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
		t.Errorf("linkCall export still carries engine link field")
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

	// z -> module mapping (ids are re-minted on import; nodes are
	// located by type and their data.childFlow label survives verbatim).
	sw := nodeByType(w, "switch")
	inj := nodeByType(w, "inject")
	if sw == nil || inj == nil {
		t.Fatalf("switch or inject node missing after import")
	}
	if stringOf(sw.GetData()["childFlow"]) != "First" {
		t.Errorf("switch module = %q, want First", sw.GetData()["childFlow"])
	}
	if stringOf(inj.GetData()["childFlow"]) != "Second" {
		t.Errorf("inject module = %q, want Second", inj.GetData()["childFlow"])
	}
	debugs := nodesByType(w, "debug")
	if len(debugs) != 3 {
		t.Fatalf("expected 3 debug nodes, got %d", len(debugs))
	}
	mods := map[string]int{}
	for _, d := range debugs {
		mods[stringOf(d.GetData()["childFlow"])]++
	}
	if mods["First"] != 1 || mods["Second"] != 2 {
		t.Errorf("debug module spread = %v, want First:1 Second:2", mods)
	}

	// wires[i] -> output_{i+1} fan-out, including two targets on one
	// port. Edge targets are the remapped ids — resolve them and
	// assert the shape (1 target on port 1, 2 on port 2).
	swEdges := edgesFrom(w, sw.GetID())
	if len(swEdges["output_1"]) != 1 {
		t.Errorf("switch output_1 wires = %v, want 1 target", swEdges["output_1"])
	}
	if len(swEdges["output_2"]) != 2 {
		t.Errorf("switch output_2 wires = %v, want 2 targets", swEdges["output_2"])
	}
	for _, tid := range append(swEdges["output_1"], swEdges["output_2"]...) {
		target := nodeByID(w, tid)
		if target == nil || target.GetType() != "debug" {
			t.Errorf("switch edge target %q does not resolve to a debug node", tid)
		}
	}

	// Disabled flag preserved verbatim in data.
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
	lout, lin, dbg := nodeByType(w, "linkOut"), nodeByType(w, "linkIn"), nodeByType(w, "debug")
	if lout == nil || lin == nil || dbg == nil {
		t.Fatalf("link types not mapped: %+v", w.GetNodes())
	}
	// linkOut's links[0] is remapped to the linkIn node's NEW id.
	if got := arrayOf(lout.GetData()["links"]); len(got) != 1 || stringOf(got[0]) != lin.GetID() {
		t.Fatalf("linkOut links not preserved/remapped: %v", lout.GetData())
	}
	if got := edgesFrom(w, lin.GetID())["output_1"]; len(got) != 1 || got[0] != dbg.GetID() {
		t.Fatalf("linkIn downstream wiring missing: %v", edgesFrom(w, lin.GetID()))
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
	got := map[string]bool{}
	for _, it := range items {
		got[stringOf(it["type"])] = true
	}
	for _, wt := range []string{"http in", "http response", "mqtt out"} {
		if !got[wt] {
			t.Errorf("round-trip type %q missing from export", wt)
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

// TestNodeREDMQTTBrokerImport verifies a Node-RED mqtt-broker config
// node lands as the engine's mqttBroker type and that the referencing
// mqtt node's "broker" field is rewritten to the config node's NEW id
// (ids are re-minted on import).
func TestNodeREDMQTTBrokerImport(t *testing.T) {
	flowJSON := `[
		{"id":"tab1","type":"tab","label":"Main"},
		{"id":"brk1","type":"mqtt-broker","broker":"broker.local","port":"8883","usetls":true,"cleansession":false,"keepalive":"30","clientid":"shared"},
		{"id":"mi1","type":"mqtt in","z":"tab1","broker":"brk1","topic":"a/#","wires":[["mo1"]]},
		{"id":"mo1","type":"mqtt out","z":"tab1","broker":"brk1","topic":"a/b","wires":[[]]}
	]`

	w, err := ImportNodeRED([]byte(flowJSON))
	if err != nil {
		t.Fatalf("import failed: %v", err)
	}
	broker := nodeByType(w, "mqttBroker")
	if broker == nil {
		t.Fatal("mqtt-broker config node did not map to mqttBroker")
	}
	if stringOf(broker.GetData()["broker"]) != "broker.local" {
		t.Fatalf("broker address lost: %v", broker.GetData())
	}
	in := nodeByType(w, "mqttIn")
	out := nodeByType(w, "mqttOut")
	if in == nil || out == nil {
		t.Fatal("mqtt nodes missing after import")
	}
	if stringOf(in.GetData()["broker"]) != broker.GetID() {
		t.Fatalf("mqttIn broker ref not remapped to config node id: %v", in.GetData()["broker"])
	}
	if stringOf(out.GetData()["broker"]) != broker.GetID() {
		t.Fatalf("mqttOut broker ref not remapped to config node id: %v", out.GetData()["broker"])
	}
}

// TestNodeREDComponentInstanceBoundaryRoundTrip proves the semantic
// boundary mapping in both directions: external Node-RED subflow:<id>
// instances import as canonical component:<id> instances remapped to the
// re-minted definition id, and exports restore Node-RED's subflow:<id>
// spelling without leaking internal component types.
func TestNodeREDComponentInstanceBoundaryRoundTrip(t *testing.T) {
	flowJSON := `[
		{"id":"tab1","type":"tab","label":"Main"},
		{"id":"defX","type":"subflow","name":"Wrap","info":"",
		 "in":[{"x":40,"y":40,"wires":[{"id":"innerFn"}]}],
		 "out":[{"x":300,"y":40,"wires":[{"id":"innerFn","port":0}]}],
		 "env":[{"name":"GREETING","type":"str","value":"hello"}]},
		{"id":"innerFn","type":"function","z":"defX","x":170,"y":40,
		 "func":"return msg;","outputs":1,"wires":[[]]},
		{"id":"inj","type":"inject","z":"tab1","payload":"hi","payloadType":"str","wires":[["inst"]]},
		{"id":"inst","type":"subflow:defX","z":"tab1","name":"Wrap 1","wires":[["dbg"]]},
		{"id":"dbg","type":"debug","z":"tab1","active":true,"wires":[[]]}
	]`

	assertCanonical := func(w WorkflowInterface, phase string) {
		def := nodeByType(w, componentDefNodeType)
		if def == nil {
			t.Fatalf("%s: component definition missing", phase)
		}
		var inst NodeInterface
		for _, n := range w.GetNodes() {
			if strings.HasPrefix(n.GetType(), "subflow:") {
				t.Fatalf("%s: external instance type leaked into internal model: %q", phase, n.GetType())
			}
			if IsComponentInstance(n.GetType()) {
				inst = n
			}
		}
		if inst == nil {
			t.Fatalf("%s: component instance missing", phase)
		}
		if inst.GetType() != componentTypePrefix+def.GetID() {
			t.Fatalf("%s: instance type = %q, want %q", phase, inst.GetType(), componentTypePrefix+def.GetID())
		}
		if len(arrayOf(def.GetData()["nodes"])) != 1 {
			t.Fatalf("%s: definition internal nodes = %v", phase, def.GetData()["nodes"])
		}
	}

	w, err := ImportNodeRED([]byte(flowJSON))
	if err != nil {
		t.Fatalf("import failed: %v", err)
	}
	assertCanonical(w, "import")

	raw, err := ExportNodeRED(w)
	if err != nil {
		t.Fatalf("export failed: %v", err)
	}
	var items []map[string]any
	if err := json.Unmarshal(raw, &items); err != nil {
		t.Fatalf("export produced invalid JSON: %v", err)
	}
	defIDs := map[string]bool{}
	for _, it := range items {
		if stringOf(it["type"]) == "subflow" {
			defIDs[stringOf(it["id"])] = true
		}
	}
	if len(defIDs) != 1 {
		t.Fatalf("exported subflow definitions = %v", defIDs)
	}
	instances := 0
	for _, it := range items {
		typ := stringOf(it["type"])
		if typ == componentDefNodeType || strings.HasPrefix(typ, componentTypePrefix) {
			t.Fatalf("internal component type leaked into Node-RED export: %q", typ)
		}
		if strings.HasPrefix(typ, "subflow:") {
			id := strings.TrimPrefix(typ, "subflow:")
			if !defIDs[id] {
				t.Fatalf("exported instance references missing definition %q", id)
			}
			instances++
		}
	}
	if instances != 1 {
		t.Fatalf("exported subflow instances = %d, want 1", instances)
	}

	w2, err := ImportNodeRED(raw)
	if err != nil {
		t.Fatalf("re-import failed: %v", err)
	}
	assertCanonical(w2, "round-trip re-import")
}
