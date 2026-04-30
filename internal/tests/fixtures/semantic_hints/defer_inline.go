package handler

func DeferInline() {
	defer func() {
		cleanup()
	}()
}

func cleanup() {}
