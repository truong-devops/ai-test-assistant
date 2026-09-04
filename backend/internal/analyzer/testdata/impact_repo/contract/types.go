package contract

type ID string

type Reader interface {
	Read(ID) string
}

type Box[T any] struct {
	Value T
}

func Normalize(id ID) ID {
	return ID("normalized:" + string(id))
}
