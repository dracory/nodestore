package nodestore

import (
	"time"

	"github.com/dracory/entitystore"
)

// == INTERFACE ===============================================================

// DeployRecordInterface is one immutable entry in a workflow's deploy
// history: who deployed (actor) and the exact flow document deployed
// (flow, raw JSON — credential fields carry sentinel markers, never
// secrets). The deploy timestamp comes from the entity's created_at —
// per the GOD ruling no duplicated time attributes exist.
type DeployRecordInterface interface {
	GetID() string
	SetID(id string) DeployRecordInterface

	GetWorkflowID() string
	SetWorkflowID(workflowID string) DeployRecordInterface

	GetActor() string
	SetActor(actor string) DeployRecordInterface

	// GetFlow returns the deployed flow document as raw JSON.
	GetFlow() string
	SetFlow(flow string) DeployRecordInterface

	GetCreatedAt() time.Time
	SetCreatedAt(createdAt time.Time) DeployRecordInterface
}

// == IMPLEMENTATION ==========================================================

// DeployRecordRetention caps the deploy history kept per workflow —
// the last N records survive, older ones are pruned on append (same
// cap flowadmin used).
const DeployRecordRetention = 20

var deployRecordManagedAttributes = map[string]bool{
	ATTRIBUTE_WORKFLOW_ID: true,
	ATTRIBUTE_ACTOR:       true,
	ATTRIBUTE_FLOW:        true,
}

type deployRecordImplementation struct {
	entity     entitystore.EntityInterface
	workflowID string
	actor      string
	flow       string
}

// NewDeployRecord creates a new deploy record entity
func NewDeployRecord(workflowID, actor, flow string) DeployRecordInterface {
	entity := entitystore.NewEntity()
	entity.SetType(EntityTypeDeployRecord)
	return &deployRecordImplementation{
		entity:     entity,
		workflowID: workflowID,
		actor:      actor,
		flow:       flow,
	}
}

func deployRecordFromEntity(entity entitystore.EntityInterface, attrs map[string]string) DeployRecordInterface {
	return &deployRecordImplementation{
		entity:     entity,
		workflowID: attrs[ATTRIBUTE_WORKFLOW_ID],
		actor:      attrs[ATTRIBUTE_ACTOR],
		flow:       attrs[ATTRIBUTE_FLOW],
	}
}

func (d *deployRecordImplementation) attributes() map[string]string {
	return map[string]string{
		ATTRIBUTE_WORKFLOW_ID: d.workflowID,
		ATTRIBUTE_ACTOR:       d.actor,
		ATTRIBUTE_FLOW:        d.flow,
	}
}

func (d *deployRecordImplementation) GetID() string { return d.entity.GetID() }
func (d *deployRecordImplementation) SetID(id string) DeployRecordInterface {
	d.entity.SetID(id)
	return d
}

func (d *deployRecordImplementation) GetWorkflowID() string { return d.workflowID }
func (d *deployRecordImplementation) SetWorkflowID(workflowID string) DeployRecordInterface {
	d.workflowID = workflowID
	return d
}

func (d *deployRecordImplementation) GetActor() string { return d.actor }
func (d *deployRecordImplementation) SetActor(actor string) DeployRecordInterface {
	d.actor = actor
	return d
}

func (d *deployRecordImplementation) GetFlow() string { return d.flow }
func (d *deployRecordImplementation) SetFlow(flow string) DeployRecordInterface {
	d.flow = flow
	return d
}

func (d *deployRecordImplementation) GetCreatedAt() time.Time {
	t, _ := parseDatetime(d.entity.GetCreatedAt())
	return t
}
func (d *deployRecordImplementation) SetCreatedAt(createdAt time.Time) DeployRecordInterface {
	d.entity.SetCreatedAt(formatDatetime(createdAt))
	return d
}
