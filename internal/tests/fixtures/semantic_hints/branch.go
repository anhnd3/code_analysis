package handler

func BranchOnCondition() bool {
	if true {
		return leftPath()
	} else {
		return rightPath()
	}
}

func leftPath() bool  { return false }
func rightPath() bool { return false }
