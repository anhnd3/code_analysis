package sqlite

import (
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"

	"analysis-module/internal/domain/reviewgraph"

	_ "modernc.org/sqlite"
)

type Store struct {
	db *sql.DB
}

func New(path string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	store := &Store{db: db}
	if err := store.migrate(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return store, nil
}

func (s *Store) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

func (s *Store) ReplaceSnapshot(snapshotID string, nodes []reviewgraph.Node, edges []reviewgraph.Edge, artifacts []reviewgraph.Artifact) error {
	if _, err := s.db.Exec(`pragma journal_mode = WAL`); err != nil {
		return err
	}
	if _, err := s.db.Exec(`pragma synchronous = NORMAL`); err != nil {
		return err
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`delete from artifacts where snapshot_id = ?`, snapshotID); err != nil {
		return err
	}
	if _, err := tx.Exec(`delete from derived_edges where snapshot_id = ?`, snapshotID); err != nil {
		return err
	}
	if _, err := tx.Exec(`delete from derived_nodes where snapshot_id = ?`, snapshotID); err != nil {
		return err
	}
	if _, err := tx.Exec(`delete from edges where snapshot_id = ?`, snapshotID); err != nil {
		return err
	}
	if _, err := tx.Exec(`delete from nodes where snapshot_id = ?`, snapshotID); err != nil {
		return err
	}

	nodeStmt, err := tx.Prepare(`insert into nodes(id, snapshot_id, repo, service, language, kind, symbol, file_path, start_line, end_line, signature, visibility, node_role, metadata_json) values(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	defer nodeStmt.Close()
	for _, node := range nodes {
		if _, err := nodeStmt.Exec(node.ID, node.SnapshotID, node.Repo, node.Service, node.Language, string(node.Kind), node.Symbol, node.FilePath, node.StartLine, node.EndLine, node.Signature, node.Visibility, string(node.NodeRole), node.MetadataJSON); err != nil {
			return err
		}
	}

	edgeStmt, err := tx.Prepare(`insert into edges(id, snapshot_id, src_id, dst_id, edge_type, flow_kind, confidence, evidence_file, evidence_line, evidence_text, transport, topic_or_channel, metadata_json) values(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	defer edgeStmt.Close()
	for _, edge := range edges {
		if _, err := edgeStmt.Exec(edge.ID, edge.SnapshotID, edge.SrcID, edge.DstID, string(edge.EdgeType), string(edge.FlowKind), edge.Confidence, edge.EvidenceFile, edge.EvidenceLine, edge.EvidenceText, edge.Transport, edge.TopicOrChannel, edge.MetadataJSON); err != nil {
			return err
		}
	}

	artifactStmt, err := tx.Prepare(`insert into artifacts(id, snapshot_id, artifact_type, target_node_id, path, metadata_json) values(?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	defer artifactStmt.Close()
	for _, artifact := range artifacts {
		if _, err := artifactStmt.Exec(artifact.ID, artifact.SnapshotID, string(artifact.ArtifactType), artifact.TargetNodeID, artifact.Path, artifact.MetadataJSON); err != nil {
			return err
		}
	}

	return tx.Commit()
}

func (s *Store) UpsertArtifact(artifact reviewgraph.Artifact) error {
	_, err := s.db.Exec(`insert into artifacts(id, snapshot_id, artifact_type, target_node_id, path, metadata_json) values(?, ?, ?, ?, ?, ?)
	on conflict(id) do update set snapshot_id = excluded.snapshot_id, artifact_type = excluded.artifact_type, target_node_id = excluded.target_node_id, path = excluded.path, metadata_json = excluded.metadata_json`,
		artifact.ID, artifact.SnapshotID, string(artifact.ArtifactType), artifact.TargetNodeID, artifact.Path, artifact.MetadataJSON)
	return err
}

func (s *Store) ReplaceDerivedForAnchor(snapshotID, anchorTargetID string, nodes []reviewgraph.DerivedNode, edges []reviewgraph.DerivedEdge) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`delete from derived_edges where snapshot_id = ? and anchor_target_id = ?`, snapshotID, anchorTargetID); err != nil {
		return err
	}
	if _, err := tx.Exec(`delete from derived_nodes where snapshot_id = ? and anchor_target_id = ?`, snapshotID, anchorTargetID); err != nil {
		return err
	}
	nodeStmt, err := tx.Prepare(`insert into derived_nodes(id, snapshot_id, anchor_target_id, kind, label, metadata_json) values(?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	defer nodeStmt.Close()
	for _, node := range nodes {
		if _, err := nodeStmt.Exec(node.ID, node.SnapshotID, node.AnchorTargetID, string(node.Kind), node.Label, node.MetadataJSON); err != nil {
			return err
		}
	}
	edgeStmt, err := tx.Prepare(`insert into derived_edges(id, snapshot_id, anchor_target_id, src_id, dst_id, edge_type, metadata_json) values(?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	defer edgeStmt.Close()
	for _, edge := range edges {
		if _, err := edgeStmt.Exec(edge.ID, edge.SnapshotID, edge.AnchorTargetID, edge.SrcID, edge.DstID, string(edge.EdgeType), edge.MetadataJSON); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) SnapshotID() (string, error) {
	for _, query := range []string{
		`select snapshot_id from nodes limit 1`,
		`select snapshot_id from edges limit 1`,
		`select snapshot_id from artifacts limit 1`,
	} {
		var snapshotID string
		err := s.db.QueryRow(query).Scan(&snapshotID)
		if err == nil {
			return snapshotID, nil
		}
		if errors.Is(err, sql.ErrNoRows) {
			continue
		}
		return "", err
	}
	return "", sql.ErrNoRows
}

func (s *Store) ListNodes() ([]reviewgraph.Node, error) {
	rows, err := s.db.Query(`select id, snapshot_id, repo, service, language, kind, symbol, file_path, start_line, end_line, signature, visibility, node_role, metadata_json from nodes order by id`)
	if err != nil {
		return nil, err
	}
	return scanNodes(rows)
}

func (s *Store) ListEdges() ([]reviewgraph.Edge, error) {
	rows, err := s.db.Query(`select id, snapshot_id, src_id, dst_id, edge_type, flow_kind, confidence, evidence_file, evidence_line, evidence_text, transport, topic_or_channel, metadata_json from edges order by id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []reviewgraph.Edge{}
	for rows.Next() {
		var edge reviewgraph.Edge
		var edgeType, flowKind string
		if err := rows.Scan(&edge.ID, &edge.SnapshotID, &edge.SrcID, &edge.DstID, &edgeType, &flowKind, &edge.Confidence, &edge.EvidenceFile, &edge.EvidenceLine, &edge.EvidenceText, &edge.Transport, &edge.TopicOrChannel, &edge.MetadataJSON); err != nil {
			return nil, err
		}
		edge.EdgeType = reviewgraph.EdgeType(edgeType)
		edge.FlowKind = reviewgraph.FlowKind(flowKind)
		result = append(result, edge)
	}
	return result, rows.Err()
}

func (s *Store) ListArtifacts() ([]reviewgraph.Artifact, error) {
	rows, err := s.db.Query(`select id, snapshot_id, artifact_type, target_node_id, path, metadata_json from artifacts order by id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []reviewgraph.Artifact{}
	for rows.Next() {
		var artifact reviewgraph.Artifact
		var artifactType string
		if err := rows.Scan(&artifact.ID, &artifact.SnapshotID, &artifactType, &artifact.TargetNodeID, &artifact.Path, &artifact.MetadataJSON); err != nil {
			return nil, err
		}
		artifact.ArtifactType = reviewgraph.ArtifactType(artifactType)
		result = append(result, artifact)
	}
	return result, rows.Err()
}

func (s *Store) ListDerivedNodesByAnchor(anchorTargetID string) ([]reviewgraph.DerivedNode, error) {
	rows, err := s.db.Query(`select id, snapshot_id, anchor_target_id, kind, label, metadata_json from derived_nodes where anchor_target_id = ? order by kind, label, id`, anchorTargetID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []reviewgraph.DerivedNode{}
	for rows.Next() {
		var node reviewgraph.DerivedNode
		var kind string
		if err := rows.Scan(&node.ID, &node.SnapshotID, &node.AnchorTargetID, &kind, &node.Label, &node.MetadataJSON); err != nil {
			return nil, err
		}
		node.Kind = reviewgraph.NodeKind(kind)
		result = append(result, node)
	}
	return result, rows.Err()
}

func (s *Store) ListDerivedEdgesByAnchor(anchorTargetID string) ([]reviewgraph.DerivedEdge, error) {
	rows, err := s.db.Query(`select id, snapshot_id, anchor_target_id, src_id, dst_id, edge_type, metadata_json from derived_edges where anchor_target_id = ? order by edge_type, src_id, dst_id, id`, anchorTargetID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []reviewgraph.DerivedEdge{}
	for rows.Next() {
		var edge reviewgraph.DerivedEdge
		var edgeType string
		if err := rows.Scan(&edge.ID, &edge.SnapshotID, &edge.AnchorTargetID, &edge.SrcID, &edge.DstID, &edgeType, &edge.MetadataJSON); err != nil {
			return nil, err
		}
		edge.EdgeType = reviewgraph.EdgeType(edgeType)
		result = append(result, edge)
	}
	return result, rows.Err()
}

func (s *Store) FindNodeByID(id string) (reviewgraph.Node, error) {
	row := s.db.QueryRow(`select id, snapshot_id, repo, service, language, kind, symbol, file_path, start_line, end_line, signature, visibility, node_role, metadata_json from nodes where id = ?`, id)
	var node reviewgraph.Node
	var kind, role string
	if err := row.Scan(&node.ID, &node.SnapshotID, &node.Repo, &node.Service, &node.Language, &kind, &node.Symbol, &node.FilePath, &node.StartLine, &node.EndLine, &node.Signature, &node.Visibility, &role, &node.MetadataJSON); err != nil {
		return reviewgraph.Node{}, err
	}
	node.Kind = reviewgraph.NodeKind(kind)
	node.NodeRole = reviewgraph.NodeRole(role)
	return node, nil
}

func (s *Store) FindNodesBySymbol(symbol string) ([]reviewgraph.Node, error) {
	rows, err := s.db.Query(`select id, snapshot_id, repo, service, language, kind, symbol, file_path, start_line, end_line, signature, visibility, node_role, metadata_json from nodes where symbol = ? or lower(symbol) = lower(?) order by file_path, id`, symbol, symbol)
	if err != nil {
		return nil, err
	}
	return scanNodes(rows)
}

func (s *Store) FindNodesByFile(filePath string) ([]reviewgraph.Node, error) {
	rows, err := s.db.Query(`select id, snapshot_id, repo, service, language, kind, symbol, file_path, start_line, end_line, signature, visibility, node_role, metadata_json from nodes where file_path = ? order by kind, symbol, id`, filepath.ToSlash(filePath))
	if err != nil {
		return nil, err
	}
	return scanNodes(rows)
}

func (s *Store) FindNodesByKinds(kinds ...reviewgraph.NodeKind) ([]reviewgraph.Node, error) {
	if len(kinds) == 0 {
		return nil, nil
	}
	query := `select id, snapshot_id, repo, service, language, kind, symbol, file_path, start_line, end_line, signature, visibility, node_role, metadata_json from nodes where kind in (` + placeholders(len(kinds)) + `) order by kind, symbol, id`
	args := make([]any, 0, len(kinds))
	for _, kind := range kinds {
		args = append(args, string(kind))
	}
	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	return scanNodes(rows)
}

func (s *Store) FindNodesByRoles(roles ...reviewgraph.NodeRole) ([]reviewgraph.Node, error) {
	if len(roles) == 0 {
		return nil, nil
	}
	query := `select id, snapshot_id, repo, service, language, kind, symbol, file_path, start_line, end_line, signature, visibility, node_role, metadata_json from nodes where node_role in (` + placeholders(len(roles)) + `) order by node_role, symbol, id`
	args := make([]any, 0, len(roles))
	for _, role := range roles {
		args = append(args, string(role))
	}
	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	return scanNodes(rows)
}

func (s *Store) FindArtifactsByType(artifactType reviewgraph.ArtifactType) ([]reviewgraph.Artifact, error) {
	rows, err := s.db.Query(`select id, snapshot_id, artifact_type, target_node_id, path, metadata_json from artifacts where artifact_type = ? order by id`, string(artifactType))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []reviewgraph.Artifact{}
	for rows.Next() {
		var artifact reviewgraph.Artifact
		var rawType string
		if err := rows.Scan(&artifact.ID, &artifact.SnapshotID, &rawType, &artifact.TargetNodeID, &artifact.Path, &artifact.MetadataJSON); err != nil {
			return nil, err
		}
		artifact.ArtifactType = reviewgraph.ArtifactType(rawType)
		result = append(result, artifact)
	}
	return result, rows.Err()
}

func (s *Store) FindArtifactByTypeAndTarget(artifactType reviewgraph.ArtifactType, targetNodeID string) (reviewgraph.Artifact, error) {
	row := s.db.QueryRow(`select id, snapshot_id, artifact_type, target_node_id, path, metadata_json from artifacts where artifact_type = ? and target_node_id = ? limit 1`, string(artifactType), targetNodeID)
	var artifact reviewgraph.Artifact
	var rawType string
	if err := row.Scan(&artifact.ID, &artifact.SnapshotID, &rawType, &artifact.TargetNodeID, &artifact.Path, &artifact.MetadataJSON); err != nil {
		return reviewgraph.Artifact{}, err
	}
	artifact.ArtifactType = reviewgraph.ArtifactType(rawType)
	return artifact, nil
}

func (s *Store) FindArtifactByTypeAndPath(artifactType reviewgraph.ArtifactType, path string) (reviewgraph.Artifact, error) {
	row := s.db.QueryRow(`select id, snapshot_id, artifact_type, target_node_id, path, metadata_json from artifacts where artifact_type = ? and path = ? limit 1`, string(artifactType), filepath.Clean(path))
	var artifact reviewgraph.Artifact
	var rawType string
	if err := row.Scan(&artifact.ID, &artifact.SnapshotID, &rawType, &artifact.TargetNodeID, &artifact.Path, &artifact.MetadataJSON); err != nil {
		return reviewgraph.Artifact{}, err
	}
	artifact.ArtifactType = reviewgraph.ArtifactType(rawType)
	return artifact, nil
}

func (s *Store) migrate() error {
	statements := []string{
		`create table if not exists nodes(id text primary key, snapshot_id text not null, repo text not null, service text, language text not null, kind text not null, symbol text not null, file_path text not null, start_line integer, end_line integer, signature text, visibility text, node_role text, metadata_json text)`,
		`create table if not exists edges(id text primary key, snapshot_id text not null, src_id text not null, dst_id text not null, edge_type text not null, flow_kind text not null, confidence real, evidence_file text, evidence_line integer, evidence_text text, transport text, topic_or_channel text, metadata_json text)`,
		`create table if not exists artifacts(id text primary key, snapshot_id text not null, artifact_type text not null, target_node_id text, path text not null, metadata_json text)`,
		`create table if not exists derived_nodes(id text primary key, snapshot_id text not null, anchor_target_id text not null, kind text not null, label text not null, metadata_json text not null)`,
		`create table if not exists derived_edges(id text primary key, snapshot_id text not null, anchor_target_id text not null, src_id text not null, dst_id text not null, edge_type text not null, metadata_json text not null)`,
		`create index if not exists idx_review_nodes_symbol on nodes(symbol)`,
		`create index if not exists idx_review_nodes_file_path on nodes(file_path)`,
		`create index if not exists idx_review_nodes_service on nodes(service)`,
		`create index if not exists idx_review_nodes_role on nodes(node_role)`,
		`create index if not exists idx_review_edges_src on edges(src_id)`,
		`create index if not exists idx_review_edges_dst on edges(dst_id)`,
		`create index if not exists idx_review_edges_type on edges(edge_type)`,
		`create index if not exists idx_review_edges_flow on edges(flow_kind)`,
		`create index if not exists idx_review_edges_topic on edges(topic_or_channel)`,
		`create index if not exists idx_review_derived_nodes_snapshot on derived_nodes(snapshot_id)`,
		`create index if not exists idx_review_derived_nodes_anchor on derived_nodes(anchor_target_id)`,
		`create index if not exists idx_review_derived_edges_snapshot on derived_edges(snapshot_id)`,
		`create index if not exists idx_review_derived_edges_anchor on derived_edges(anchor_target_id)`,
		`create index if not exists idx_review_derived_edges_src on derived_edges(src_id)`,
		`create index if not exists idx_review_derived_edges_dst on derived_edges(dst_id)`,
	}
	for _, statement := range statements {
		if _, err := s.db.Exec(statement); err != nil {
			return err
		}
	}
	return nil
}

func scanNodes(rows *sql.Rows) ([]reviewgraph.Node, error) {
	defer rows.Close()
	result := []reviewgraph.Node{}
	for rows.Next() {
		var node reviewgraph.Node
		var kind, role string
		if err := rows.Scan(&node.ID, &node.SnapshotID, &node.Repo, &node.Service, &node.Language, &kind, &node.Symbol, &node.FilePath, &node.StartLine, &node.EndLine, &node.Signature, &node.Visibility, &role, &node.MetadataJSON); err != nil {
			return nil, err
		}
		node.Kind = reviewgraph.NodeKind(kind)
		node.NodeRole = reviewgraph.NodeRole(role)
		result = append(result, node)
	}
	return result, rows.Err()
}

func placeholders(count int) string {
	values := make([]string, 0, count)
	for i := 0; i < count; i++ {
		values = append(values, "?")
	}
	return strings.Join(values, ",")
}

func EncodeJSON(payload any) string {
	if payload == nil {
		return ""
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return ""
	}
	return string(data)
}
