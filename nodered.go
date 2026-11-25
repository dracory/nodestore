package nodestore

import (
	"encoding/json"
	"fmt"
	"hash/fnv"
	"log"
	"sort"
	"strings"
	"uuid"
)

// This file converts between the engine's {nodes, edges} workflow shape
// and the Node-RED flow JSON array. Node-RED stores a flow file as a
// flat array of objects:
//
//   - {id, type:"tab", label}                    — flow tabs (modules)
//   - {id, type:"subflow", name, in, out, env}   — subflow definitions
//   - {id, type, z, x, y, wires, ...config}      — regular nodes;
//     z is the owning tab or subflow id, wires[i] is the list of
//     target node ids fed by output port i+1
//
// The engine keeps subflow definitions as a node of type "subflow" whose
// Data carries in/out/env plus "nodes"/"edges" for the internal
// graph; Node-RED instead stores the internal nodes flat in the array
// with z = <defId>. ImportNodeRED folds those in; ExportNodeRED
// unfolds them back out so a Node-RED export round-trips.

// generateID generates a unique ID for workflow elements (RFC 9562
// UUIDv7 — time-ordered, random suffix, same role as Node-RED's random
// hex ids).
func generateID() string {
	return uuid.NewV7().String()
}

// nodeREDTabMap maps an engine module name to the object id of the tab
// created for it on export.
type nodeREDTabMap map[string]string

// nodeREDTabID returns a deterministic tab id for a module name so
// repeated exports of the same workflow produce stable, diffable ids.
func nodeREDTabID(module string) string {
	h := fnv.New32a()
	_, _ = h.Write([]byte(module))
	return fmt.Sprintf("tab%08x", h.Sum32())
}

// nodeREDTypeToModule maps Node-RED core type names that differ from
// the engine's ids to their engine equivalents. Node-RED types already
// spelled the engine's way (inject, debug, function, switch, change,
// delay, template, join, split, batch, sort, range, rbe, trigger,
// exec, catch, status, complete, junction, json, xml, csv, yaml,
// file) need no entry — they import verbatim. Types with no engine
// equivalent stay untouched and land as unknown nodes.
var nodeREDTypeToModule = map[string]string{
	"http in":       "httpTrigger",
	"http response": "httpResponse",
	"http request":  "httpRequest",
	"link in":       "linkIn",
	"link out":      "linkOut",
	"link call":     "linkCall",
	"mqtt in":       "mqttIn",
	"mqtt out":      "mqttOut",
	"mqtt-broker":   "mqttBroker",
	"tcp in":        "tcpIn",
	"tcp out":       "tcpOut",
	"tcp request":   "tcpRequest",
	"tls-config":    "tlsConfig",
	"udp in":        "udpIn",
	"udp out":       "udpOut",
	"websocket in":  "websocketIn",
	"websocket out": "websocketOut",
	"file in":       "fileIn",
}

// moduleTypeToNodeRED is the inverse of nodeREDTypeToModule, used on
// export so engine nodes emit the Node-RED spellings.
var moduleTypeToNodeRED = func() map[string]string {
	m := make(map[string]string, len(nodeREDTypeToModule))
	for nr, mod := range nodeREDTypeToModule {
		m[mod] = nr
	}
	return m
}()

// importType maps a Node-RED type name to the engine executor type;
// unmapped names pass through verbatim (unknown-node handling).
func importType(nrType string) string {
	if mod, ok := nodeREDTypeToModule[nrType]; ok {
		return mod
	}
	return nrType
}

// exportType maps an engine executor type back to the Node-RED spelling;
// unmapped names pass through verbatim.
func exportType(modType string) string {
	if nr, ok := moduleTypeToNodeRED[modType]; ok {
		return nr
	}
	return modType
}

// importLinkCallData translates the Node-RED "link call" config shape —
// the target linkIn id lives in links[0] — into the engine's linkCall
// "link" field. Other fields pass through unchanged.
func importLinkCallData(data map[string]any) {
	if stringOf(data["link"]) == "" {
		if links := arrayOf(data["links"]); len(links) > 0 {
			data["link"] = stringOf(links[0])
		}
	}
	delete(data, "links")
}

// exportLinkCallData rewrites an emitted "link call" object's engine
// "link" field back into Node-RED's links array.
func exportLinkCallData(obj map[string]any) {
	if link := stringOf(obj["link"]); link != "" {
		obj["links"] = []any{link}
	}
	delete(obj, "link")
}

// ImportNodeRED converts a Node-RED flow JSON array into a workflow.
// Regular nodes keep their config keys verbatim in Data (engine node
// configs use the Node-RED field names); tab membership becomes
// Data["module"]; wires become edges. Subflow definitions and their
// internal nodes fold into a single type:"subflow" node.
func ImportNodeRED(data []byte) (WorkflowInterface, error) {
	var items []map[string]any
	if err := json.Unmarshal(data, &items); err != nil {
		return nil, fmt.Errorf("invalid Node-RED flow JSON: %w", err)
	}

	tabs := map[string]string{} // tab id -> label
	tabDisabled := map[string]bool{}
	var tabEnv []any // merged env lists from tab objects
	var defObjs []map[string]any
	var groupObjs []map[string]any
	var nodeObjs []map[string]any
	for _, obj := range items {
		typ := stringOf(obj["type"])
		switch typ {
		case "tab":
			label := stringOf(obj["label"])
			if label == "" {
				label = "Main"
			}
			tabID := stringOf(obj["id"])
			tabs[tabID] = label
			if truthyFlag(obj["disabled"]) {
				tabDisabled[tabID] = true
			}
			// Node-RED stores flow env on the tab object. The engine
			// folds every tab into one workflow, so tab envs merge
			// into the workflow-level view.env (documented divergence:
			// per-tab scoping collapses to per-workflow).
			if env, ok := obj["env"].([]any); ok {
				tabEnv = append(tabEnv, env...)
			}
		case "subflow":
			defObjs = append(defObjs, obj)
		case "group":
			// Node-RED groups fold into the engine's view.groups model
			// (view boxes with member ids + an optional disabled flag).
			groupObjs = append(groupObjs, obj)
		case "global", "global-config":
			// Global config nodes are skipped; the global-config env
			// is a storage-level singleton that imports cannot reach
			// (documented divergence).
			continue
		default:
			nodeObjs = append(nodeObjs, obj)
		}
	}

	w := NewWorkflow("Imported flow")
	nodes := []NodeInterface{}
	edges := []EdgeInterface{}

	// Fold subflow definitions: internal nodes are the node objects
	// whose z matches the definition id; their wires become the
	// definition's internal edges.
	defIDs := map[string]bool{}
	for _, obj := range defObjs {
		defID := stringOf(obj["id"])
		if defID == "" {
			return nil, fmt.Errorf("subflow definition without id")
		}
		data := map[string]any{}
		for k, v := range obj {
			switch k {
			case "id", "type":
				continue
			}
			data[k] = v
		}
		var innerNodes []any
		var innerEdges []any
		for _, no := range nodeObjs {
			if stringOf(no["z"]) != defID {
				continue
			}
			inner, wires := nodeREDNodeToInternal(no)
			innerNodes = append(innerNodes, inner)
			innerEdges = append(innerEdges, wires...)
		}
		data["nodes"] = innerNodes
		data["edges"] = innerEdges
		nodes = append(nodes, NewNode(defID, "subflow").SetData(data))
		defIDs[defID] = true
	}

	// Regular nodes: anything whose z is a tab (or unset). Nodes
	// belonging to a subflow were consumed above; a z that is neither
	// a tab nor a definition is a dangling reference — import the
	// node anyway at top level and warn instead of dropping it.
	for _, no := range nodeObjs {
		z := stringOf(no["z"])
		if z != "" {
			if _, isTab := tabs[z]; !isTab {
				if defIDs[z] {
					continue // internal subflow node, already folded
				}
				log.Printf("ImportNodeRED: node %q has unknown z %q (not a tab or subflow); importing it as a top-level node", stringOf(no["id"]), z)
			}
		}
		node, nodeEdges := nodeREDToNode(no, tabs[z])
		nodes = append(nodes, node)
		edges = append(edges, nodeEdges...)
	}
	w.SetNodes(nodes)
	w.SetEdges(edges)

	// Merge tab envs into the workflow-level env table (view.env).
	// Names deduplicate later-wins, preserving first-seen order.
	if len(tabEnv) > 0 {
		var merged []any
		seen := map[string]int{}
		for _, raw := range tabEnv {
			m, ok := raw.(map[string]any)
			if !ok {
				continue
			}
			name := stringOf(m["name"])
			if idx, dup := seen[name]; dup && name != "" {
				merged[idx] = raw
				continue
			}
			seen[name] = len(merged)
			merged = append(merged, raw)
		}
		if len(merged) > 0 {
			w.GetView()["env"] = merged
		}
	}

	// Disabled tabs map onto the engine's per-module disabled flag:
	// view.disabledModules lists the module references — labels here,
	// since import predates the id-keyed model; the editor migrates
	// them to ids on the next save, and the engine resolves either
	// form through view.modules.
	if len(tabDisabled) > 0 {
		var disabledModules []any
		for _, obj := range items {
			if stringOf(obj["type"]) == "tab" && tabDisabled[stringOf(obj["id"])] {
				disabledModules = append(disabledModules, tabs[stringOf(obj["id"])])
			}
		}
		w.GetView()["disabledModules"] = disabledModules
	}

	// Node-RED group objects become view.groups entries — the same
	// flat shape the editor persists ({id, module, name, x, y, w, h,
	// nodeIds, disabled}).
	if len(groupObjs) > 0 {
		var viewGroups []any
		for _, g := range groupObjs {
			var nodeIds []any
			for _, m := range arrayOf(g["nodes"]) {
				if id := stringOf(m); id != "" {
					nodeIds = append(nodeIds, id)
				}
			}
			entry := map[string]any{
				"id":      stringOf(g["id"]),
				"name":    stringOf(g["name"]),
				"x":       floatOf(g["x"]),
				"y":       floatOf(g["y"]),
				"w":       floatOf(g["w"]),
				"h":       floatOf(g["h"]),
				"nodeIds": nodeIds,
			}
			if mod := tabs[stringOf(g["z"])]; mod != "" {
				entry["module"] = mod
			}
			if truthyFlag(g["disabled"]) {
				entry["disabled"] = true
			}
			viewGroups = append(viewGroups, entry)
		}
		if len(viewGroups) > 0 {
			w.GetView()["groups"] = viewGroups
		}
	}

	// Imported ids come from an untrusted document — re-mint them all
	// so an import can never collide with (or target) existing
	// entities, and rewrite every intra-document reference to match.
	remapImportedWorkflowIDs(w)

	return w, nil
}

// remapImportedWorkflowIDs re-mints every id carried in from an
// imported document — regular nodes, subflow definitions and their
// internal graphs — and rewrites edges, node-id references, group
// membership and "subflow:<defID>" instance types to the new ids.
// External (dangling) references pass through untouched.
func remapImportedWorkflowIDs(w WorkflowInterface) {
	nodeIDs := map[string]string{}
	defIDs := map[string]string{}
	for _, n := range w.GetNodes() {
		if n.GetType() == subflowDefNodeType {
			defIDs[n.GetID()] = generateID()
		} else {
			nodeIDs[n.GetID()] = generateID()
		}
	}

	for _, n := range w.GetNodes() {
		if n.GetType() == subflowDefNodeType {
			innerIDs := map[string]string{}
			for _, rawNode := range arrayOf(n.GetData()["nodes"]) {
				if m, ok := rawNode.(map[string]any); ok {
					if id := stringOf(m["id"]); id != "" {
						innerIDs[id] = generateID()
					}
				}
			}
			n.SetID(defIDs[n.GetID()])
			n.SetData(rewriteDefData(n.GetData(), innerIDs, defIDs))
			continue
		}
		data := n.GetData()
		for _, key := range nodeRefKeys {
			if rv, ok := data[key]; ok {
				data[key] = remapIDRefs(rv, nodeIDs)
			}
		}
		if IsSubflowInstance(n.GetType()) {
			old := strings.TrimPrefix(n.GetType(), subflowTypePrefix)
			if mapped, ok := defIDs[old]; ok {
				n.SetType(subflowTypePrefix + mapped)
			}
		}
		n.SetID(nodeIDs[n.GetID()])
	}

	for _, e := range w.GetEdges() {
		if mapped, ok := nodeIDs[e.GetSource()]; ok {
			e.SetSource(mapped)
		}
		if mapped, ok := nodeIDs[e.GetTarget()]; ok {
			e.SetTarget(mapped)
		}
	}

	for _, g := range arrayOf(w.GetView()["groups"]) {
		gm, ok := g.(map[string]any)
		if !ok {
			continue
		}
		gm["id"] = generateID()
		var memberIDs []any
		for _, m := range arrayOf(gm["nodeIds"]) {
			if mapped, ok := nodeIDs[stringOf(m)]; ok {
				memberIDs = append(memberIDs, mapped)
			}
		}
		gm["nodeIds"] = memberIDs
	}
}

// nodeREDToNode converts one flat Node-RED node object into a backend
// node plus its outgoing edges. module is the owning tab label.
func nodeREDToNode(obj map[string]any, module string) (NodeInterface, []EdgeInterface) {
	id := stringOf(obj["id"])
	node := NewNode(id, importType(stringOf(obj["type"]))).
		SetPosition(floatOf(obj["x"]), floatOf(obj["y"]))
	data := node.GetData()
	for k, v := range obj {
		switch k {
		case "id", "type", "z", "x", "y", "wires":
			continue
		}
		data[k] = v
	}
	if node.GetType() == "linkCall" {
		importLinkCallData(data)
	}
	if module != "" {
		data["module"] = module
	}

	var edges []EdgeInterface
	for i, wires := range arrayOf(obj["wires"]) {
		for _, target := range arrayOf(wires) {
			tid := stringOf(target)
			if tid == "" {
				continue
			}
			edges = append(edges, NewEdge(generateID(), id, tid).
				SetSourceKey(fmt.Sprintf("output_%d", i+1)))
		}
	}
	return node, edges
}

// nodeREDNodeToInternal converts a Node-RED node inside a subflow
// (z = defId) into the inner-node shape stored in a definition's
// Data["nodes"], plus its internal edges.
func nodeREDNodeToInternal(obj map[string]any) (map[string]any, []any) {
	id := stringOf(obj["id"])
	inner := map[string]any{
		"id":   id,
		"type": importType(stringOf(obj["type"])),
		"position": map[string]any{
			"x": floatOf(obj["x"]),
			"y": floatOf(obj["y"]),
		},
		"data": map[string]any{},
	}
	data := inner["data"].(map[string]any)
	for k, v := range obj {
		switch k {
		case "id", "type", "z", "x", "y", "wires":
			continue
		}
		data[k] = v
	}
	if inner["type"] == "linkCall" {
		importLinkCallData(data)
	}

	var edges []any
	for i, wires := range arrayOf(obj["wires"]) {
		for _, target := range arrayOf(wires) {
			tid := stringOf(target)
			if tid == "" {
				continue
			}
			edges = append(edges, map[string]any{
				"id":        generateID(),
				"source":    id,
				"sourceKey": fmt.Sprintf("output_%d", i+1),
				"target":    tid,
			})
		}
	}
	return inner, edges
}

// ExportNodeRED converts a workflow into the Node-RED flow JSON array
// shape. Subflow definitions unfold: the type:"subflow" object is
// followed by its internal nodes as flat objects with z = <defId>.
func ExportNodeRED(w WorkflowInterface) ([]byte, error) {
	var items []map[string]any

	// Tabs: one per distinct module, stable order for diffability.
	// data.module stores the module id under the id-keyed model (a
	// name in legacy documents) — resolve to the display label, which
	// is what Node-RED tabs carry.
	tabs := nodeREDTabMap{}
	var moduleOrder []string
	for _, n := range w.GetNodes() {
		if n.GetType() == subflowDefNodeType {
			continue
		}
		mod := moduleLabel(w, stringOf(n.GetData()["module"]))
		if mod == "" {
			mod = "Main"
		}
		if _, ok := tabs[mod]; !ok {
			tabs[mod] = nodeREDTabID(mod)
			moduleOrder = append(moduleOrder, mod)
		}
	}
	sort.Strings(moduleOrder)
	// Node-RED stores flow env on the tab object and exports it —
	// mirror that by stamping the workflow env (view.env) onto every
	// emitted tab (all tabs fold back into one workflow on import).
	view := w.GetView()
	var flowEnv any
	if view != nil {
		if env, ok := view["env"].([]any); ok && len(env) > 0 {
			flowEnv = env
		}
	}
	disabledMods := map[string]bool{}
	if view != nil {
		for _, m := range arrayOf(view["disabledModules"]) {
			disabledMods[moduleLabel(w, stringOf(m))] = true
		}
		// Module entities flagged enabled:false count as disabled too.
		for _, m := range arrayOf(view["modules"]) {
			if mm, ok := m.(map[string]any); ok {
				if en, isBool := mm["enabled"].(bool); isBool && !en {
					disabledMods[stringOf(mm["label"])] = true
				}
			}
		}
	}
	for _, mod := range moduleOrder {
		tab := map[string]any{
			"id":    tabs[mod],
			"type":  "tab",
			"label": mod,
		}
		if flowEnv != nil {
			tab["env"] = flowEnv
		}
		// A module listed in view.disabledModules exports as a
		// Node-RED disabled tab (disabled flow).
		if disabledMods[mod] {
			tab["disabled"] = true
		}
		items = append(items, tab)
	}

	// View groups export as Node-RED group objects (z = owning tab).
	if view != nil {
		for _, g := range arrayOf(view["groups"]) {
			gm, ok := g.(map[string]any)
			if !ok {
				continue
			}
			mod := moduleLabel(w, stringOf(gm["module"]))
			if mod == "" {
				mod = "Main"
			}
			zid := tabs[mod]
			if zid == "" {
				continue // group whose module has no nodes — no tab to hang it on
			}
			obj := map[string]any{
				"id":    stringOf(gm["id"]),
				"type":  "group",
				"z":     zid,
				"name":  stringOf(gm["name"]),
				"x":     floatOf(gm["x"]),
				"y":     floatOf(gm["y"]),
				"w":     floatOf(gm["w"]),
				"h":     floatOf(gm["h"]),
				"nodes": arrayOf(gm["nodeIds"]),
			}
			if truthyFlag(gm["disabled"]) {
				obj["disabled"] = true
			}
			items = append(items, obj)
		}
	}

	wiresOf := func(nodeID string, edges []EdgeInterface) [][]any {
		// Rebuild per-port wire lists from outgoing edges.
		maxPort := 0
		for _, e := range edges {
			if e.GetSource() != nodeID {
				continue
			}
			if p := outputPortIndex(e.GetSourceKey()) + 1; p > maxPort {
				maxPort = p
			}
		}
		wires := make([][]any, maxPort)
		for _, e := range edges {
			if e.GetSource() != nodeID {
				continue
			}
			p := outputPortIndex(e.GetSourceKey())
			wires[p] = append(wires[p], e.GetTarget())
		}
		return wires
	}

	emitNode := func(obj map[string]any, n NodeInterface, z string) {
		for k, v := range n.GetData() {
			// Credential fields never leave the server (Node-RED strips
			// them from exported flows); sentinel markers go too.
			if k == "module" || k == "moduleName" || k == subflowEnvKey ||
				IsCredentialKey(n.GetType(), k) || IsCredentialSentinel(v) {
				continue
			}
			obj[k] = v
		}
		obj["id"] = n.GetID()
		obj["type"] = exportType(n.GetType())
		if z != "" {
			obj["z"] = z
		}
		obj["x"] = n.GetPositionX()
		obj["y"] = n.GetPositionY()
		if n.GetType() == "linkCall" {
			exportLinkCallData(obj)
		}
	}

	for _, n := range w.GetNodes() {
		if n.GetType() == subflowDefNodeType {
			def := map[string]any{
				"id":   n.GetID(),
				"type": "subflow",
			}
			var innerNodes []any
			var innerEdges []EdgeInterface
			for k, v := range n.GetData() {
				switch k {
				case "nodes":
					innerNodes, _ = v.([]any)
				case "edges":
					raw, _ := json.Marshal(v)
					var rawEdges []edgeJSON
					_ = json.Unmarshal(raw, &rawEdges)
					for _, je := range rawEdges {
						innerEdges = append(innerEdges, edgeFromJSON(je))
					}
				default:
					def[k] = v
				}
			}
			items = append(items, def)
			// Unfold internal nodes: z = definition id.
			for _, raw := range innerNodes {
				m, ok := raw.(map[string]any)
				if !ok {
					continue
				}
				inner, err := parseSubflowInnerNode(mustJSON(m))
				if err != nil {
					continue
				}
				obj := map[string]any{}
				for k, v := range inner.GetData() {
					if k == subflowEnvKey ||
						IsCredentialKey(inner.GetType(), k) || IsCredentialSentinel(v) {
						continue
					}
					obj[k] = v
				}
				obj["id"] = inner.GetID()
				obj["type"] = exportType(inner.GetType())
				obj["z"] = n.GetID()
				obj["x"] = inner.GetPositionX()
				obj["y"] = inner.GetPositionY()
				if inner.GetType() == "linkCall" {
					exportLinkCallData(obj)
				}
				obj["wires"] = wiresOf(inner.GetID(), innerEdges)
				items = append(items, obj)
			}
			continue
		}

		mod := moduleLabel(w, stringOf(n.GetData()["module"]))
		if mod == "" {
			mod = "Main"
		}
		obj := map[string]any{}
		emitNode(obj, n, tabs[mod])
		obj["wires"] = wiresOf(n.GetID(), w.GetEdges())
		items = append(items, obj)
	}

	return json.Marshal(items)
}

// parseSubflowInnerNode decodes one internal node of a definition,
// accepting both the backend ({position, data}) and the Node-RED flat
// ({x, y, ...config}) shapes.
func parseSubflowInnerNode(raw json.RawMessage) (NodeInterface, error) {
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, err
	}
	node := NewNode(stringOf(m["id"]), stringOf(m["type"]))
	if node.GetID() == "" {
		return nil, fmt.Errorf("internal node without id")
	}
	if pos, ok := m["position"].(map[string]any); ok {
		node.SetPosition(floatOf(pos["x"]), floatOf(pos["y"]))
	} else {
		node.SetPosition(floatOf(m["x"]), floatOf(m["y"]))
	}
	if data, ok := m["data"].(map[string]any); ok {
		node.SetData(data)
	} else {
		// Node-RED flat shape: every key other than the structural
		// ones is node configuration.
		data := map[string]any{}
		for k, v := range m {
			switch k {
			case "id", "type", "x", "y", "z", "wires", "position", "data":
				continue
			}
			data[k] = v
		}
		node.SetData(data)
	}
	return node, nil
}

// arrayOf coerces a decoded JSON value to []any.
func arrayOf(v any) []any {
	arr, _ := v.([]any)
	return arr
}

// mustJSON marshals v to JSON, ignoring errors (best-effort copy).
func mustJSON(v any) json.RawMessage {
	raw, _ := json.Marshal(v)
	return raw
}
