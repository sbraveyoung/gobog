package structs

var (
	Users    []*User
	Articles []*Article
)

type Login struct {
	Title string
	Info  string
}

type Article struct {
	Id           int
	FilePath     string
	Url          string
	Title        string
	Desctription string
	Author       string
	Type         string
	Time         string
	Content      string
	Comments     []comment
}

type comment struct {
	Id               int
	ArticleId        int
	Publisher        User //must login
	ReponseCommentId int
	Time             string
}

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
