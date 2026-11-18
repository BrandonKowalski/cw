package artwork

func SetMaxIthmbSize(n int) func() {
	old := maxIthmbSize
	maxIthmbSize = n
	return func() { maxIthmbSize = old }
}
