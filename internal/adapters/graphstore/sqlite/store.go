package sqlite

import (
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"time"

	"analysis-module/internal/domain/graph"
	"analysis-module/internal/domain/symbol"
	graphstoreport "analysis-module/internal/ports/graphstore"

	_ "modernc.org/sqlite"
)

type Store struct {
	db *sql.DB
}

func New(path string) (graphstoreport.Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	store := &Store{db: db}
	if err := store.migrate(); err != nil {
		return nil, err
	}
	return store, nil
}

func (s *Store) SaveSnapshot(snapshot graph.GraphSnapshot) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	metadata, err := json.Marshal(snapshot.Metadata)
	if err != nil {
		return err
	}
	if _, err := tx.Exec(`insert or replace into snapshots(id, workspace_id, created_at, metadata_json) values(?, ?, ?, ?)`, snapshot.ID, snapshot.WorkspaceID, snapshot.CreatedAt.UTC().Format(timeFormat), string(metadata)); err != nil {
		return err
	}
	if _, err := tx.Exec(`delete from nodes where snapshot_id = ?`, snapshot.ID); err != nil {
		return err
	}
	if _, err := tx.Exec(`delete from edges where snapshot_id = ?`, snapshot.ID); err != nil {
		return err
	}
	for _, node := range snapshot.Nodes {
		locationJSON, _ := json.Marshal(node.Location)
		propertiesJSON, _ := json.Marshal(node.Properties)
		if _, err := tx.Exec(`insert into nodes(id, snapshot_id, kind, canonical_name, language, repository_id, file_path, location_json, properties_json) values(?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			node.ID, node.SnapshotID, string(node.Kind), node.CanonicalName, node.Language, node.RepositoryID, node.FilePath, string(locationJSON), string(propertiesJSON)); err != nil {
			return err
		}
	}
	for _, edge := range snapshot.Edges {
		evidenceJSON, _ := json.Marshal(edge.Evidence)
		confidenceJSON, _ := json.Marshal(edge.Confidence)
		propertiesJSON, _ := json.Marshal(edge.Properties)
		if _, err := tx.Exec(`insert into edges(id, snapshot_id, kind, from_node, to_node, evidence_json, confidence_json, properties_json) values(?, ?, ?, ?, ?, ?, ?, ?)`,
			edge.ID, edge.SnapshotID, string(edge.Kind), edge.From, edge.To, string(evidenceJSON), string(confidenceJSON), string(propertiesJSON)); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) GetSnapshot(snapshotID string) (graph.GraphSnapshot, error) {
	row := s.db.QueryRow(`select workspace_id, created_at, metadata_json from snapshots where id = ?`, snapshotID)
	var workspaceID, createdAt, metadataJSON string
	if err := row.Scan(&workspaceID, &createdAt, &metadataJSON); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return graph.GraphSnapshot{}, err
		}
		return graph.GraphSnapshot{}, err
	}
	nodes, err := s.GetNodes(snapshotID)
	if err != nil {
		return graph.GraphSnapshot{}, err
	}
	edges, err := s.GetEdges(snapshotID)
	if err != nil {
		return graph.GraphSnapshot{}, err
	}
	metadata := graph.SnapshotMetadata{}
	_ = json.Unmarshal([]byte(metadataJSON), &metadata)
	return graph.GraphSnapshot{
		ID:          snapshotID,
		WorkspaceID: workspaceID,
		CreatedAt:   parseTime(createdAt),
		Nodes:       nodes,
		Edges:       edges,
		Metadata:    metadata,
	}, nil
}

func (s *Store) GetNodes(snapshotID string) ([]graph.Node, error) {
	rows, err := s.db.Query(`select id, kind, canonical_name, language, repository_id, file_path, location_json, properties_json from nodes where snapshot_id = ?`, snapshotID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	nodes := []graph.Node{}
	for rows.Next() {
		var node graph.Node
		var kind, locationJSON, propertiesJSON string
		if err := rows.Scan(&node.ID, &kind, &node.CanonicalName, &node.Language, &node.RepositoryID, &node.FilePath, &locationJSON, &propertiesJSON); err != nil {
			return nil, err
		}
		node.Kind = graph.NodeKind(kind)
		node.SnapshotID = snapshotID
		if locationJSON != "" && locationJSON != "null" {
			location := &symbol.CodeLocation{}
			if err := json.Unmarshal([]byte(locationJSON), location); err == nil {
				node.Location = location
			}
		}
		if propertiesJSON != "" && propertiesJSON != "null" {
			_ = json.Unmarshal([]byte(propertiesJSON), &node.Properties)
		}
		nodes = append(nodes, node)
	}
	return nodes, rows.Err()
}

func (s *Store) GetEdges(snapshotID string) ([]graph.Edge, error) {
	rows, err := s.db.Query(`select id, kind, from_node, to_node, evidence_json, confidence_json, properties_json from edges where snapshot_id = ?`, snapshotID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	edges := []graph.Edge{}
	for rows.Next() {
		var edge graph.Edge
		var kind, evidenceJSON, confidenceJSON, propertiesJSON string
		if err := rows.Scan(&edge.ID, &kind, &edge.From, &edge.To, &evidenceJSON, &confidenceJSON, &propertiesJSON); err != nil {
			return nil, err
		}
		edge.Kind = graph.EdgeKind(kind)
		edge.SnapshotID = snapshotID
		_ = json.Unmarshal([]byte(evidenceJSON), &edge.Evidence)
		_ = json.Unmarshal([]byte(confidenceJSON), &edge.Confidence)
		if propertiesJSON != "" && propertiesJSON != "null" {
			_ = json.Unmarshal([]byte(propertiesJSON), &edge.Properties)
		}
		edges = append(edges, edge)
	}
	return edges, rows.Err()
}

func (s *Store) FindNode(snapshotID, canonicalName string) (graph.Node, error) {
	row := s.db.QueryRow(`select id, kind, canonical_name, language, repository_id, file_path, location_json, properties_json from nodes where snapshot_id = ? and (canonical_name = ? or id = ?) limit 1`, snapshotID, canonicalName, canonicalName)
	var node graph.Node
	var kind, locationJSON, propertiesJSON string
	if err := row.Scan(&node.ID, &kind, &node.CanonicalName, &node.Language, &node.RepositoryID, &node.FilePath, &locationJSON, &propertiesJSON); err != nil {
		return graph.Node{}, err
	}
	node.Kind = graph.NodeKind(kind)
	node.SnapshotID = snapshotID
	if locationJSON != "" && locationJSON != "null" {
		location := &symbol.CodeLocation{}
		if err := json.Unmarshal([]byte(locationJSON), location); err == nil {
			node.Location = location
		}
	}
	if propertiesJSON != "" && propertiesJSON != "null" {
		_ = json.Unmarshal([]byte(propertiesJSON), &node.Properties)
	}
	return node, nil
}

func (s *Store) migrate() error {
	statements := []string{
		`create table if not exists snapshots(id text primary key, workspace_id text not null, created_at text not null, metadata_json text not null)`,
		`create table if not exists nodes(id text not null, snapshot_id text not null, kind text not null, canonical_name text not null, language text, repository_id text, file_path text, location_json text, properties_json text, primary key(id, snapshot_id))`,
		`create table if not exists edges(id text not null, snapshot_id text not null, kind text not null, from_node text not null, to_node text not null, evidence_json text not null, confidence_json text not null, properties_json text, primary key(id, snapshot_id))`,
		`create index if not exists idx_nodes_snapshot_name on nodes(snapshot_id, canonical_name)`,
		`create index if not exists idx_edges_snapshot_from on edges(snapshot_id, from_node)`,
		`create index if not exists idx_edges_snapshot_to on edges(snapshot_id, to_node)`,
	}
	for _, statement := range statements {
		if _, err := s.db.Exec(statement); err != nil {
			return err
		}
	}
	return nil
}

const timeFormat = "2006-01-02T15:04:05.999999999Z07:00"

func parseTime(raw string) time.Time {
	parsed, err := time.Parse(timeFormat, raw)
	if err != nil {
		return time.Time{}
	}
	return parsed
}
