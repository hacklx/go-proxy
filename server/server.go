package main

import (
	"errors"
	"flag"
	"fmt"
	"github.com/proxy/util"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

var (
	_port = flag.String("p", "3456", "监听端口")
	_mu   = flag.String("mu", "root", "MYSQL 用户名")
	_mp   = flag.String("mp", "#6842666", "MYSQL 密码")
	_ml   = flag.String("ml", "127.0.0.1", "MYSQL 地址")
	_mh   = flag.Int("mh", 3306, "MYSQL 端口")
	_md   = flag.String("md", "go_proxy", "MYSQL 数据库名")
)

var mysql *util.Dbconfig = util.MysqlInit(*_mu, *_mp, *_ml, *_mh, *_md)
var db *util.Exec = nil
var hashLogin map[string]Login = make(map[string]Login)
var chSession chan net.Conn = make(chan net.Conn, 100)
var localhost = "127.0.0.1"

type Login struct {
	*UserInfo

	SessionId   string
	SessionConn net.Conn
	BindPort    int32
	Listener    net.Listener
}

type UserInfo struct {
	Id       int32
	User     string
	Pwd      string
	Status   int
	Ratelimt int
	Recvtime string
	Port     int
}

type IsPort struct {
	Id int32
}

type OnConnectFunc func(net.Conn, chan net.Conn, string)

func main() {

	db = mysql.Connect()
	flag.Usage = util.Usage
	flag.Parse()
	log.Println("服务正在启动....")

	go func() {
		resp, err := http.Get("http://myexternalip.com/raw")
		if err != nil {
			log.Println(err.Error())
			return
		}

		defer resp.Body.Close()

		data, err2 := ioutil.ReadAll(resp.Body)
		if err2 != nil {
			log.Println("ip获取失败")
			return
		}

		localhost = strings.Replace(string(data), "\n", "", -1)
	}()

	mysqlResut := db.Ping()

	if mysqlResut != nil {
		log.Println(mysqlResut)
		os.Exit(1)
	}

	log.Println("服务启动成功....")
	if nil != listen(*_port, chSession, onClientConnect, "") {
		return
	}

	defer db.Close()

	for {
		for _, item := range hashLogin {

			if item.Recvtime == "0000-00-00 00:00:00" {
				continue
			}

			tm, err := time.ParseInLocation("2006-01-02 15:04:05", item.Recvtime, time.Local)

			if err != nil {
				util.WriteString(item.SessionConn, "ERROR:账户异常")
				util.CloseConn(item.SessionConn)
				continue
			}

			if tm.Unix() <= time.Now().Unix() {
				util.WriteString(item.SessionConn, "ERROR:账户已过期")
				util.CloseConn(item.SessionConn)
				db.Query("UPDATE db_user SET port = ? WHERE id = ?", 0, item.Id)
				continue
			}
		}

		time.Sleep(10 * time.Second)
	}
}

func listen(port string, chSession chan net.Conn, onConnect OnConnectFunc, SessionId string) error {
	server, err := net.Listen("tcp", net.JoinHostPort("0.0.0.0", port))

	if err != nil {
		log.Fatal(err)
		return err
	}

	if SessionId != "" {
		LoginClient, flag := hashLogin[SessionId]
		if !flag {
			return errors.New("error")
		}

		LoginClient.Listener = server
		hashLogin[SessionId] = LoginClient
	}

	go func() {
		defer server.Close()
		for {
			conn, err := server.Accept()
			if err != nil {
				continue
			}
			go onConnect(conn, chSession, SessionId)
		}
	}()
	return nil
}

func onClientConnect(conn net.Conn, chSession chan net.Conn, SessionId string) {

	conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	msg, err := util.ReadString(conn)
	conn.SetReadDeadline(time.Time{})

	if err != nil {
		log.Println("Can't Read: ", err)
		conn.Close()
		return
	}

	msgs := strings.Split(msg, "\n")
	userinfo := strings.Split(util.Base64Decode(msgs[0]), util.S_token)

	if len(userinfo) < 2 {
		util.CloseConn(conn)
		return
	}

	userRow := &UserInfo{}

	resultError := db.QueryRow(
		"select * from db_user where "+
			"user =? and pwd =? ", userinfo[0], userinfo[1]).Scan(
		&userRow.Id, &userRow.User, &userRow.Pwd, &userRow.Status,
		&userRow.Ratelimt, &userRow.Recvtime, &userRow.Port,
	)

	if resultError != nil {
		util.WriteString(conn, "ERROR:用户名或密码错误")
		util.CloseConn(conn)
		return
	}

	if userRow.Status == 0 {
		util.WriteString(conn, "ERROR:账户已被禁用")
		util.CloseConn(conn)
		return
	}

	if userRow.Recvtime != "0000-00-00 00:00:00" {
		tm, err := time.ParseInLocation("2006-01-02 15:04:05", userRow.Recvtime, time.Local)

		if err != nil {
			util.WriteString(conn, "ERROR:账户异常")
			util.CloseConn(conn)
			return
		}

		if tm.Unix() <= time.Now().Unix() {
			util.WriteString(conn, "ERROR:账户已过期")
			util.CloseConn(conn)
			db.Query("UPDATE db_user SET port = ? WHERE id = ?", 0, userRow.Id)
			return
		}
	}

	token := msgs[1]
	if token == util.C2P_CONNECT {
		clientConnect(conn, userRow)
		return
	} else if token == util.C2P_SESSION {
		initUserSession(conn, chSession)
		return
	}
}

func clientConnect(conn net.Conn, userRow *UserInfo) {
	defer util.CloseConn(conn)
	SessionId := fmt.Sprintf("#%d", userRow.Id)

	_, isLogin := hashLogin[SessionId]

	if isLogin == true {
		util.WriteString(conn, "ERROR:请勿重复登录")
		util.CloseConn(conn)
		return
	}

	var BindPort int32 = 0
	var IsUse = false

	if userRow.Port == 0 {

		for {

			BindPort = util.RandPort(10000, 65535)
			_, err := net.Dial("tcp", fmt.Sprintf("127.0.0.1:%d", BindPort))
			if err != nil {
				portFlag := &IsPort{}
				resultError := db.QueryRow("select id from db_user where port = ? ", BindPort).Scan(portFlag.Id)

				if resultError != nil {

					IsUpdate := db.Query("UPDATE db_user SET port = ? WHERE id = ?", BindPort, userRow.Id)

					if IsUpdate {
						IsUse = true
					} else {
						//err log
						break
					}
				}
			}
			if IsUse {
				break
			}
			time.Sleep(time.Second * 1)
		}
	} else {
		BindPort = int32(userRow.Port)
	}

	hashLogin[SessionId] = Login{UserInfo: userRow, SessionId: SessionId, SessionConn: conn, BindPort: BindPort,Listener:nil}

	if nil != listen(strconv.Itoa(int(BindPort)), chSession, onUserConnect, SessionId) {
		delete(hashLogin, SessionId)
		util.CloseConn(conn)
		return
	}

	util.WriteString(conn, fmt.Sprintf("绑定地址:%s:%d\n", localhost, BindPort))

	for {
		_, err := util.ReadString(conn)

		if err != nil {
			Loginhash ,flag:= hashLogin[SessionId]
			if flag{
				if Loginhash.Listener != nil{
					Loginhash.Listener.Close()
				}
			}
			delete(hashLogin, SessionId)
			break
		}
	}
}

func initUserSession(conn net.Conn, chSession chan net.Conn) {
	chSession <- conn
}

func onUserConnect(conn net.Conn, chSession chan net.Conn, SessionId string) {

	SessionKeep, flag := hashLogin[SessionId]

	if flag == false {
		util.CloseConn(conn)
		return
	}

	_, err := util.WriteString(SessionKeep.SessionConn, util.P2C_NEW_SESSION)
	if err != nil {
		util.CloseConn(conn)
		return
	}

	connSession := recvSession(chSession)
	if connSession == nil {
		util.CloseConn(conn)
		return
	}

	go util.CopyRateTo(conn, connSession, SessionKeep.Ratelimt)
	go util.CopyRateTo(connSession, conn, SessionKeep.Ratelimt)
}

func recvSession(ch chan net.Conn) net.Conn {
	var conn net.Conn = nil
	select {
	case conn = <-ch:
	case <-time.After(time.Second * 5):
		conn = nil
	}
	return conn
}
