package targetref

type Kind string

const (
	KindUnknown           Kind = ""
	KindExactSymbolID     Kind = "exact_symbol_id"
	KindExactCanonical    Kind = "exact_canonical"
	KindPackageMethodHint Kind = "package_method_hint"
)
