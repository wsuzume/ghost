package main

import (
	"bufio"
	"fmt"
	"math/rand"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
)

const (
	USTATE_JOIN = iota
	USTATE_WATCH
	USTATE_BATTLE
)

func ustate(s uint) string {
	switch s {
	case USTATE_JOIN:
		return "join"
	case USTATE_WATCH:
		return "watch"
	case USTATE_BATTLE:
		return "battle"
	default:
		break
	}
	return "unrecognized"
}

type User struct {
	Name   string
	Id     uuid.UUID
	Vote   string
	Judge  uint
	Team   uint
	socket *websocket.Conn
	state  uint
}

type UserState struct {
	Name  string `json:"username"`
	Vote  string `json:"vote"`
	Judge uint   `json:"judge"`
	Team  uint   `json:"team"`
	State string `json:"state"`
}

func (u *User) getState() UserState {
	return UserState{
		u.Name,
		u.Vote,
		u.Judge,
		u.Team,
		ustate(u.state),
	}
}

const (
	RSTATE_STANDBY = iota
	RSTATE_BATTLE
	RSTATE_TRIAL
)

func rstate(s uint) string {
	switch s {
	case RSTATE_STANDBY:
		return "standby"
	case RSTATE_BATTLE:
		return "battle"
	case RSTATE_TRIAL:
		return "trial"
	default:
		break
	}
	return "unrecognized"
}

type Room struct {
	Name     string
	Password string
	Users    map[string]*User
	Channel  chan GameResponse
	State    uint
	A        string
	B        string
}

type RoomState struct {
	Name  string `json:"roomname"`
	State string `json:"state"`
	A     string `json:"a"`
	B     string `json:"b"`
}

func (r *Room) broadcast() {
	for {
		message := <-r.Channel
		for username, user := range r.Users {
			err := user.socket.WriteJSON(message)
			if err != nil {
				user.socket.Close()
				delete(r.Users, username)
			}
		}
	}
}

func (r *Room) getState() RoomState {
	return RoomState{
		r.Name,
		rstate(r.State),
		r.A,
		r.B,
	}
}

func (r *Room) getUsernames() []string {
	keys := make([]string, 0, len(r.Users))
	for key := range r.Users {
		keys = append(keys, key)
	}

	return keys
}

func (r *Room) getUserStatus() []UserState {
	ret := make([]UserState, 0, len(r.Users))
	for _, user := range r.Users {
		ret = append(ret, user.getState())
	}

	return ret
}

func (r *Room) startBattle() {
	if r.State == RSTATE_BATTLE {
		return
	}

	r.State = RSTATE_BATTLE

	rand.Seed(time.Now().UnixNano())
	odai_idx := rand.Intn(len(odai))

	x := odai[odai_idx]

	rand.Seed(time.Now().UnixNano())
	if rand.Intn(2) == 0 {
		r.A = x.A
		r.B = x.B
	} else {
		r.A = x.B
		r.B = x.A
	}

	rand.Seed(time.Now().UnixNano())
	oni := rand.Intn(len(r.Users))

	i := 0
	for _, user := range r.Users {
		if user.state == USTATE_JOIN {
			user.state = USTATE_BATTLE
			if i == oni {
				user.Team = 1
			} else {
				user.Team = 0
			}
		}
		i += 1
	}
}

func (r *Room) endBattle() {
	if r.State == RSTATE_STANDBY {
		return
	}

	r.State = RSTATE_STANDBY

	r.A = ""
	r.B = ""

	for _, user := range r.Users {
		if user.state == USTATE_BATTLE {
			user.state = USTATE_JOIN
			user.Vote = ""
			user.Judge = 0
		}
	}
}

type Session struct {
	user *User
	room *Room
}

var roomDatabase map[string]*Room
var sessionStore map[uuid.UUID]*Session

type RoomRequest struct {
	Command      string `json:"command"`
	RoomName     string `json:"room_name"`
	RoomPassword string `json:"room_password"`
	UserName     string `json:"username"`
}

func (req RoomRequest) toRoom() *Room {
	return &Room{
		req.RoomName,
		req.RoomPassword,
		map[string]*User{
			req.UserName: &User{req.UserName, uuid.New(), "", 0, 0, nil, USTATE_JOIN},
		},
		make(chan GameResponse),
		RSTATE_STANDBY,
		"",
		"",
	}
}

type GameRequest struct {
	Command string `json:"command"`
	Meta    string `json:"meta"`
}

type GameResponse struct {
	Command string `json:"command"`
	Meta    gin.H  `json:"meta"`
}

func (room *Room) HandleGameRequest(c *gin.Context, session *Session, req GameRequest) (gin.H, error) {
	var ret gin.H

	user := session.user

	switch req.Command {
	case "update":
		ret = gin.H{
			"command":    "update",
			"user_state": user.getState(),
			"room_state": room.getState(),
			"members":    room.getUserStatus(),
		}
		return ret, nil
	case "start":
		room.startBattle()

		ret = gin.H{
			"command":    "start",
			"user_state": user.getState(),
			"room_state": room.getState(),
			"members":    room.getUserStatus(),
		}
		return ret, nil
	case "end":
		room.endBattle()

		ret = gin.H{
			"command":    "end",
			"user_state": user.getState(),
			"room_state": room.getState(),
			"members":    room.getUserStatus(),
		}
		return ret, nil
	case "join":
		user.state = USTATE_JOIN

		ret = gin.H{
			"command":    "join",
			"user_state": user.getState(),
			"room_state": room.getState(),
			"members":    room.getUserStatus(),
		}
		return ret, nil
	case "watch":
		user.state = USTATE_WATCH

		ret = gin.H{
			"command":    "watch",
			"user_state": user.getState(),
			"room_state": room.getState(),
			"members":    room.getUserStatus(),
		}
		return ret, nil
	case "vote":
		if user.state == USTATE_BATTLE {
			_, ue := room.Users[req.Meta]
			if ue {
				if user.Name != req.Meta {
					user.Vote = req.Meta
				}
			}
		}
		ret = gin.H{
			"command":    "vote",
			"user_state": user.getState(),
			"room_state": room.getState(),
			"members":    room.getUserStatus(),
		}
		return ret, nil
	case "judge":
		if user.state == USTATE_BATTLE {
			user.Judge = 1
		}
		ret = gin.H{
			"command":    "vote",
			"user_state": user.getState(),
			"room_state": room.getState(),
			"members":    room.getUserStatus(),
		}
		return ret, nil
	case "extend":
		ret = gin.H{
			"command":    "vote",
			"user_state": user.getState(),
			"room_state": room.getState(),
			"members":    room.getUserStatus(),
		}
		return ret, nil
	case "exit":
		delete(room.Users, user.Name)
		delete(sessionStore, user.Id)

		if len(room.Users) == 0 {
			delete(roomDatabase, room.Name)
		}

		c.SetCookie("who-is-the-ghost", "", -1, "/", "", false, false)

		ret = gin.H{
			"command":   "exit",
			"exit_user": user.Name,
		}
		return ret, nil

	}

	return gin.H{}, nil
}

func ApiPost() gin.HandlerFunc {
	return func(c *gin.Context) {
		req := RoomRequest{}
		err := c.Bind(&req)

		if err != nil {
			return
		}

		room, exists := roomDatabase[req.RoomName]
		switch req.Command {
		case "create":
			if exists {
				c.JSON(http.StatusBadRequest, gin.H{
					"message": "room already exists",
				})
				return
			}
			room = req.toRoom()
			roomDatabase[req.RoomName] = room
			user := room.Users[req.UserName]
			sessionStore[user.Id] = &Session{user, room}

			c.SetCookie("who-is-the-ghost", user.Id.String(), 300, "/", "", false, false)
			c.JSON(http.StatusOK, gin.H{
				"roomname": req.RoomName,
				"message":  "room created successfully",
			})
			return
		case "enter":
			if !exists {
				c.JSON(http.StatusBadRequest, gin.H{
					"message": "room not exists",
				})
				return
			}
			_, uexists := room.Users[req.UserName]
			if uexists {
				c.JSON(http.StatusBadRequest, gin.H{
					"message": "the username is already used",
				})
				return
			}

			user := &User{req.UserName, uuid.New(), "", 0, 0, nil, USTATE_WATCH}
			room.Users[user.Name] = user
			sessionStore[user.Id] = &Session{user, room}
			c.SetCookie("who-is-the-ghost", user.Id.String(), 300, "/", "", false, false)
			c.JSON(http.StatusOK, gin.H{
				"roomname": req.RoomName,
				"message":  "user registered successfully",
			})
			return
		default:
			break
		}

		c.JSON(http.StatusBadRequest, gin.H{
			"message": "request unrecognized",
		})
	}
}

var upgrader = websocket.Upgrader{}

func GamePost() gin.HandlerFunc {
	return func(c *gin.Context) {
		req := GameRequest{}
		err := c.Bind(&req)

		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"message": "request unrecognized",
			})
			return
		}

		userid, err := c.Cookie("who-is-the-ghost")
		if err != nil {
			fmt.Println(err)
			c.JSON(http.StatusBadRequest, gin.H{
				"message": "invalid user",
			})
			return
		}

		uid, err := uuid.Parse(userid)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"message": "invalid user",
			})
			return
		}

		session, uexists := sessionStore[uid]
		if !uexists {
			c.JSON(http.StatusBadRequest, gin.H{
				"message": "invalid user",
			})
			return
		}

		room := session.room

		meta, err := room.HandleGameRequest(c, session, req)

		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"command": req.Command,
			"meta":    meta,
		})

	}
}

func GameSocket() gin.HandlerFunc {
	return func(c *gin.Context) {
		fmt.Println("here")
		userid, err := c.Cookie("who-is-the-ghost")
		if err != nil {
			fmt.Println(err)
			c.JSON(http.StatusBadRequest, gin.H{
				"message": "invalid user",
			})
			return
		}

		uid, err := uuid.Parse(userid)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"message": "invalid user",
			})
			return
		}

		session, uexists := sessionStore[uid]
		if !uexists {
			c.JSON(http.StatusBadRequest, gin.H{
				"message": "invalid user",
			})
			return
		}

		user := session.user
		room := session.room

		websocket, err := upgrader.Upgrade(c.Writer, c.Request, nil)
		if err != nil {
			fmt.Println(err)
		}
		defer websocket.Close()

		room.Users[user.Name].socket = websocket

		go room.broadcast()

		for {
			var message GameRequest
			err := websocket.ReadJSON(&message)
			if err != nil {
				fmt.Println("cloooooose")
				delete(room.Users, user.Name)
				break
			}

			meta, err := room.HandleGameRequest(c, session, message)

			if err != nil {
				c.JSON(http.StatusBadRequest, gin.H{})
				return
			}

			response := GameResponse{
				message.Command,
				meta,
			}

			room.Channel <- response
		}
	}
}

func IndexGet() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.HTML(http.StatusOK, "index.html", gin.H{})
	}
}

func RoomGet() gin.HandlerFunc {
	return func(c *gin.Context) {
		roomname := c.Param("name")

		room, exists := roomDatabase[roomname]
		if !exists {
			c.HTML(http.StatusOK, "notfound.html", gin.H{})
			return
		}

		userid, err := c.Cookie("who-is-the-ghost")
		if err != nil {
			c.HTML(http.StatusOK, "notfound.html", gin.H{})
			return
		}

		uid, err := uuid.Parse(userid)
		if err != nil {
			c.HTML(http.StatusOK, "notfound.html", gin.H{})
			return
		}

		session, uexists := sessionStore[uid]
		if !uexists {
			c.HTML(http.StatusOK, "notfound.html", gin.H{})
			return
		}

		user := session.user

		_, isinroom := room.Users[user.Name]
		if !isinroom {
			c.HTML(http.StatusOK, "notfound.html", gin.H{})
			return
		}

		c.HTML(http.StatusOK, "room.html", gin.H{})
	}
}

func main() {
	readOdai()

	roomDatabase = make(map[string]*Room)
	sessionStore = make(map[uuid.UUID]*Session)

	r := gin.Default()

	r.LoadHTMLGlob("views/*")

	r.GET("/", IndexGet())
	r.GET("/room/:name", RoomGet())

	r.POST("/api", ApiPost())
	r.POST("/game", GamePost())

	r.GET("/socket", GameSocket())

	r.Run(":8080")
}

type Odai struct {
	A string
	B string
}

var odai []Odai

func readOdai() {
	fp, err := os.Open("src/odai.txt")
	if err != nil {
		panic(err)
	}
	defer fp.Close()

	scanner := bufio.NewScanner(fp)
	for scanner.Scan() {
		txt := scanner.Text()
		fmt.Println(txt)
		buf := strings.Split(txt, ",")
		odai = append(odai, Odai{buf[0], buf[1]})
	}

}
