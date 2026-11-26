package nodestore

// Entity types stored in the underlying entity store
const EntityTypeCredential = "credential"
const EntityTypeDeployRecord = "deploy_record"
const EntityTypeEdge = "edge"
const EntityTypeMessage = "message"
const EntityTypeNode = "node"
const EntityTypeNodeGroup = "node_group"
const EntityTypeWorkflow = "workflow"
const EntityTypeWorkflowGroup = "workflow_group"
const EntityTypeContextValue = "context_value"

// Attribute keys (persisted in the entity store attributes table)
const ATTRIBUTE_ACTOR = "actor"
const ATTRIBUTE_DATA = "data"
const ATTRIBUTE_DEPLOYED = "deployed"
const ATTRIBUTE_DESCRIPTION = "description"
const ATTRIBUTE_ERROR = "error"
const ATTRIBUTE_FLOW = "flow"
const ATTRIBUTE_GROUP_ID = "group_id"
const ATTRIBUTE_METADATA = "metadata"
const ATTRIBUTE_NODE_ID = "node_id"
const ATTRIBUTE_PARENT_ID = "parent_id"
const ATTRIBUTE_NAME = "name"
const ATTRIBUTE_PAYLOAD = "payload"
const ATTRIBUTE_POSITION = "position"
const ATTRIBUTE_SOURCE = "source"
const ATTRIBUTE_SOURCE_KEY = "source_key"
const ATTRIBUTE_SCOPE = "scope"
const ATTRIBUTE_SCOPE_ID = "scope_id"
const ATTRIBUTE_CONTEXT_KEY = "context_key"
const ATTRIBUTE_TARGET = "target"
const ATTRIBUTE_TARGET_KEY = "target_key"
const ATTRIBUTE_TYPE = "type"
const ATTRIBUTE_VALUE = "value"
const ATTRIBUTE_VIEW = "view"
const ATTRIBUTE_WORKFLOWS = "workflows"
const ATTRIBUTE_WORKFLOW_ID = "workflow_id"

// Legacy attribute keys — never written anymore, only read as a
// fallback when loading pre-migration records and cleaned up by
// entityAttributesReplace (they are in each type's managed key set).
const ATTRIBUTE_EDGES = "edges"         // legacy: JSON blob on workflow entities
const ATTRIBUTE_NODES = "nodes"         // legacy: JSON blob on workflow entities
const ATTRIBUTE_TIMESTAMP = "timestamp" // legacy: duplicated entity created_at on messages

const ASC = "asc"
const DESC = "desc"

// datetimeLayout matches the entitystore timestamp format
// (carbon.Now(carbon.UTC).ToDateTimeString)
const datetimeLayout = "2006-01-02 15:04:05"

// EntityTypeConfig is the entity type for child-flow-level key/value config
const EntityTypeConfig = "config"

// AttributeConfigValue holds a config entry's raw string payload
const AttributeConfigValue = "value"
