package main

import (
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
)

type User struct {
	Name   string
	Id     uuid.UUID
	Vote   string
	socket *websocket.Conn
}

type Room struct {
	Name     string
	Password string
	Users    map[string]*User
	Channel  chan GameRequest
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
			req.UserName: &User{req.UserName, uuid.New(), "", nil},
		},
		make(chan GameRequest),
	}
}

type GameRequest struct {
	Command string `json:"command"`
	Meta    string `json:"meta"`
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

			user := &User{req.UserName, uuid.New(), "", nil}
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

		user := session.user
		room := session.room

		switch req.Command {
		case "update":
			keys := make([]string, 0, len(room.Users))
			for key := range room.Users {
				keys = append(keys, key)
			}

			c.JSON(http.StatusOK, gin.H{
				"command": "update",
				"members": keys,
			})
			return
		case "start":
			c.JSON(http.StatusOK, gin.H{
				"command": "start",
			})
			return
		case "end":
			c.JSON(http.StatusOK, gin.H{
				"command": "end",
			})
			return
		case "join":
			c.JSON(http.StatusOK, gin.H{
				"command": "join",
			})
			return
		case "watch":
			c.JSON(http.StatusOK, gin.H{
				"command": "watch",
			})
			return
		case "vote":
			c.JSON(http.StatusOK, gin.H{
				"command": "vote",
			})
			return
		case "judge":
			c.JSON(http.StatusOK, gin.H{
				"command": "judge",
			})
			return
		case "extend":
			c.JSON(http.StatusOK, gin.H{
				"command": "extend",
			})
			return
		case "exit":
			delete(room.Users, user.Name)
			delete(sessionStore, user.Id)

			if len(room.Users) == 0 {
				delete(roomDatabase, room.Name)
			}

			c.SetCookie("who-is-the-ghost", "", -1, "/", "", false, false)
			c.JSON(http.StatusOK, gin.H{
				"command": "exit",
			})
			return
		}
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
			room.Channel <- message
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
