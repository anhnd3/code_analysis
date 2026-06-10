# P3-UA-AGENT-MODE-1 — Local Sub-Agent Operating Model Design

**Project:** `anhnd3/code_analysis`  
**Phase:** Phase 1 / P3-UA-AGENT-MODE-1  
**Primary goal:** define and implement the local sub-agent operating model before building the first `FunctionLogic` analyzer output.  
**Reference architecture:** `Lum1104/Understand-Anything` public GitHub repository.  
**Target implementation language:** Go.  
**Authoring date:** 2026-05-30.

---

## 0. Executive Decision

The requirement is clear:

1. We should **not jump directly** to `cameraV2Handler.DetectQR` Mermaid/function-output MVP.
2. We should first copy the **agent/sub-agent operating model** from `understand-anything`.
3. We should implement the local Go-native agent-mode foundation:
   - sub-agent contracts,
   - role registry,
   - context packet contracts,
   - artifact discipline,
   - validation model,
   - mock runner,
   - and prompt-to-pseudocode transfer documents.
4. Only after this phase should we run the first real `FunctionLogicAnalyzerAgent`.

The next phase is therefore:

```text
P3-UA-AGENT-MODE-1
Local Sub-Agent Operating Model + Harness Contracts
```

The follow-up phase will be:

```text
P3-UA-FUNCTIONLOGIC-1
First real FunctionLogic run for cameraV2Handler.DetectQR
```

---

## 1. Current Repository Review Summary

### 1.1 Current active product path

The current `README.md` still defines the active path as:

```text
scan -> index -> inspect-function -> review-flow -> export-md/export-mermaid/export-graphjson
```

Current active commands:

```text
scan
index
inspect-function
review-flow
export-md
export-mermaid
export-graphjson
```

This means the repository already has a stabilized facts-first workflow, but it is still centered on a transitional review-flow model.

### 1.2 Existing package layout

Current documented package layout:

```text
cmd/analysis-cli          — CLI entry point
internal/app              — Bootstrap, config, lifecycle, logging
internal/indexer          — Workspace scanning & symbol extraction
internal/facts            — Fact model types & builders
internal/flow             — Flow skeleton
internal/review           — LLM-led review service, transitional
internal/export           — Markdown, Mermaid, GraphJSON renderers
internal/llm              — LLM integration
```

### 1.3 Existing useful foundation

The repository already has:

- workspace scan,
- index/facts generation,
- SQLite/JSONL persistence,
- bounded function inspection,
- basic LLM-led review flow,
- Markdown/Mermaid/GraphJSON export,
- baseline test gate.

The existing `facts.QueryService.InspectFunction` is important. It can already resolve one symbol and build a bounded function context packet containing:

```text
root symbol
root file
function source
surrounding context
imports
outgoing call candidates
incoming call candidates
nearby tests
```

This is directly reusable as a lower-level context source for future sub-agents.

### 1.4 Existing deterministic control hints

The Go indexer already detects:

```text
if
switch
for
range
go
defer
Wait()
```

These are important future inputs for `FunctionLogicAnalyzerAgent`, but they are not yet organized as a role-specific context packet.

### 1.5 Existing LLM client limitation

Current `internal/llm/client.go` is review-flow specific. It defines:

```text
ReviewRequest
ReviewResponse
HopDecision
Client.Review(...)
```

This is not a generic sub-agent execution client yet. We should not delete it. We should add a new generic JSON-task client later, beside the existing review client.

### 1.6 Existing ADR mismatch

The existing AI-led flow ADR is valuable, but it still points toward:

```text
FlowPack model
-> deterministic BFS
-> evidence collector
-> LLM planner
-> boundary resolvers
-> exporters
-> MCP
```

Our latest decision inserts a required earlier phase:

```text
sub-agent operating model
-> harness contracts
-> role registry
-> context packet contracts
-> mock runner
-> only then FunctionLogic
```

So the current architecture is not wrong, but it is incomplete for the new Phase 1.

---

## 2. Understand-Anything Public Architecture Review

### 2.1 Public reference position

`understand-anything` describes itself as a system that turns a codebase, knowledge base, or docs into an interactive knowledge graph. It works across Claude Code, Codex, Cursor, Copilot, Gemini CLI, and more. Its README states that `/understand` runs a multi-agent pipeline that scans a project, extracts files/functions/classes/dependencies, and writes `.understand-anything/knowledge-graph.json`.

Source:
- https://github.com/Lum1104/Understand-Anything

### 2.2 Key operating pattern

The key reusable pattern is **not the UI** and not the TypeScript implementation.

The key reusable pattern is:

```text
deterministic preprocessing
-> bounded agent role prompt
-> structured JSON artifact
-> validation/review
-> downstream role consumes artifact
```

### 2.3 Specialized reference roles

Public roles reviewed:

```text
project-scanner
file-analyzer
architecture-analyzer
domain-analyzer
graph-reviewer
tour-builder
article-analyzer
```

For our local SDLC project, we do not need to copy all roles literally. We should adapt them into 8 Go-native local roles:

```text
WorkspaceScannerAgent
FileStructureAnalyzerAgent
ArchitectureRoleClassifierAgent
BoundaryAnalyzerAgent
FunctionLogicAnalyzerAgent
FlowComposerAgent
DomainFlowAgent
FlowReviewerAgent
```

### 2.4 What we copy conceptually

Copy these behaviors:

```text
1. Role-specific responsibility boundary.
2. Deterministic extraction before LLM interpretation.
3. Bounded context packets.
4. Structured JSON outputs.
5. Intermediate artifacts.
6. Reviewer/validator role.
7. Incremental execution direction.
8. Multi-language architecture mindset.
9. Domain mapping: domain -> flow -> step.
10. Do-not-invent constraints.
```

### 2.5 What we do not copy

Do not copy:

```text
1. TypeScript/Node implementation.
2. React/dashboard UI.
3. Chat UI.
4. Plugin-specific Claude/Cursor/Codex installation structure.
5. Graph-first output as the first product.
6. Guided tour UI.
7. Knowledge-base article analyzer in Phase 1.
```

---

## 3. Phase 1 Goal

### 3.1 Phase name

```text
P3-UA-AGENT-MODE-1 — Local Sub-Agent Operating Model
```

### 3.2 Phase objective

Build the architecture and minimal executable contracts for local sub-agents.

This phase should answer:

```text
Can our Go codebase represent and execute an Understand-Anything-style local sub-agent pipeline?
```

It should not yet answer:

```text
Can we generate final function logic Mermaid for cameraV2Handler.DetectQR?
```

### 3.3 Phase outputs

Documentation outputs:

```text
docs/architecture/sub_agent_operating_model.md
docs/architecture/sub_agent_contracts.md
docs/architecture/context_packet_contracts.md
docs/architecture/artifact_contracts.md
docs/architecture/role_pipeline.md
docs/research/understand_anything_prompt_to_pseudocode.md
docs/plan/p3_ua_agent_mode_1_execution_plan.md
```

Code outputs:

```text
internal/harness/
internal/agents/
internal/contextpack/
```

Optional CLI output:

```text
analysis-cli list-agent-roles
```

### 3.4 Phase non-goals

```text
No FunctionLogic Analyzer implementation yet.
No Mermaid output yet.
No full flow tracing.
No BFS trace-flow.
No cross-service resolution.
No Java/JS/Python adapter implementation.
No UI/chat/dashboard.
No broad refactor of internal/indexer or internal/facts.
No deletion of internal/review.
```

---

## 4. Design Principles

### 4.1 Agent role means contract, not chatbot

In this project, a sub-agent is not a separate chat UI. It is a bounded execution role:

```text
role name
purpose
input artifact types
context packet builder
prompt template
output schema
token budget
validator
retry policy
artifact output path
```

### 4.2 LLM is not source of truth

The existing ADR rule remains valid:

```text
No evidence, no accepted edge.
```

LLMs may summarize, classify, rank, and suggest. They must not create accepted facts without deterministic evidence.

### 4.3 Artifacts first

Every role writes durable artifacts:

```text
context_packet.json
prompt.txt
llm_raw.json
parsed.json
validation.json
diagnostics.md
```

Markdown/Mermaid are renderings, not source of truth.

### 4.4 Harness before analyzer

The first engineering objective is not a high-quality function analysis result. The first objective is a reusable harness that can run future sub-agents safely.

### 4.5 Local-small-LLM friendly implementation

For implementation using local smaller LLMs, every task must follow the agreed process:

```text
1. World-class model produces final contract + pseudocode.
2. Local small model writes new files/new functions from the contract.
3. Avoid in-place mutation.
4. Prefer full replacement/new function over partial patching.
5. Switch call sites only after new path compiles.
6. Keep old path temporarily for rollback.
7. Delete old path only after baseline passes.
```

---

## 5. Target Package Architecture

### 5.1 New packages

```text
internal/harness
internal/agents
internal/contextpack
```

### 5.2 Package responsibilities

#### `internal/harness`

Owns generic sub-agent execution contracts.

```text
SubAgentRole
SubAgentTask
ArtifactRef
TokenBudget
RetryPolicy
ValidationReport
RoleRegistry
Runner
ArtifactWriter
```

#### `internal/agents`

Owns role metadata and later role handlers.

```text
WorkspaceScannerAgent
FileStructureAnalyzerAgent
ArchitectureRoleClassifierAgent
BoundaryAnalyzerAgent
FunctionLogicAnalyzerAgent
FlowComposerAgent
DomainFlowAgent
FlowReviewerAgent
```

In Phase 1, these should be metadata-only or stub handlers. No real analysis.

#### `internal/contextpack`

Owns generic context packet contracts.

```text
ContextPacket
ContextInput
SourceSlice
ContextConstraint
OutputSchemaRef
```

In Phase 1, this is schema + validation only.

---

## 6. Core Contracts

### 6.1 `SubAgentRole`

```go
type RoleName string

type SubAgentRole struct {
    Name        RoleName
    Description string

    InputTypes  []string
    OutputTypes []string

    TokenBudget TokenBudget
    RetryPolicy RetryPolicy
}
```

### 6.2 `SubAgentTask`

```go
type SubAgentTask struct {
    ID          string
    Role        RoleName
    WorkspaceID string
    SnapshotID  string

    InputArtifacts []ArtifactRef
    OutputDir      string
}
```

### 6.3 `ArtifactRef`

```go
type ArtifactRef struct {
    Type string
    Path string
}
```

### 6.4 `TokenBudget`

```go
type TokenBudget struct {
    MaxInputTokens  int
    MaxOutputTokens int
}
```

### 6.5 `RetryPolicy`

```go
type RetryPolicy struct {
    MaxAttempts        int
    RetryOnJSONError   bool
    RetryOnSchemaError bool
}
```

### 6.6 `ValidationReport`

```go
type ValidationReport struct {
    Accepted bool
    Issues   []ValidationIssue
    Warnings []ValidationIssue
}

type ValidationIssue struct {
    Code     string
    Message  string
    Severity string
    Source   string
}
```

### 6.7 `ContextPacket`

```go
type ContextPacket struct {
    TaskID       string
    Role         string
    Goal         string

    Inputs       []ContextInput
    SourceSlices []SourceSlice
    Facts        map[string]any
    Constraints  []string

    OutputSchema string
    TokenBudget  harness.TokenBudget
}
```

### 6.8 `SourceSlice`

```go
type SourceSlice struct {
    FilePath  string
    StartLine int
    EndLine   int
    Content   string
}
```

---

## 7. Role-by-Role Design and Pseudocode

## 7.1 WorkspaceScannerAgent

### Reference role

`understand-anything`: `project-scanner`.

### Reference behavior

The project scanner discovers files, languages, frameworks, import maps, file categories, and estimated complexity. It performs deterministic discovery first, then adds a short project description.

### Local role purpose

Create deterministic workspace and service inventory.

### Inputs

```text
workspace_path
ignore_patterns
target_hints
```

### Outputs

```text
workspace_scan.json
file_catalog.json
language_summary.json
framework_summary.json
service_candidates.json
```

### Deterministic responsibility

```text
file discovery
file category
language detection
manifest detection
service candidate detection
framework signal extraction
```

### LLM responsibility

```text
short project/service summary only
```

### Pseudocode

```text
function RunWorkspaceScanner(request):
    validate workspace_path exists
    load ignore policy
    discover files using git-aware scan
    classify each file by category and language
    detect manifest files
    infer frameworks from manifests
    infer service candidates from repo layout and entrypoints
    build workspace_scan artifact
    build file_catalog artifact
    if LLM enabled:
        synthesize short project narrative from README/manifests
    validate all file paths exist
    write artifacts
    return scan result
```

### Phase 1 implementation note

Do not implement scanner logic again. Existing `internal/indexer` already scans. In Phase 1, only define this as an agent role and document its future mapping.

### Test strategy

```text
DefaultRoles includes workspace-scanner.
Role metadata has required input/output types.
No execution yet.
```

---

## 7.2 FileStructureAnalyzerAgent

### Reference role

`understand-anything`: `file-analyzer`.

### Reference behavior

The file analyzer processes file batches, runs deterministic structural extraction, then applies semantic analysis to produce graph nodes/edges. It requires source-grounded structured output.

### Local role purpose

Extract language-neutral structural facts.

### Inputs

```text
file batch
file_catalog.json
import_map
language adapter
```

### Outputs

```text
symbol_index.json
function_index.json
import_index.json
call_index.json
control_skeleton.json
diagnostics.json
```

### Deterministic responsibility

```text
symbols
imports
exports
call candidates
control skeleton
line ranges
receiver/class/method info
```

### LLM responsibility

```text
optional summaries only
```

### Pseudocode

```text
function RunFileStructureAnalyzer(request):
    validate batch files exist
    for each file in batch:
        adapter = language_registry.Get(file.language)
        symbols = adapter.ExtractSymbols(file)
        imports = adapter.ExtractImports(file)
        calls = adapter.ExtractCalls(file, symbols)
        controls = adapter.ExtractControlBlocks(file, symbols)
        diagnostics += adapter diagnostics
    normalize all IDs
    deduplicate symbols and calls
    write structure artifacts
    validate every symbol has file path and line range
    return structure result
```

### Phase 1 implementation note

No real analyzer yet. Define the role. Existing `internal/indexer` stays source of facts.

### Test strategy

```text
DefaultRoles includes file-structure-analyzer.
The role declares deterministic output artifacts.
```

---

## 7.3 ArchitectureRoleClassifierAgent

### Reference role

`understand-anything`: `architecture-analyzer`.

### Reference behavior

It computes structural patterns, directory groupings, import adjacency, dependency direction, non-code relationships, and then assigns architecture layers.

### Local role purpose

Classify files/functions into SDLC roles.

### Inputs

```text
file_catalog.json
symbol_index.json
call_index.json
import_index.json
boundary_candidates.json
```

### Outputs

```text
architecture_roles.json
```

### Role vocabulary

```text
entrypoint
http_handler
grpc_handler
message_consumer
message_producer
request_validator
business_orchestrator
domain_processor
repository
external_gateway
response_mapper
config_provider
test_owner
unknown
```

### Deterministic responsibility

```text
path pattern detection
file name pattern detection
symbol name pattern detection
boundary marker detection
dependency direction
fan-in/fan-out
```

### LLM responsibility

```text
semantic classification for ambiguous cases only
```

### Pseudocode

```text
function RunArchitectureRoleClassifier(request):
    load file catalog and symbol index
    compute deterministic signals:
        path patterns
        file names
        symbol names
        import/call fan-in/fan-out
        boundary markers
    assign high-confidence roles by rule
    build ambiguous role candidates
    if LLM enabled:
        ask LLM to classify ambiguous cases using compact context
    validate every file/function has zero or one primary role
    write architecture_roles.json
    return role classification result
```

### Phase 1 implementation note

Metadata only.

### Test strategy

```text
Role metadata exists.
Role output type includes architecture_roles.json.
```

---

## 7.4 BoundaryAnalyzerAgent

### Reference role

This is a local SDLC-specific role. It is not a one-to-one copy of `understand-anything`.

### Local role purpose

Detect HTTP/gRPC/MQ/CLI/startup boundaries and map them to internal handlers.

### Inputs

```text
symbol_index.json
call_index.json
file_catalog.json
architecture_roles.json
source snippets
```

### Outputs

```text
boundary_index.json
http_boundaries.json
grpc_boundaries.json
async_boundaries.json
startup_boundaries.json
```

### Boundary types

```text
http
grpc
message_queue
cron
cli
startup
```

### Deterministic responsibility

```text
Gin/Echo/Chi route registration
Cobra commands
gRPC service registration
Kafka/RabbitMQ producer/consumer hints
config/proto/topic evidence
```

### LLM responsibility

```text
resolve ambiguous wrapper patterns only
```

### Pseudocode

```text
function RunBoundaryAnalyzer(request):
    load symbols, calls, roles, file catalog
    scan known framework patterns
    detect HTTP route registration
    detect gRPC server/client registrations
    detect producer/consumer topic usage
    detect CLI/startup entrypoints
    map each boundary to handler symbol if possible
    mark unresolved mappings as ambiguous
    validate each accepted boundary has evidence
    write boundary artifacts
    return boundary result
```

### Phase 1 implementation note

Metadata only. Real boundary logic belongs after context packet/harness is available.

### Test strategy

```text
Role metadata exists.
Output artifact types include boundary_index.json.
```

---

## 7.5 FunctionLogicAnalyzerAgent

### Reference role

This is our product-specific differentiator. It uses the role-isolated pattern from `understand-anything`, but its actual output is ours.

### Local role purpose

Analyze one function into executable logic blocks.

### Inputs

```text
function source
surrounding context
imports
call candidates
control skeleton
architecture role
boundary context
```

### Outputs

```text
function_logic.json
function_logic.md
function_logic.mmd
diagnostics.md
```

### Supported block types

```text
sequence
if
else_if
else
switch
case
default
for
range
return
defer
goroutine
call
method_call
interface_call
external_call
validate
transform
repository_read
repository_write
response_write
error_exit
```

### Pseudocode

```text
function RunFunctionLogicAnalyzer(request):
    resolve target symbol
    load context packet from facts.QueryService.InspectFunction
    load control skeleton and call candidates
    build FunctionLogicContextPacket
    if LLM enabled:
        ask LLM to map source into ordered logic blocks
    parse FunctionLogicV1
    validate every major block has line range or evidence
    validate calls have evidence or diagnostics
    render markdown and mermaid from parsed JSON
    write function logic artifacts
    return FunctionLogic result
```

### Phase 1 implementation note

Only role metadata now. Do not implement real FunctionLogic in this phase.

### Test strategy

```text
Role metadata exists.
Token budget is larger than simpler roles.
Output artifact types include function_logic.json.
```

---

## 7.6 FlowComposerAgent

### Local role purpose

Compose boundary + selected function logic into a service flow.

### Inputs

```text
boundary_index.json
function_logic.json
architecture_roles.json
call_index.json
evidence artifacts
```

### Outputs

```text
flow_pack.json
flow.md
flow.mmd
graph.json
evidence.md
diagnostics.md
```

### Expansion policy

Expand called functions only when they are meaningful:

```text
contains branch/loop/switch
validates input
transforms data
calls repository/gateway/external service
maps response
crosses boundary
explicitly selected by user
```

### Pseudocode

```text
function RunFlowComposer(request):
    resolve root boundary or root function
    load handler FunctionLogic
    identify expansion candidates
    expand only meaningful functions:
        validation
        branch/loop/switch
        external effects
        repository/gateway calls
        response mapping
    compose ordered FlowPack
    attach evidence
    mark unresolved edges ambiguous
    write flow artifacts
    return FlowPack
```

### Phase 1 implementation note

Metadata only.

### Test strategy

```text
Role metadata exists.
Output artifact types include flow_pack.json.
```

---

## 7.7 DomainFlowAgent

### Reference role

`understand-anything`: `domain-analyzer`.

### Reference behavior

It maps code into a hierarchy:

```text
Business Domain
-> Business Flow
-> Business Step
```

### Local role purpose

Map technical flows to business-domain language while preserving evidence links.

### Inputs

```text
flow_pack.json
architecture_roles.json
boundary_index.json
selected source evidence
```

### Outputs

```text
domain_flow.json
```

### Pseudocode

```text
function RunDomainFlowAgent(request):
    load FlowPack and boundary metadata
    infer business domain from route names, function names, docs, config
    group related flows into business domains
    map function logic steps into business steps
    preserve evidence references
    validate every business step links to technical evidence
    write domain_flow.json
    return domain graph
```

### Phase 1 implementation note

Metadata only.

### Test strategy

```text
Role metadata exists.
Output artifact types include domain_flow.json.
```

---

## 7.8 FlowReviewerAgent

### Reference role

`understand-anything`: `graph-reviewer`.

### Reference behavior

It validates schema, referential integrity, completeness, quality, node/edge consistency, and then renders approval/rejection.

### Local role purpose

Validate artifacts and block hallucinated outputs.

### Inputs

```text
function_logic.json
boundary_index.json
flow_pack.json
domain_flow.json
```

### Outputs

```text
validation_report.json
```

### Deterministic checks

```text
schema validity
referential integrity
evidence coverage
unknown type checks
ambiguity checks
duplicate IDs
line range validity
artifact path validity
```

### LLM responsibility

```text
optional quality critique only
```

### Pseudocode

```text
function RunFlowReviewer(request):
    load target artifact
    run schema validation
    run referential integrity checks
    run evidence coverage checks
    run unknown type checks
    run ambiguity checks
    if LLM review enabled:
        ask LLM for quality critique only
    decision = approved if no critical issues else rejected
    write validation_report.json
    return validation result
```

### Phase 1 implementation note

Metadata only. Generic `ValidationReport` belongs in `internal/harness`.

### Test strategy

```text
Role metadata exists.
Output artifact types include validation_report.json.
```

---

## 8. Agent Pipeline

### 8.1 Full future pipeline

```text
WorkspaceScannerAgent
-> FileStructureAnalyzerAgent
-> ArchitectureRoleClassifierAgent
-> BoundaryAnalyzerAgent
-> FunctionLogicAnalyzerAgent
-> FlowComposerAgent
-> DomainFlowAgent
-> FlowReviewerAgent
```

### 8.2 Phase 1 executable pipeline

In Phase 1, the pipeline is **metadata-level only**:

```text
DefaultRoles()
-> RegisterDefaultRoles(registry)
-> list-agent-roles
-> validate role contracts
-> mock runner executes fake role
-> writes parsed.json + validation.json
```

### 8.3 Phase 2 executable pipeline

Later:

```text
FunctionLogicAnalyzerAgent
-> context packet from facts.QueryService.InspectFunction
-> local LLM JSON call
-> parsed FunctionLogicV1
-> FlowReviewerAgent validation
-> render Markdown/Mermaid
```

---

## 9. Artifact Layout

Phase 1 should introduce the artifact convention even if only mock output exists.

```text
.local-agent/
  runs/
    <run_id>/
      tasks/
        <task_id>/
          context_packet.json
          prompt.txt
          llm_raw.json
          parsed.json
          validation.json
          diagnostics.md
```

Example:

```text
.local-agent/runs/run_001/tasks/mock_workspace_scanner/
  context_packet.json
  parsed.json
  validation.json
  diagnostics.md
```

Later FunctionLogic example:

```text
.local-agent/runs/run_002/tasks/function_logic_cameraV2Handler_DetectQR/
  context_packet.json
  prompt.txt
  llm_raw.json
  parsed_function_logic.json
  validation.json
  diagnostics.md
  function_logic.md
  function_logic.mmd
```

---

## 10. Detailed Implementation Plan

## 10.1 Task 1 — Add agent operating model docs

### Files

```text
docs/architecture/sub_agent_operating_model.md
docs/architecture/sub_agent_contracts.md
docs/architecture/context_packet_contracts.md
docs/architecture/artifact_contracts.md
docs/architecture/role_pipeline.md
docs/research/understand_anything_prompt_to_pseudocode.md
docs/plan/p3_ua_agent_mode_1_execution_plan.md
```

### Local-small-LLM instruction

```text
Create the new markdown files exactly as provided.
Do not edit existing docs.
Do not touch Go code.
```

### Acceptance

```bash
git diff -- docs/architecture docs/research docs/plan
```

---

## 10.2 Task 2 — Add `internal/harness` contracts

### Files

```text
internal/harness/types.go
internal/harness/registry.go
internal/harness/validation.go
internal/harness/types_test.go
```

### Final function contracts

```text
RegisterRole(role SubAgentRole) error
GetRole(name RoleName) (SubAgentRole, bool)
ValidateTask(task SubAgentTask) ValidationReport
```

### Pseudocode

```text
function RegisterRole(role):
    if role.Name empty: error
    if duplicate role name: error
    save role by name

function GetRole(name):
    if missing: return false
    return role, true

function ValidateTask(task):
    require task.ID
    require task.Role
    require task.OutputDir
    return report
```

### Local-small-LLM instruction

```text
Implement new package internal/harness.
Create only new files.
Do not import indexer/facts/review.
Keep package dependency-free except standard library.
Add tests for duplicate role registration and invalid task validation.
```

### Acceptance

```bash
gofmt -w internal/harness
go test ./internal/harness
./scripts/test_required_baseline.sh
```

---

## 10.3 Task 3 — Add role registry stubs

### Files

```text
internal/agents/roles.go
internal/agents/roles_test.go
```

### Final function contracts

```text
DefaultRoles() []harness.SubAgentRole
RegisterDefaultRoles(registry *harness.RoleRegistry) error
```

### Pseudocode

```text
function DefaultRoles():
    return [
      WorkspaceScannerAgent,
      FileStructureAnalyzerAgent,
      ArchitectureRoleClassifierAgent,
      BoundaryAnalyzerAgent,
      FunctionLogicAnalyzerAgent,
      FlowComposerAgent,
      DomainFlowAgent,
      FlowReviewerAgent
    ]

function RegisterDefaultRoles(registry):
    for role in DefaultRoles:
        registry.Register(role)
```

### Local-small-LLM instruction

```text
Implement internal/agents using internal/harness types.
No business logic.
No LLM calls.
No facts/indexer imports.
```

### Acceptance

```bash
gofmt -w internal/agents
go test ./internal/agents ./internal/harness
./scripts/test_required_baseline.sh
```

---

## 10.4 Task 4 — Add context packet contracts

### Files

```text
internal/contextpack/types.go
internal/contextpack/types_test.go
```

### Final function contract

```text
ValidateContextPacket(packet ContextPacket) harness.ValidationReport
```

### Pseudocode

```text
function ValidateContextPacket(packet):
    require TaskID
    require Role
    require Goal
    require OutputSchema
    if SourceSlice exists:
        require FilePath
        require valid line range
    return ValidationReport
```

### Local-small-LLM instruction

```text
Implement contextpack package.
Use harness.ValidationReport if useful.
No LLM calls.
No production analyzer logic.
```

### Acceptance

```bash
gofmt -w internal/contextpack
go test ./internal/contextpack ./internal/harness
./scripts/test_required_baseline.sh
```

---

## 10.5 Task 5 — Add artifact writer

### Files

```text
internal/harness/artifact_writer.go
internal/harness/artifact_writer_test.go
```

### Final function contracts

```text
TaskArtifactDir(root, runID, taskID string) string
WriteJSON(path string, payload any) error
WriteText(path string, body string) error
```

### Pseudocode

```text
function WriteJSON(path, payload):
    mkdir parent
    marshal indent
    write file

function WriteText(path, body):
    mkdir parent
    write file

function TaskArtifactDir(root, runID, taskID):
    sanitize taskID
    return root/.local-agent/runs/runID/tasks/taskID
```

### Local-small-LLM instruction

```text
Implement only artifact path and writer functions.
Do not wire into CLI yet.
```

### Acceptance

```bash
gofmt -w internal/harness
go test ./internal/harness
./scripts/test_required_baseline.sh
```

---

## 10.6 Task 6 — Add mock runner

### Files

```text
internal/harness/runner.go
internal/harness/runner_test.go
```

### Final function contracts

```go
type Runner interface {
    Run(ctx context.Context, task SubAgentTask, packet contextpack.ContextPacket) (RunResult, error)
}

type RoleHandler interface {
    Handle(ctx context.Context, task SubAgentTask, packet contextpack.ContextPacket) (any, ValidationReport, error)
}
```

### Pseudocode

```text
function Runner.Run(task, packet):
    validate task
    validate packet
    load role metadata
    call role handler
    write parsed.json
    write validation.json
    write diagnostics.md
    return RunResult
```

### Local-small-LLM instruction

```text
Implement a mock-capable runner.
Do not call actual LLM.
Use simple fake handler in tests.
```

### Acceptance

```bash
gofmt -w internal/harness
go test ./internal/harness ./internal/contextpack ./internal/agents
./scripts/test_required_baseline.sh
```

---

## 10.7 Task 7 — Add CLI command `list-agent-roles`

### File

```text
cmd/analysis-cli/main.go
```

### Final function contract

```text
runListAgentRoles(args []string)
```

### Pseudocode

```text
function runListAgentRoles(args):
    registry = harness.NewRoleRegistry()
    agents.RegisterDefaultRoles(registry)
    roles = registry.List()
    write JSON roles
```

### Local-small-LLM instruction

```text
Add a new CLI command that only lists role metadata.
Do not run agents.
Do not alter existing commands except adding one switch case and usage line.
```

### Acceptance

```bash
gofmt -w cmd/analysis-cli internal/agents internal/harness
go test ./cmd/... ./internal/...
./scripts/test_required_baseline.sh
analysis-cli list-agent-roles
```

---

## 11. First Phase Acceptance Criteria

Phase 1 is complete when:

```text
1. New docs exist for sub-agent operating model.
2. internal/harness compiles and has tests.
3. internal/agents defines 8 role metadata stubs.
4. internal/contextpack defines generic context packet.
5. artifact writer creates correct .local-agent run/task paths.
6. mock runner can execute a fake role.
7. optional list-agent-roles command returns 8 roles.
8. No existing scan/index/review/export behavior changes.
9. ./scripts/test_required_baseline.sh passes.
```

---

## 12. High-Level Take Notes for Later Per-Agent Planning

Each agent will need its own detailed planning document later:

```text
docs/plan/agent_workspace_scanner_plan.md
docs/plan/agent_file_structure_analyzer_plan.md
docs/plan/agent_architecture_role_classifier_plan.md
docs/plan/agent_boundary_analyzer_plan.md
docs/plan/agent_function_logic_analyzer_plan.md
docs/plan/agent_flow_composer_plan.md
docs/plan/agent_domain_flow_plan.md
docs/plan/agent_flow_reviewer_plan.md
```

Each per-agent plan should use this template:

```text
1. Role purpose
2. Inputs
3. Outputs
4. Current repo data source
5. Understand-Anything reference behavior
6. Deterministic responsibilities
7. LLM responsibilities
8. Context packet schema
9. Output schema
10. Validator rules
11. Pseudocode
12. Implementation tasks
13. Test cases
14. Acceptance criteria
15. Known limitations
```

---

## 13. Risks and Controls

### Risk 1 — Building one-off FunctionLogic too early

Control:

```text
Finish agent contracts and harness first.
```

### Risk 2 — Local LLM modifies too much code

Control:

```text
Use contract-first replacement workflow.
Prefer new files and new functions.
Do not patch existing logic unless explicitly required.
```

### Risk 3 — Agent role metadata becomes unused documentation

Control:

```text
Implement mock runner and list-agent-roles.
Make roles executable, even with fake handlers.
```

### Risk 4 — Current docs and new docs conflict

Control:

```text
Add a note that existing ADR represents earlier flow-trace direction.
New P3-UA-AGENT-MODE-1 inserts a prerequisite phase before flow tracing.
```

### Risk 5 — Overengineering before proof

Control:

```text
Keep Phase 1 small:
contracts
registry
context packet
artifact writer
mock runner
role list CLI
```

---

## 14. Final Recommendation

Do this now:

```text
P3-UA-AGENT-MODE-1
```

Implement only:

```text
sub-agent docs
harness contracts
role registry
context packet contracts
artifact writer
mock runner
list-agent-roles
```

Do not implement:

```text
FunctionLogic Analyzer
Mermaid renderer
Flow Composer
Boundary resolver
Cross-service tracing
```

After Phase 1 passes, proceed to:

```text
P3-UA-FUNCTIONLOGIC-1
```

First real target:

```text
cameraV2Handler.DetectQR
```

Expected later output:

```text
function_logic.json
function_logic.md
function_logic.mmd
diagnostics.md
```

But that output belongs to the next phase, not Phase 1.

---

## 15. Source References

### Current `anhnd3/code_analysis` repo review

Reviewed active files:

```text
README.md
docs/architecture/overview.md
docs/ai_led_flow_architecture_decision_record.md
internal/flow/types.go
internal/llm/client.go
internal/facts/query_service.go
internal/indexer/extractor_go_semantic_control.go
internal/indexer/extractor_go_semantic_async.go
cmd/analysis-cli/main.go
```

### Public `understand-anything` reference

Reviewed public GitHub pages:

```text
https://github.com/Lum1104/Understand-Anything
https://github.com/Lum1104/Understand-Anything/blob/main/understand-anything-plugin/agents/project-scanner.md
https://github.com/Lum1104/Understand-Anything/blob/main/understand-anything-plugin/agents/file-analyzer.md
https://github.com/Lum1104/Understand-Anything/blob/main/understand-anything-plugin/agents/architecture-analyzer.md
https://github.com/Lum1104/Understand-Anything/blob/main/understand-anything-plugin/agents/domain-analyzer.md
https://github.com/Lum1104/Understand-Anything/blob/main/understand-anything-plugin/agents/graph-reviewer.md
```
