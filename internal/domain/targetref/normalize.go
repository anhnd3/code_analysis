package targetref

import "strings"

type PackageMethodRef struct {
	PackageToken string
	MethodName   string
}

func NormalizePackageToken(raw string) string {
	raw = strings.TrimSpace(raw)
	raw = strings.Trim(raw, "()")
	raw = strings.TrimLeft(raw, "*")
	if idx := strings.Index(raw, "["); idx >= 0 {
		raw = raw[:idx]
	}
	raw = strings.TrimSpace(raw)
	return raw
}

func PackageTokenFromCanonical(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	parts := strings.Split(raw, ".")
	if len(parts) < 2 {
		return ""
	}
	return NormalizePackageToken(parts[0])
}

func PackageTokenFromTypeText(raw, currentPkg string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return NormalizePackageToken(currentPkg)
	}
	raw = strings.Trim(raw, "()")
	for {
		switch {
		case strings.HasPrefix(raw, "*"):
			raw = strings.TrimPrefix(raw, "*")
		case strings.HasPrefix(raw, "[]"):
			raw = strings.TrimPrefix(raw, "[]")
		case strings.HasPrefix(raw, "..."):
			raw = strings.TrimPrefix(raw, "...")
		case strings.HasPrefix(raw, "chan<-"):
			raw = strings.TrimSpace(strings.TrimPrefix(raw, "chan<-"))
		case strings.HasPrefix(raw, "<-chan"):
			raw = strings.TrimSpace(strings.TrimPrefix(raw, "<-chan"))
		case strings.HasPrefix(raw, "chan"):
			raw = strings.TrimSpace(strings.TrimPrefix(raw, "chan"))
		default:
			goto normalized
		}
		raw = strings.TrimSpace(raw)
	}

normalized:
	if idx := strings.Index(raw, "["); idx >= 0 {
		raw = raw[:idx]
	}
	raw = strings.TrimSpace(raw)
	if idx := strings.LastIndex(raw, "."); idx >= 0 {
		return NormalizePackageToken(raw[:idx])
	}
	return NormalizePackageToken(currentPkg)
}

func ParsePackageMethodHint(raw string) (PackageMethodRef, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" || strings.Contains(raw, "/") {
		return PackageMethodRef{}, false
	}
	if strings.Count(raw, ".") != 1 {
		return PackageMethodRef{}, false
	}
	idx := strings.LastIndex(raw, ".")
	if idx <= 0 || idx >= len(raw)-1 {
		return PackageMethodRef{}, false
	}
	ref := PackageMethodRef{
		PackageToken: NormalizePackageToken(raw[:idx]),
		MethodName:   strings.TrimSpace(raw[idx+1:]),
	}
	if ref.PackageToken == "" || ref.MethodName == "" {
		return PackageMethodRef{}, false
	}
	return ref, true
}

func BuildPackageMethodHint(packageToken, methodName string) string {
	packageToken = NormalizePackageToken(packageToken)
	methodName = strings.TrimSpace(methodName)
	if packageToken == "" || methodName == "" {
		return ""
	}
	return packageToken + "." + methodName
}
