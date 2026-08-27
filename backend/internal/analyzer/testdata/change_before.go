package fixture

func Keep() {
	println("old")
}

func Delete() {}

type Service struct{}

func (s *Service) Run() {
	println("old")
}
