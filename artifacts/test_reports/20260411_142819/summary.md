# Test Report

## Overall Coverage

| Status | Passed | Failed | Skipped | Modules | Files | Covered Statements | Total Statements | Coverage |
| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| PASS | 11 | 0 | 0 | 31 | 33 | 1099 | 8651 | 12.7% |

## Module Coverage

| Module | Files | Covered Statements | Total Statements | Coverage |
| --- | ---: | ---: | ---: | ---: |
| `cmd/analysis-cli` | 1 | 0 | 620 | 0.0% |
| `cmd/analysisd` | 1 | 0 | 100 | 0.0% |
| `internal/adapters/api/http` | 1 | 17 | 540 | 3.1% |
| `internal/adapters/artifactstore/filesystem` | 1 | 44 | 306 | 14.4% |
| `internal/adapters/extractor/go` | 1 | 135 | 900 | 15.0% |
| `internal/adapters/extractor/treesitter` | 1 | 15 | 54 | 27.8% |
| `internal/adapters/graphstore/memory` | 1 | 4 | 80 | 5.0% |
| `internal/adapters/graphstore/sqlite` | 1 | 144 | 990 | 14.5% |
| `internal/adapters/scanner/detectors` | 3 | 179 | 828 | 21.6% |
| `internal/adapters/scanner/filesystem` | 1 | 18 | 260 | 6.9% |
| `internal/app/bootstrap` | 1 | 16 | 170 | 9.4% |
| `internal/app/config` | 1 | 7 | 110 | 6.4% |
| `internal/app/errors` | 1 | 0 | 110 | 0.0% |
| `internal/app/logging` | 1 | 1 | 10 | 10.0% |
| `internal/services/graph_build` | 1 | 185 | 918 | 20.2% |
| `internal/services/graph_query` | 1 | 44 | 530 | 8.3% |
| `internal/services/packet_build` | 1 | 0 | 20 | 0.0% |
| `internal/services/quality_report` | 1 | 19 | 230 | 8.3% |
| `internal/services/repo_inventory` | 1 | 30 | 189 | 15.9% |
| `internal/services/repomap_build` | 1 | 0 | 20 | 0.0% |
| `internal/services/snapshot_manage` | 1 | 3 | 30 | 10.0% |
| `internal/services/symbol_index` | 1 | 20 | 230 | 8.7% |
| `internal/services/workspace_scan` | 1 | 97 | 576 | 16.8% |
| `internal/tests/fixtures` | 1 | 44 | 130 | 33.8% |
| `internal/workflows/analyze_workspace` | 1 | 26 | 270 | 9.6% |
| `internal/workflows/blast_radius` | 1 | 7 | 80 | 8.8% |
| `internal/workflows/build_packet` | 1 | 0 | 10 | 0.0% |
| `internal/workflows/build_repomap` | 1 | 0 | 10 | 0.0% |
| `internal/workflows/build_snapshot` | 1 | 17 | 200 | 8.5% |
| `internal/workflows/impacted_tests` | 1 | 7 | 80 | 8.8% |
| `pkg/ids` | 1 | 20 | 50 | 40.0% |

## File Coverage

| Module | File | Covered Statements | Total Statements | Coverage |
| --- | --- | ---: | ---: | ---: |
| `cmd/analysis-cli` | `main.go` | 0 | 620 | 0.0% |
| `cmd/analysisd` | `main.go` | 0 | 100 | 0.0% |
| `internal/adapters/api/http` | `handler.go` | 17 | 540 | 3.1% |
| `internal/adapters/artifactstore/filesystem` | `store.go` | 44 | 306 | 14.4% |
| `internal/adapters/extractor/go` | `extractor.go` | 135 | 900 | 15.0% |
| `internal/adapters/extractor/treesitter` | `parser.go` | 15 | 54 | 27.8% |
| `internal/adapters/graphstore/memory` | `cache.go` | 4 | 80 | 5.0% |
| `internal/adapters/graphstore/sqlite` | `store.go` | 144 | 990 | 14.5% |
| `internal/adapters/scanner/detectors` | `repo_root.go` | 36 | 189 | 19.0% |
| `internal/adapters/scanner/detectors` | `service.go` | 30 | 180 | 16.7% |
| `internal/adapters/scanner/detectors` | `techstack.go` | 113 | 459 | 24.6% |
| `internal/adapters/scanner/filesystem` | `walker.go` | 18 | 260 | 6.9% |
| `internal/app/bootstrap` | `app.go` | 16 | 170 | 9.4% |
| `internal/app/config` | `config.go` | 7 | 110 | 6.4% |
| `internal/app/errors` | `errors.go` | 0 | 110 | 0.0% |
| `internal/app/logging` | `logging.go` | 1 | 10 | 10.0% |
| `internal/services/graph_build` | `service.go` | 185 | 918 | 20.2% |
| `internal/services/graph_query` | `service.go` | 44 | 530 | 8.3% |
| `internal/services/packet_build` | `service.go` | 0 | 20 | 0.0% |
| `internal/services/quality_report` | `service.go` | 19 | 230 | 8.3% |
| `internal/services/repo_inventory` | `service.go` | 30 | 189 | 15.9% |
| `internal/services/repomap_build` | `service.go` | 0 | 20 | 0.0% |
| `internal/services/snapshot_manage` | `service.go` | 3 | 30 | 10.0% |
| `internal/services/symbol_index` | `service.go` | 20 | 230 | 8.7% |
| `internal/services/workspace_scan` | `service.go` | 97 | 576 | 16.8% |
| `internal/tests/fixtures` | `path.go` | 44 | 130 | 33.8% |
| `internal/workflows/analyze_workspace` | `workflow.go` | 26 | 270 | 9.6% |
| `internal/workflows/blast_radius` | `workflow.go` | 7 | 80 | 8.8% |
| `internal/workflows/build_packet` | `workflow.go` | 0 | 10 | 0.0% |
| `internal/workflows/build_repomap` | `workflow.go` | 0 | 10 | 0.0% |
| `internal/workflows/build_snapshot` | `workflow.go` | 17 | 200 | 8.5% |
| `internal/workflows/impacted_tests` | `workflow.go` | 7 | 80 | 8.8% |
| `pkg/ids` | `ids.go` | 20 | 50 | 40.0% |
