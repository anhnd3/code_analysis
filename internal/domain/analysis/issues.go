package analysis

type IssueCounts struct {
	UnresolvedImports             int `json:"unresolved_imports"`
	AmbiguousRelations            int `json:"ambiguous_relations"`
	UnsupportedConstructs         int `json:"unsupported_constructs"`
	SkippedIgnoredFiles           int `json:"skipped_ignored_files"`
	DeferredBoundaryStitching     int `json:"deferred_boundary_stitching"`
	ServiceAttributionAmbiguities int `json:"service_attribution_ambiguities"`
}

func (c *IssueCounts) Add(other IssueCounts) {
	c.UnresolvedImports += other.UnresolvedImports
	c.AmbiguousRelations += other.AmbiguousRelations
	c.UnsupportedConstructs += other.UnsupportedConstructs
	c.SkippedIgnoredFiles += other.SkippedIgnoredFiles
	c.DeferredBoundaryStitching += other.DeferredBoundaryStitching
	c.ServiceAttributionAmbiguities += other.ServiceAttributionAmbiguities
}
