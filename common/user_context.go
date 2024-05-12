package common


type RuntimeUserData struct {
	Route map[string]string
}

type UserData map[int64]*RuntimeUserData

func (u *UserData) Get(id int64) *RuntimeUserData {
	if result, ok := (*u)[id]; ok {
		return result
	}
	(*u)[id] = &RuntimeUserData{ 
		Route : make(map[string]string),
	}
	return (*u)[id]
}

var UserContext UserData

func init() {
	UserContext = make(UserData)
}
