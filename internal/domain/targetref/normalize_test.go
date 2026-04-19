package targetref

import "testing"

func TestPackageTokenFromCanonical(t *testing.T) {
	tests := map[string]string{
		"handler_v2.cameraV2Handler.DetectQR": "handler_v2",
		"camerarepo.DetectQR":                 "camerarepo",
		"":                                    "",
	}
	for raw, want := range tests {
		if got := PackageTokenFromCanonical(raw); got != want {
			t.Fatalf("canonical %q: expected %q, got %q", raw, want, got)
		}
	}
}

func TestParsePackageMethodHint(t *testing.T) {
	ref, ok := ParsePackageMethodHint("repository_v2.DetectQR")
	if !ok {
		t.Fatal("expected package method hint to parse")
	}
	if ref.PackageToken != "repository_v2" || ref.MethodName != "DetectQR" {
		t.Fatalf("unexpected ref %+v", ref)
	}
	if _, ok := ParsePackageMethodHint("handler_v2.cameraV2Handler.DetectQR"); ok {
		t.Fatal("expected exact canonical to stay distinct from package method hint")
	}
}

func TestPackageTokenFromTypeText(t *testing.T) {
	tests := []struct {
		raw        string
		currentPkg string
		want       string
	}{
		{raw: "*repository_v2.CameraV2Repository", currentPkg: "handler_v2", want: "repository_v2"},
		{raw: "session.ISessionService", currentPkg: "handler", want: "session"},
		{raw: "[]CameraRepo", currentPkg: "camerahandler", want: "camerahandler"},
		{raw: "map[string]*repo.CameraRepo", currentPkg: "handler", want: "handler"},
	}
	for _, tc := range tests {
		if got := PackageTokenFromTypeText(tc.raw, tc.currentPkg); got != tc.want {
			t.Fatalf("type %q: expected %q, got %q", tc.raw, tc.want, got)
		}
	}
}
