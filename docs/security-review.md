# Security Review Report
Date: 2026-11-24
Reviewer: Senior Principal Golang Engineer
Codebase: nodestore (github.com/dracory/nodestore) — entitystore-backed persistence layer for GoFlow workflows, nodes, edges, messages, credentials and deploy records

## Executive Summary

nodestore is a storage library, not a network-facing service: it exposes no HTTP endpoints, performs no authentication, and delegates all SQL execution to `entitystore` (which uses parameterized queries via its query builder — no raw SQL string concatenation was found). The classic injection surface (SQLi, command injection, XSS, CSRF, CORS) is therefore largely out of scope by design.

The material risks concentrate on **secret handling and data integrity**:

- Credentials are persisted as **plaintext JSON** with no encryption at rest, key management, or access auditing.
- The deploy-record and credential-injection paths rely on caller discipline to keep real secrets out of stored documents; nothing inside the library enforces that contract.
- Multi-entity write operations are **not transactional**, so crashes or races can leave orphaned children, half-replaced credentials, or lost deploy history.
- Import paths (`ImportNodeRED`, `ImportModule`) parse unbounded, attacker-controllable JSON.

Overall risk level: **Medium-High** for deployments that store real secrets; the architecture intentionally mimics Node-RED's `flows_cred.json` model but, unlike Node-RED, currently offers no `credentialSecret`-style encryption layer.

## Critical Findings (Severity: Critical)

None.

## High Severity Findings

### Finding 1: Credentials stored unencrypted at rest
- **Location**: `credential.go:91-101`, `store_credential.go:29-45`
- **Description**: Credential values (usernames, passwords for `httpRequest`, `mqttIn`, `mailIn`, `queue`, `fsIn`, `fsOut`, etc.) are serialized as a plaintext JSON map into the `data` attribute of `credential` entities. Anyone with read access to the SQLite file, a database backup, a dump, or any raw `entitystore` query obtains every stored secret in clear text.
- **Impact**: CWE-312 (Cleartext Storage of Sensitive Information). Full disclosure of all downstream-system credentials (SMTP, MQTT, HTTP basic-auth, queues, filesystem services) if the DB file is exfiltrated, backed up insecurely, or accessed by another process. Also enables silent credential harvesting by any other module sharing the same entitystore.
- **Recommendation**: Encrypt the `data` attribute before persisting. Use AEAD (AES-256-GCM or `chacha20poly1305`) with a caller-supplied master key (Node-RED's `credentialSecret` model). Keep the plaintext `map[string]string` API but encrypt inside `attributes()` / decrypt inside `credentialFromEntity()`. Store a key-version marker so keys can be rotated. At minimum, document prominently that the store is unencrypted so deployers can apply filesystem-level encryption.
- **Code Example**:
```go
// credential.go:91 — plaintext secrets into a plain-text column
func (c *credentialImplementation) attributes() (map[string]string, error) {
	data, err := json.Marshal(c.values)   // {"password":"hunter2"} in cleartext
	...
	return map[string]string{
		...
		ATTRIBUTE_DATA: string(data),      // persisted verbatim
	}, nil
}
```
- **Suggested Fix**:
```go
func (c *credentialImplementation) attributes(enc CredentialCipher) (map[string]string, error) {
	data, err := json.Marshal(c.values)
	if err != nil {
		return nil, err
	}
	sealed, err := enc.Encrypt(data)      // AES-256-GCM, random nonce per write
	if err != nil {
		return nil, err
	}
	return map[string]string{
		ATTRIBUTE_WORKFLOW_ID: c.workflowID,
		ATTRIBUTE_NODE_ID:     c.nodeID,
		ATTRIBUTE_DATA:        sealed,     // "v1:<base64 nonce+ciphertext>"
	}, nil
}
```

### Finding 2: `DeployRecordCreate` persists the flow document without verifying it is credential-scrubbed
- **Location**: `store_deploy_record.go:31-45`, `deploy_record.go:45-68`
- **Description**: `DeployRecord.flow` stores "the exact flow document deployed" as raw JSON. The contract that it contains "sentinel markers, never secrets" is documented in a comment but **not enforced**. If the caller snapshots the workflow after `ApplyWorkflowCredentials` has injected real values (the natural point at which "the deployed document" exists), every deploy record retains a full plaintext copy of all credentials — and up to `DeployRecordRetention` (20) historical copies survive.
- **Impact**: CWE-532 (Insertion of Sensitive Information into Log/File). A single caller mistake turns the deploy-history table into a durable secret store that is easy to overlook (it is not named "credential").
- **Recommendation**: Enforce the invariant inside the library. In `DeployRecordCreate`, run the flow JSON through the same scrubbing logic used by exports (reject or strip values matching `CredentialSentinel` violations for `CredentialKeys` of each node type), or return an error when a credential key carries a non-sentinel value.
- **Code Example**:
```go
// store_deploy_record.go — trusts the caller completely
if err := st.entityStore.EntityCreate(ctx, impl.entity); err != nil { ... }
if err := st.entityStore.AttributesSet(ctx, impl.entity.GetID(), impl.attributes()); err != nil { ... }
```
- **Suggested Fix**:
```go
func (st *Store) DeployRecordCreate(ctx context.Context, record DeployRecordInterface) error {
	// Reject snapshots that still carry real credential values.
	if leaked, key := flowContainsPlaintextCredential(record.GetFlow()); leaked {
		return fmt.Errorf("deploy record create: credential key %q contains a non-sentinel value", key)
	}
	... // existing persistence
}
```

### Finding 3: `ApplyWorkflowCredentials` mutates the workflow in place — secrets can be persisted by a later `WorkflowUpdate`
- **Location**: `credentials.go:164-186`
- **Description**: `ApplyWorkflowCredentials` writes real secret values into `node.Data` of the passed workflow object. The doc comment says callers "deploy a clone", and `DeepCloneWorkflow` exists for that purpose — but nothing prevents calling `ApplyWorkflowCredentials(w)` directly and then `WorkflowUpdate(ctx, w)`, which would write the real values into the stored `data` attribute, defeating the sentinel design entirely.
- **Impact**: CWE-312. Accidental persistence of plaintext credentials in the workflow document (and subsequently in deploy records and exports) through an easy-to-make API mistake.
- **Recommendation**: Make the safe path the only path. Options: (a) change `ApplyWorkflowCredentials` to return a new, credential-injected clone instead of mutating; (b) in `WorkflowUpdate`/`WorkflowCreate`, reject node data where a declared credential key holds a non-sentinel string (same check as Finding 2). Option (b) is a cheap, defense-in-depth guard that also protects `ImportModule`/`ImportNodeRED` round-trips.
- **Code Example**:
```go
// In-place mutation — caller's stored object is now tainted.
func ApplyWorkflowCredentials(w WorkflowInterface, creds map[string]map[string]string) {
	for _, n := range w.GetNodes() {
		data := n.GetData()
		applyCredentialFields(n.GetType(), data, creds[n.GetID()])
		...
	}
}
```
- **Suggested Fix**:
```go
func ApplyWorkflowCredentials(w WorkflowInterface, creds map[string]map[string]string) WorkflowInterface {
	clone := DeepCloneWorkflow(w)
	applyCredentialsInto(clone, creds)
	return clone // stored object never sees secrets
}
```

## Medium Severity Findings

### Finding 4: Multi-entity writes are non-transactional — partial failure leaves inconsistent state
- **Location**: `store_workflow.go:160-197` (`WorkflowUpdate`), `store_workflow.go:247-289` (`persistWorkflowChildren`), `store_credential.go:204-233` (`CredentialReplaceForWorkflow`), `store_migrate.go:52-122`
- **Description**: `WorkflowUpdate` performs EntityUpdate, attribute replacement, then N child create/update/delete calls, then group sync — each an independent statement with no transaction. `CredentialReplaceForWorkflow` deletes/updates/creates credential entities one at a time. A failure mid-sequence (or a crash) leaves orphaned nodes, dangling edges, partially replaced credentials, or a half-migrated blob split between old and new layouts.
- **Impact**: CWE-662 (Improper Synchronization) / data-integrity loss. In the credential path, a mid-loop failure can silently delete secrets that were never re-created — an availability/integrity failure, and the retry path (`keep` semantics of `extractCredentialFields`) may not recover them.
- **Recommendation**: If `entitystore` exposes `GetDB()`/`sql.Tx`, wrap the composite writes in `db.Begin()`/`tx.Commit()` with rollback on error. If it does not, add a compensating cleanup (e.g. record the intended end-state and retry idempotently). Document the non-atomicity on `WorkflowUpdate` and `CredentialReplaceForWorkflow`.
- **Code Example**:
```go
// WorkflowUpdate: six separate writes, each can fail independently
st.entityStore.EntityUpdate(...)
st.entityAttributesReplace(...)
st.persistWorkflowChildren(...)      // N writes
st.syncWorkflowGroups(...)           // N writes
```
- **Suggested Fix**:
```go
tx, err := st.entityStore.GetDB().BeginTx(ctx, nil)
if err != nil { return err }
defer tx.Rollback()
// ... all writes through a tx-scoped entitystore
return tx.Commit()
```

### Finding 5: TOCTOU on child-ID snapshot during `WorkflowUpdate`; no locking anywhere
- **Location**: `store_workflow.go:181-196`, `store_workflow.go:224-242`
- **Description**: `workflowChildIDs` snapshots the persisted node/edge IDs, then `persistWorkflowChildren` diffs against that snapshot. Two concurrent `WorkflowUpdate` calls for the same workflow interleave: both snapshot the same set, both create "new" children with the same IDs (PK conflicts or duplicates depending on DB), and each deletes children the other just wrote. `Store` itself carries no mutex, and domain objects (`workflowImplementation`, `nodeImplementation`) are plain structs — a `GetNodes()` iteration racing `AddNode` is a data race (`-race` would flag it).
- **Impact**: CWE-367 (TOCTOU) and CWE-362 (race condition). Lost or duplicated child entities, orphaned credentials referencing deleted nodes, corrupted workflow documents under concurrent saves (e.g. two editor sessions, or autosave + manual save).
- **Recommendation**: Serialize writes per workflow — a `sync.Mutex` keyed by workflow ID (or a store-wide write mutex, acceptable at this scale). For the domain objects, either document that they are not goroutine-safe or guard `nodes`/`edges` slices with a `sync.RWMutex`.
- **Suggested Fix**:
```go
type Store struct {
	...
	writeLocks keyedMutex // map[workflowID]*sync.Mutex with refcount
}
func (st *Store) WorkflowUpdate(ctx context.Context, w WorkflowInterface) error {
	unlock := st.writeLocks.Lock(w.GetID())
	defer unlock()
	...
}
```

### Finding 6: Unbounded parsing and listing of attacker-controllable input
- **Location**: `nodered.go:128-307` (`ImportNodeRED`), `module_exchange.go:661-819` (`ImportModule`), `store_workflow.go:103-151` (`WorkflowList`), `store_credential.go:100-161`
- **Description**: `ImportNodeRED`/`ImportModule` `json.Unmarshal` an arbitrary byte slice with no size cap, no node/edge count limit, and allocate unbounded maps/slices; deeply nested `data`/`nodes` structures recurse into `rewriteDefData`/`cloneDataMap` via multiple JSON round-trips. Separately, every `*List`/`*Count` method loads **all** matching entities into memory before applying offset/limit in Go (`matched[off:]` / `matched[:lim]`), and `WorkflowList` eagerly loads every workflow's children.
- **Impact**: CWE-770 (Allocation of Resources Without Limits) / CWE-400. A host application that imports user-supplied flow JSON, or that stores many entities, is exposed to memory exhaustion; `WorkflowList` without a limit is O(N) in children regardless of the requested page size.
- **Recommendation**: Document or enforce input limits: wrap reads in `http.MaxBytesReader`-style caps at the caller boundary, or accept an `io.Reader` and stream-decode with `json.Decoder` + a max-items guard (`if len(nodeObjs) > maxNodes`). For listings, push `limit`/`offset` down to the entitystore query where possible, or cap default page sizes.
- **Code Example**:
```go
if query.HasOffset() && query.GetOffset() < uint64(len(matched)) {
	matched = matched[query.GetOffset():]   // full list already materialized
}
```

### Finding 7: Attacker-supplied entity IDs accepted on import/create paths
- **Location**: `nodered.go:311-341` (`nodeREDToNode`), `store_node.go:29-44` (`NodeCreate`), `credential.go:59-71`
- **Description**: Imported JSON `id` fields are passed verbatim to `NewNode(id, ...)` and then `EntityCreate`, which uses the caller-supplied ID if non-empty (`entitystore` auto-mints a UUIDv7 only for empty IDs). A crafted import can collide with existing entity IDs (failing the insert — a DoS — or, depending on DB constraints, upserting) and can create credential entities with chosen IDs (`workflowID:nodeID` per the doc comment).
- **Impact**: CWE-639-ish (user-controlled key) — integrity: ID collisions abort or corrupt imports; predictability aids enumeration of entities through `EntityFindByID`. Severity is contained because `EntityCreate` inserts rather than updates, but a future upsert change would silently upgrade this to clobbering.
- **Recommendation**: Re-mint all imported IDs (the module importer already does this correctly via `generateID()` maps — `ImportNodeRED` does not). Validate IDs server-side: charset/length whitelist, and reject create requests whose ID already exists rather than relying on the DB constraint error.

## Low Severity Findings

### Finding 8: Log injection via unescaped imported values
- **Location**: `nodered.go:224`, `module_exchange.go:816`
- **Description**: `log.Printf` interpolates `z`, module labels, and workflow IDs taken from the imported JSON directly into log lines. Embedded `\n`/ANSI sequences in a malicious payload can forge log entries.
- **Impact**: CWE-117 (Improper Output Neutralization for Logs). Log spoofing; in aggregated/SIEM pipelines, forged or split entries.
- **Recommendation**: Sanitize or quote values (`%q` is already used for ids in some places — apply it consistently), or prefer `slog` with structured attributes which encode values.
- **Suggested Fix**:
```go
slog.Warn("ImportNodeRED: unknown z reference", "node", no["id"], "z", z)
```

### Finding 9: `GetDB()` exposes the raw `*sql.DB` handle
- **Location**: `store.go:148-150`
- **Description**: The store hands out its underlying `*sql.DB`, bypassing all entity-layer invariants (type checks, attribute managed-key rules, ID validation, retention pruning). Any consumer can read/write credential rows directly — including reading the plaintext secrets of Finding 1 — with no audit trail.
- **Impact**: CWE-668 (Exposure of Resource to Wrong Sphere). Erases the already-thin trust boundary around credential storage; not a vulnerability alone but multiplies Findings 1-3.
- **Recommendation**: Remove `GetDB()` from the public surface, or return a read-only/transaction-scoped wrapper if transactions are added for Finding 4.

### Finding 10: Error swallowing in migration dedup and export paths
- **Location**: `store_migrate.go:68`, `store_migrate.go:101`, `subflow.go:50`, `nodered.go:642-645`
- **Description**: `existing, _ := st.entityStore.EntityFindByID(ctx, newID)` treats a database error as "does not exist" and proceeds to create a duplicate. `mustJSON`/`cloneDataMap` silently drop marshal failures, producing `{}` output — an export can silently emit an empty data map where the source had content.
- **Impact**: CWE-390 (Detection of Error Condition Without Action). Silent data loss/corruption that surfaces downstream as missing config — a silent failure mode in a persistence layer.
- **Recommendation**: Propagate the error: `existing, err := ...; if err != nil { return migrated, err }`. For `mustJSON`, either return an error or at minimum log.

### Finding 11: Dead debug/logging surface
- **Location**: `store.go:120,140,152-155`
- **Description**: `debugEnabled` and `logger` are stored but never used; the `slog` logger writes unconditionally to `os.Stdout` and cannot be configured by the host app. `EnableDebug` toggles a flag nothing reads. (In `entitystore`, `GetDebug()` *does* `log.Println` full entity rows — see Finding 1 — so debug flags in this stack deserve discipline.)
- **Impact**: Low — misleading API; future debugging could leak attribute values (including credentials) to stdout without the caller's knowledge.
- **Recommendation**: Either wire `logger`/`debugEnabled` through (with redaction of `ATTRIBUTE_DATA`-class values) or remove them and accept a caller-supplied `*slog.Logger` in `NewStoreOptions`.

### Finding 12: Missing context cancellation in long loops; count-by-list
- **Location**: `store_credential.go:126-145`, `store_workflow.go:126-135`, `store_migrate.go:31`
- **Description**: Per-entity attribute loads and deletes iterate without checking `ctx.Err()`; `CredentialCount`/`NodeCount`/`DeployRecordCount` under `WithWorkflowID` materialize the full list just to return `len(list)`.
- **Impact**: Low — a cancelled request keeps burning DB work; counts are O(table) memory.
- **Recommendation**: Check `ctx.Err()` at loop heads; push COUNT down to a dedicated entitystore query when the underlying store gains attribute-count support.

## Best Practice Recommendations

1. **Introduce a `CredentialCipher` seam** (Finding 1) with `NewStoreOptions.CredentialKey`; make it required in a future major version, mirroring Node-RED's `credentialSecret`.
2. **Centralize the "no plaintext credential past this line" invariant** as an exported validation function and call it in `WorkflowCreate`, `WorkflowUpdate`, `DeployRecordCreate`, `MessageCreate` (payloads can also carry secrets in flight data).
3. **Add a transaction boundary** to entitystore or accept a `*sql.DB` in nodestore and open `sql.Tx` internally for composite writes.
4. **Document trust boundaries** in `README.md`: the store trusts its caller for authorization (any caller can read any workflow/credential — there is no tenant scoping beyond `workflow_id`), for input size limits, and for keeping injected documents away from persistence.
5. **Run `govulncheck` in CI** and pin `modernc.org/sqlite` upgrades deliberately — pure-Go SQLite has had memory-safety-adjacent and correctness bugs.
6. **Use `go test -race`** on `*_test.go` covering concurrent `WorkflowUpdate`/`CredentialReplaceForWorkflow` to codify the locking expectation.
7. **Consistent `%q`/structured logging** for all values derived from imported documents.
8. **Cap `DeployRecordRetention` enforcement defensively** — pruning happens post-insert; an error path currently leaves the record inserted but unpruned (ordering is already correct; keep it that way inside a transaction per Finding 4).

## Dependencies Analysis

- **Total dependencies**: 3 direct (`dracory/entitystore v1.24.0`, `dromara/carbon/v2 v2.7.1`, `modernc.org/sqlite v1.63.0`) + ~15 indirect (106 entries in `go.sum`).
- **Dependencies with known vulnerabilities**: 0 confirmed — `govulncheck` is not installed in this environment; run it before relying on this count.
- **Notable items**:
  - `modernc.org/sqlite v1.63.0` — pure-Go SQLite; keep current, it receives frequent correctness fixes.
  - ID generation uses the **Go 1.27 standard-library `uuid` package** (`import "uuid"` in `nodered.go:9`) — not `google/uuid` despite it being in `go.sum`. `NewV7` uses `crypto/rand`; this is fine, but the bare `"uuid"` import is non-obvious — consider the explicit `crypto/uuid`/path spelling if a linter flags it.
  - `dracory/entitystore` is a sibling first-party module; its `GetDebug()` path logs full entity rows (including attribute maps) — review it with the same lens.

## Compliance Considerations

- **PCI-DSS / general secret handling**: Requirement 3 (protect stored account data) and common secret-management standards are directly implicated by Findings 1-3 if any stored credential is a cardholder-data-environment secret or API key.
- **GDPR**: `message` payloads (`ATTRIBUTE_PAYLOAD`) and `deploy_record.actor` may contain personal data or message content persisted without a retention/purge story — deploy records are capped, but messages and credentials have no TTL. Data-subject erasure requires deleting credential + message entities, which `WorkflowDeleteByID` does not cascade (only nodes/edges are cascaded — credentials and deploy records are deleted by separate explicit calls, easy to forget).
- **Auditability**: `actor` is a free string with no integrity protection; combined with Finding 8, deploy history is forgeable/spoofable at the storage layer.

## Summary Statistics

- Total issues found: 12
- Critical: 0
- High: 3
- Medium: 4
- Low: 5

## Next Steps

1. Add encryption-at-rest for credential entities (`CredentialCipher`, AES-256-GCM, caller-supplied key) — Finding 1.
2. Enforce the sentinel invariant inside `DeployRecordCreate` and `WorkflowUpdate`/`WorkflowCreate` so plaintext secrets can never be persisted — Findings 2, 3.
3. Wrap composite writes (`WorkflowUpdate`, `CredentialReplaceForWorkflow`, migration) in DB transactions — Finding 4.
4. Serialize per-workflow writes and document domain-object goroutine-safety — Finding 5.
5. Re-mint imported node/edge IDs in `ImportNodeRED` (as `ImportModule` already does) and cap imported document size/item counts — Findings 6, 7.
6. Remove or restrict `GetDB()`, sanitize `log.Printf` values, stop swallowing `EntityFindByID` errors — Findings 8-10.
7. Wire `govulncheck` and `go test -race` into CI.
