package nodestore

import "strings"

// This file contains read-time normalization for legacy module/subflow
// persisted vocabulary. The canonical write shape is component/child-flow;
// legacy keys are accepted here so old documents keep loading and are
// rewritten canonically on the next workflow update.

func normalizeLegacyNodeTerminology(node NodeInterface) {
	if node == nil {
		return
	}
	nodeType := node.GetType()
	switch {
	case nodeType == "subflow":
		node.SetType("component")
	case strings.HasPrefix(nodeType, "subflow:"):
		node.SetType("component:" + strings.TrimPrefix(nodeType, "subflow:"))
	}

	data := node.GetData()
	if data == nil {
		return
	}
	normalizeLegacyDataTerminology(data)
	node.SetData(data)
}

func normalizeLegacyDataTerminology(data map[string]any) {
	if _, ok := data["childFlow"]; !ok {
		if legacy, exists := data["module"]; exists {
			data["childFlow"] = legacy
			delete(data, "module")
		}
	}
	if _, ok := data["childFlowName"]; !ok {
		if legacy, exists := data["moduleName"]; exists {
			data["childFlowName"] = legacy
			delete(data, "moduleName")
		}
	}

	for _, raw := range arrayOf(data["nodes"]) {
		inner, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		if nodeType, ok := inner["type"].(string); ok {
			switch {
			case nodeType == "subflow":
				inner["type"] = "component"
			case strings.HasPrefix(nodeType, "subflow:"):
				inner["type"] = "component:" + strings.TrimPrefix(nodeType, "subflow:")
			case nodeType == "subflowIn":
				inner["type"] = "componentIn"
			case nodeType == "subflowOut":
				inner["type"] = "componentOut"
			}
		}
		if innerData, ok := inner["data"].(map[string]any); ok {
			normalizeLegacyDataTerminology(innerData)
		}
	}
}

func normalizeLegacyViewTerminology(view map[string]any) {
	if view == nil {
		return
	}
	if _, ok := view["childFlows"]; !ok {
		if legacy, exists := view["modules"]; exists {
			view["childFlows"] = legacy
			delete(view, "modules")
		}
	}
	if _, ok := view["disabledFlows"]; !ok {
		if legacy, exists := view["disabledModules"]; exists {
			view["disabledFlows"] = legacy
			delete(view, "disabledModules")
		}
	}

	for _, raw := range arrayOf(view["childFlows"]) {
		entity, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		if entity["role"] == "subflow" {
			entity["role"] = "component"
		}
	}
	for _, raw := range arrayOf(view["treeGroups"]) {
		group, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		if _, ok := group["childFlows"]; !ok {
			if legacy, exists := group["modules"]; exists {
				group["childFlows"] = legacy
				delete(group, "modules")
			}
		}
	}
	for _, raw := range arrayOf(view["groups"]) {
		group, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		if _, ok := group["childFlow"]; !ok {
			if legacy, exists := group["module"]; exists {
				group["childFlow"] = legacy
				delete(group, "module")
			}
		}
	}
}
