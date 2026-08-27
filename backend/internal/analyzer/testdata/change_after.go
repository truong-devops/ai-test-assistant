package fixture

func Keep() {
	println("new")
}

func Add() {}

type Service struct{}

func (s *Service) Run() {
	println("new")
}
