package sqlite

import (
	"fmt"
	"path/filepath"
	"sync"

	graphstoreport "analysis-module/internal/ports/graphstore"
)

type Provider struct {
	root         string
	overridePath string
	mu           sync.Mutex
	stores       map[string]graphstoreport.Store
}

func NewProvider(root, overridePath string) graphstoreport.Provider {
	return &Provider{
		root:         root,
		overridePath: overridePath,
		stores:       map[string]graphstoreport.Store{},
	}
}

func (p *Provider) ForWorkspace(workspaceID string) (graphstoreport.Store, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if workspaceID == "" {
		return nil, fmt.Errorf("workspace id is required")
	}
	if store, ok := p.stores[workspaceID]; ok {
		return store, nil
	}
	path := p.overridePath
	if path == "" {
		path = filepath.Join(p.root, "workspaces", workspaceID, "analysis.sqlite")
	}
	store, err := New(path)
	if err != nil {
		return nil, err
	}
	p.stores[workspaceID] = store
	return store, nil
}
