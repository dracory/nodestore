# nodestore

Domain model and persistence for flow-based (Node-RED-style) workflows.
Everything crosses interfaces — consumers depend on the contracts,
never on the concrete `Store` or entity structs.

## What it provides

- `StoreInterface` — persistence contract for the whole domain, backed
  by `dracory/entitystore` (multi-database via `dracory/neat`: SQLite,
  Postgres, MySQL, …).
- `WorkflowInterface` — a workflow (flow) with its nodes, edges, view
  metadata, and deployed flag.
- `NodeInterface` — a node as a first-class entity: own ID and
  timestamps, `workflow_id` link, type, name, position, and a `Data`
  payload — never a JSON blob on the workflow.
- `EdgeInterface` — an edge as a first-class entity with its own ID,
  linking source node/port to target node.
- `MessageInterface` — the message envelope exchanged between nodes.
- `ContextStore` — node/flow/global context persistence.

Query builders (`WorkflowQuery`, `NodeQuery`, `EdgeQuery`,
`MessageQuery`) follow the same shape as taskstore/cmsstore: Interface
+ implementation + Query.

Also included:

- Node-RED flow JSON import/export (type-name mapping both ways)
- DOT/Graphviz workflow export via `ExportDOT(workflow, statuses)`: a pure,
  deterministic text transformation of the domain model with no I/O or
  rendering dependency. Pass `nil` for a structural graph, or a
  node-ID-to-status map to color nodes by runtime status.
- Component/child-flow composition helpers
- Key/value config store (global env, editor settings)

## Usage

```go
store, err := nodestore.NewStore(nodestore.NewStoreOptions{
    EntityStore: entityStore, // entitystore.StoreInterface (required)
})

wf := nodestore.NewWorkflow("my flow")
err = store.WorkflowCreate(ctx, wf)
```

## Ecosystem

| Repo | Role |
|---|---|
| `nodebase` | Engine-agnostic node executors + the `NodeHost` contract |
| `noderun` | The reference workflow engine; implements `NodeHost` |
| `nodeadmin` | Mountable admin HTTP/UI |

Node executors embed `nodestore.NodeInterface` — identity, type and
`Data` live on the node entity, not on a parallel base struct.

## Rules

- Timestamps come from the entitystore entity (`created_at`,
  `updated_at`) — no duplicate timestamp fields or attributes.
- Import direction: engine (`noderun`) and admin (`nodeadmin`) may
  import `nodestore`, never the reverse.
