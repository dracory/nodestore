package nodestore

import (
	"context"
	"errors"
	"sort"

	"github.com/dracory/entitystore"
)

func (st *Store) WorkflowCount(ctx context.Context, query WorkflowQueryInterface) (int64, error) {
	if query == nil {
		return 0, errors.New("workflow count: query is nil")
	}
	// entitystore can only count on entity columns; the deployed filter is
	// an attribute, so counting with it still requires loading the list
	// (documented limitation — same as the workflow_id counts).
	if query.HasDeployed() || query.HasPaused() {
		list, err := st.WorkflowList(ctx, query.WithCountOnly(true))
		if err != nil {
			return 0, err
		}
		return int64(len(list)), nil
	}
	entityQuery := entitystore.EntityQuery().WithEntityType(EntityTypeWorkflow)
	if query.HasID() {
		entityQuery.WithID(query.GetID())
	}
	if query.HasName() {
		entityQuery.WithEntityHandle(query.GetName())
	}
	return st.entityStore.EntityCount(ctx, entityQuery)
}

func (st *Store) WorkflowCreate(ctx context.Context, workflow WorkflowInterface) error {
	if workflow == nil {
		return errors.New("workflow create: workflow is nil")
	}
	if err := validateWorkflowCredentialSentinels(workflow); err != nil {
		return err
	}
	impl, ok := workflow.(*workflowImplementation)
	if !ok {
		return errors.New("workflow create: unsupported implementation")
	}
	attrs, err := impl.attributes()
	if err != nil {
		return err
	}
	// Validate children BEFORE persisting anything so a foreign
	// implementation errors loudly instead of being silently dropped.
	nodes, edges, err := workflowChildren(impl)
	if err != nil {
		return err
	}
	if err := st.entityStore.EntityCreate(ctx, impl.entity); err != nil {
		return err
	}
	if err := st.entityStore.AttributesSet(ctx, impl.entity.GetID(), attrs); err != nil {
		return err
	}
	if err := st.persistWorkflowChildren(ctx, impl.entity.GetID(), nodes, edges, nil, nil); err != nil {
		return err
	}
	// P5: view.groups → node_group entities (entities are the
	// authoritative store; the blob key is stripped by attributes()).
	return st.syncWorkflowGroups(ctx, impl)
}

// WorkflowDeleteByID deletes the workflow AND its child node/edge
// entities plus workflow-owned runtime context (cascade — documented
// behavior; children and owned state are never orphaned). Context
// values with no workflow owner, including global context, are preserved.
func (st *Store) WorkflowDeleteByID(ctx context.Context, id string) error {
	unlock := st.lockWorkflow(id)
	defer unlock()
	if err := st.deleteWorkflowChildren(ctx, id); err != nil {
		return err
	}
	if err := st.ContextValueDeleteByWorkflowID(ctx, id); err != nil {
		return err
	}
	_, err := st.entityStore.EntityDelete(ctx, id)
	return err
}

func (st *Store) WorkflowFindByID(ctx context.Context, id string) (WorkflowInterface, error) {
	entity, err := st.entityStore.EntityFindByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if entity == nil || entity.GetType() != EntityTypeWorkflow {
		return nil, nil
	}
	return st.workflowFromEntity(ctx, entity)
}

func (st *Store) WorkflowFindByName(ctx context.Context, name string) (WorkflowInterface, error) {
	entity, err := st.entityStore.EntityFindByHandle(ctx, EntityTypeWorkflow, unifyName(name))
	if err != nil {
		return nil, err
	}
	if entity == nil {
		return nil, nil
	}
	return st.workflowFromEntity(ctx, entity)
}

// WorkflowList lists workflows. entitystore can only filter on entity
// columns (type/handle/dates), so attribute criteria are applied in
// memory, then offset/limit. Children (nodes/edges) are loaded eagerly
// per workflow — see workflowFromEntity.
func (st *Store) WorkflowList(ctx context.Context, query WorkflowQueryInterface) ([]WorkflowInterface, error) {
	if query == nil {
		query = WorkflowQuery()
	}
	if err := query.Validate(); err != nil {
		return nil, err
	}

	entityQuery := entitystore.EntityQuery().WithEntityType(EntityTypeWorkflow)
	if query.HasID() {
		entityQuery.WithID(query.GetID())
	}
	if query.HasName() {
		entityQuery.WithEntityHandle(query.GetName())
	}
	entityQuery.WithPrefetchAttributes()

	entities, err := st.entityStore.EntityList(ctx, entityQuery)
	if err != nil {
		return nil, err
	}

	matched := make([]WorkflowInterface, 0, len(entities))
	for _, entity := range entities {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		workflow, err := st.workflowFromEntityWithAttrs(ctx, entity, entity.GetTempKeys())
		if err != nil {
			return nil, err
		}
		if query.HasDeployed() && workflow.GetDeployed() != query.GetDeployed() {
			continue
		}
		if query.HasPaused() && workflow.GetPaused() != query.GetPaused() {
			continue
		}
		matched = append(matched, workflow)
	}

	sort.SliceStable(matched, func(i, j int) bool {
		if query.GetSortOrder() == DESC {
			return matched[i].GetCreatedAt().After(matched[j].GetCreatedAt())
		}
		return matched[i].GetCreatedAt().Before(matched[j].GetCreatedAt())
	})

	if query.HasOffset() && query.GetOffset() < uint64(len(matched)) {
		matched = matched[query.GetOffset():]
	}
	if query.HasLimit() && query.GetLimit() < uint64(len(matched)) {
		matched = matched[:query.GetLimit()]
	}
	return matched, nil
}

// WorkflowTrashByID soft-deletes a workflow into the entitystore trash
// (children are left in place — restore semantics, same as before)
func (st *Store) WorkflowTrashByID(ctx context.Context, id string) error {
	_, err := st.entityStore.EntityTrash(ctx, id)
	return err
}

func (st *Store) WorkflowUpdate(ctx context.Context, workflow WorkflowInterface) error {
	if workflow == nil {
		return errors.New("workflow update: workflow is nil")
	}
	if err := validateWorkflowCredentialSentinels(workflow); err != nil {
		return err
	}
	unlock := st.lockWorkflow(workflow.GetID())
	defer unlock()
	impl, ok := workflow.(*workflowImplementation)
	if !ok {
		return errors.New("workflow update: unsupported implementation")
	}
	attrs, err := impl.attributes()
	if err != nil {
		return err
	}
	nodes, edges, err := workflowChildren(impl)
	if err != nil {
		return err
	}

	// Snapshot existing child IDs so removed children get deleted and
	// unchanged ones get updated (create new / update changed / delete
	// removed). entitystore has no multi-entity transaction; each child
	// is persisted independently — same granularity the store offers.
	existingNodes, existingEdges, err := st.workflowChildIDs(ctx, impl.entity.GetID())
	if err != nil {
		return err
	}

	if err := st.entityStore.EntityUpdate(ctx, impl.entity); err != nil {
		return err
	}
	if err := st.entityAttributesReplace(ctx, impl.entity.GetID(), attrs, workflowManagedAttributes); err != nil {
		return err
	}
	if err := st.persistWorkflowChildren(ctx, impl.entity.GetID(), nodes, edges, existingNodes, existingEdges); err != nil {
		return err
	}
	// P5: view.groups → node_group entities (see WorkflowCreate).
	return st.syncWorkflowGroups(ctx, impl)
}

// workflowChildren extracts the concrete implementations from a
// workflow's node/edge slices. Foreign implementations are a loud
// error — they can never be silently dropped (the old blob path did).
func workflowChildren(w *workflowImplementation) ([]*nodeImplementation, []*edgeImplementation, error) {
	nodes := make([]*nodeImplementation, 0, len(w.nodes))
	for _, n := range w.nodes {
		impl, ok := n.(*nodeImplementation)
		if !ok {
			return nil, nil, errors.New("workflow children: unsupported NodeInterface implementation — use nodestore.NewNode")
		}
		nodes = append(nodes, impl)
	}
	edges := make([]*edgeImplementation, 0, len(w.edges))
	for _, e := range w.edges {
		impl, ok := e.(*edgeImplementation)
		if !ok {
			return nil, nil, errors.New("workflow children: unsupported EdgeInterface implementation — use nodestore.NewEdge")
		}
		edges = append(edges, impl)
	}
	return nodes, edges, nil
}

// workflowChildIDs returns the IDs of the currently persisted node and
// edge entities for a workflow.
func (st *Store) workflowChildIDs(ctx context.Context, workflowID string) (map[string]bool, map[string]bool, error) {
	nodeIDs := map[string]bool{}
	nodes, err := st.NodeList(ctx, NodeQuery().WithWorkflowID(workflowID))
	if err != nil {
		return nil, nil, err
	}
	for _, n := range nodes {
		nodeIDs[n.GetID()] = true
	}
	edgeIDs := map[string]bool{}
	edges, err := st.EdgeList(ctx, EdgeQuery().WithWorkflowID(workflowID))
	if err != nil {
		return nil, nil, err
	}
	for _, e := range edges {
		edgeIDs[e.GetID()] = true
	}
	return nodeIDs, edgeIDs, nil
}

// persistWorkflowChildren creates new child entities, updates existing
// ones and deletes children missing from the desired set (when the
// existing ID sets are provided — nil means "create all", i.e. create).
func (st *Store) persistWorkflowChildren(ctx context.Context, workflowID string,
	nodes []*nodeImplementation, edges []*edgeImplementation,
	existingNodeIDs, existingEdgeIDs map[string]bool) error {

	for _, node := range nodes {
		node.SetWorkflowID(workflowID)
		var err error
		if existingNodeIDs != nil && existingNodeIDs[node.GetID()] {
			err = st.NodeUpdate(ctx, node)
			delete(existingNodeIDs, node.GetID())
		} else {
			err = st.NodeCreate(ctx, node)
		}
		if err != nil {
			return err
		}
	}
	for _, edge := range edges {
		edge.SetWorkflowID(workflowID)
		var err error
		if existingEdgeIDs != nil && existingEdgeIDs[edge.GetID()] {
			err = st.EdgeUpdate(ctx, edge)
			delete(existingEdgeIDs, edge.GetID())
		} else {
			err = st.EdgeCreate(ctx, edge)
		}
		if err != nil {
			return err
		}
	}
	// Anything left in the existing sets was removed from the workflow.
	for id := range existingNodeIDs {
		if err := st.NodeDeleteByID(ctx, id); err != nil {
			return err
		}
	}
	for id := range existingEdgeIDs {
		if err := st.EdgeDeleteByID(ctx, id); err != nil {
			return err
		}
	}
	return nil
}

// deleteWorkflowChildren removes all node/edge entities of a workflow.
func (st *Store) deleteWorkflowChildren(ctx context.Context, workflowID string) error {
	nodes, err := st.NodeList(ctx, NodeQuery().WithWorkflowID(workflowID))
	if err != nil {
		return err
	}
	for _, n := range nodes {
		if err := st.NodeDeleteByID(ctx, n.GetID()); err != nil {
			return err
		}
	}
	edges, err := st.EdgeList(ctx, EdgeQuery().WithWorkflowID(workflowID))
	if err != nil {
		return err
	}
	for _, e := range edges {
		if err := st.EdgeDeleteByID(ctx, e.GetID()); err != nil {
			return err
		}
	}
	return nil
}

// loadWorkflowChildren populates w.nodes/w.edges from child entities
// (eager load-on-read: children are fetched once when the workflow is
// loaded and cached on the object — GetNodes()/GetEdges() never
// re-query, so engine hot paths are safe). Legacy JSON blob attributes
// parsed by workflowFromEntity remain as fallback only when the
// workflow has no child entities yet (unmigrated records).
func (st *Store) loadWorkflowChildren(ctx context.Context, w *workflowImplementation) error {
	nodes, err := st.NodeList(ctx, NodeQuery().WithWorkflowID(w.GetID()))
	if err != nil {
		return err
	}
	if len(nodes) > 0 {
		w.nodes = nodes
	}
	edges, err := st.EdgeList(ctx, EdgeQuery().WithWorkflowID(w.GetID()))
	if err != nil {
		return err
	}
	if len(edges) > 0 {
		w.edges = edges
	}
	return nil
}

func (st *Store) workflowFromEntity(ctx context.Context, entity entitystore.EntityInterface) (WorkflowInterface, error) {
	attrs, err := st.entityAttributes(ctx, entity.GetID())
	if err != nil {
		return nil, err
	}
	return st.workflowFromEntityWithAttrs(ctx, entity, attrs)
}

func (st *Store) workflowFromEntityWithAttrs(ctx context.Context, entity entitystore.EntityInterface, attrs map[string]string) (WorkflowInterface, error) {
	workflow, err := workflowFromEntity(entity, attrs)
	if err != nil {
		return nil, err
	}
	impl := workflow.(*workflowImplementation)
	if err := st.loadWorkflowChildren(ctx, impl); err != nil {
		return nil, err
	}
	// P5: view["groups"] is served from node_group entities; the blob
	// (if still present on an unmigrated record) is the fallback.
	if err := st.loadWorkflowGroups(ctx, impl); err != nil {
		return nil, err
	}
	return workflow, nil
}
