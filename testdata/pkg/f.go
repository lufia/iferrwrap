package pkg

func (t *T) PtrMethod1() {
}

func (t T) Method2() error {
	return nil
}

func (t *T) PtrMethod3() (int, error) {
	return 0, nil
}

func (t T) Method4() (string, int, error) {
	return "", 0, nil
}

func F() error {
	type T struct{}

	return nil
}
