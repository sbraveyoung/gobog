package user

type User struct {
	Id           int
	Name         string
	Phone        string
	Email        string
	RegisterTime string
}

func NewUser(name, phone, email string) *User {
	return &User{
		Name:  name,
		Phone: phone,
		Email: email,
	}
}
