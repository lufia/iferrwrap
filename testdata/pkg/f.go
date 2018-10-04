package pkg

func (t *T) PtrMethod1() {
}

func (t T) Method2(s string) error {
	return nil
}

func (t *T) PtrMethod3(int) (int, error) {
	return 0, nil
}

func (t T) Method4(v interface{}, err error) (string, int, error) {
	return "", 0, nil
}

func F() error {
	type T struct{}

	return nil
}
