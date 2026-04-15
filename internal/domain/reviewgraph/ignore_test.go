package reviewgraph

import "testing"

func TestIgnoreRulesMatchTreatsGeneratedProtoRuntimeAsGenerated(t *testing.T) {
	rules := IgnoreRules{}
	cases := []string{
		"pkg/proto/scan_service.pb.go",
		"pkg/proto/scan_service_grpc.pb.go",
		"proto/gen/scan_service.pb.gw.go",
		"proto_gen/scan_service.pb.go",
	}
	for _, path := range cases {
		matched, generated := rules.Match(path)
		if !matched || !generated {
			t.Fatalf("expected %s to be treated as generated, got matched=%v generated=%v", path, matched, generated)
		}
	}
}

func TestIgnoreRulesMatchTreatsTestFilesAsIgnoredNotGenerated(t *testing.T) {
	rules := IgnoreRules{}
	matched, generated := rules.Match("service/foo_test.go")
	if !matched {
		t.Fatal("expected test file to be ignored")
	}
	if generated {
		t.Fatal("expected test file to be ignored without being marked generated")
	}
}
