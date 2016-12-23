package main

import (
	"flag"

	"github.com/proxy/util"
	"log"
	"net"
	"os"
	"strings"
	"time"
)

var (
	local = flag.String("l", "127.0.0.1:80", "反向代理地址")
	pwd   = flag.String("p", "guest", "密码")
	use   = flag.String("u", "guest", "用户名")
)

const remote = "vps.phpboke.cn:3456"

func main() {
	flag.Usage = util.Usage
	flag.Parse()
	log.Println("客户端启动: ", *local, "-> Server")
	for {
		connectServer()
		log.Println("断线重连中....")
		time.Sleep(5 * time.Second)
	}
}

func connectServer() {
	proxy, err := net.DialTimeout("tcp", remote, 5*time.Second)
	if err != nil {
		log.Println("服务器连接失败....")
		return
	}

	log.Println("服务器连接成功....")

	defer proxy.Close()
	util.WriteString(proxy, util.Base64Encode([]byte(*use+util.S_token+*pwd))+util.SEPS+util.C2P_CONNECT)

	for {
		proxy.SetReadDeadline(time.Now().Add(2 * time.Second))
		msg, err := util.ReadString(proxy)
		proxy.SetReadDeadline(time.Time{})
		if err == nil {
			if msg == util.P2C_NEW_SESSION {
				go session()
			} else {
				if strings.Count(msg, "ERROR:") > 0 {
					list := strings.Split(msg, "ERROR:")
					log.Println(list[1])
					os.Exit(1)
				}

				log.Println(msg)
			}
		} else {
			if nerr, ok := err.(net.Error); ok && nerr.Timeout() {
				proxy.SetWriteDeadline(time.Now().Add(2 * time.Second))
				_, werr := util.WriteString(proxy, util.C2P_KEEP_ALIVE)
				if werr != nil {
					return
				}

				continue
			} else {
				if *use == "guest" {
					log.Println("服务器已关闭")
					os.Exit(1)
				}

				return
			}
		}
	}
}

func session() {
	rp, err := net.Dial("tcp", remote)
	if err != nil {
		log.Println("服务器连接失败:")
		return
	}
	//defer util.CloseConn(rp)

	util.WriteString(rp, util.Base64Encode([]byte(*use+util.S_token+*pwd))+util.SEPS+util.C2P_SESSION)
	lp, err := net.Dial("tcp", *local)
	if err != nil {
		log.Println("连接失败:", *local)
		rp.Close()
		return
	}

	go util.CopyFromTo(rp, lp, nil)
	go util.CopyFromTo(lp, rp, nil)
}
