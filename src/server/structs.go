package server

var (
//Users []*User
)

type Login struct {
	Title string
	Info  string
}

type resp struct {
	Images   []ImageType `json:"images"`
	ToolTips ToolTipType `json:"-"`
}

type ImageType struct {
	StartDate     string `json:"startdate"`
	FullStartDate string `json:"fullstartdate"`
	Url           string `json:"url"`
	UrlBase       string `json:"urlbase"`
	CopyRight     string `json:"copyright"`
	CopyRightLink string `json:"copyrightlink"`
	Title         string `json:"title"`
	Quiz          string `json:"quiz"`
	Wp            bool   `json:"wp"`
	Hsh           string `json:"hsh"`
	Drk           int    `json:"drk"`
	Top           int    `json:"top"`
	Bot           int    `json:"bot"`
	//Hs  unknown type `json:"-"`
}

type ToolTipType struct {
	Loading  string `json:"loading"`
	Previous string `json:"previous"`
	Next     string `json:"next"`
	Walle    string `json:"walle"`
	Walls    string `json:"walls"`
}

//func (u *User) Register() error {
//	if dao.ExistUser(u) {
//		return errors.New("This user has exist!")
//	} else {
//		return dao.Register(u)
//	}
//}
