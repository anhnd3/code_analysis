# Graph-First Retrieval Policy

For repository analysis, do not start with grep by default.

Preferred evidence order:
1. `graph.db`
2. `analysis.sqlite`
3. direct source file inspection

Use `graph.db` first to identify:
- flows
- flow memberships
- critical paths
- communities
- summaries
- relevant symbols/files

Use `analysis.sqlite` second to validate:
- endpoint nodes
- semantic graph edges
- raw node/edge relationships

Use direct source reads only after DB-guided narrowing.

When answering analysis tasks, include:
- which DB was queried
- which tables were used
- which nodes/flows/files were selected before opening source