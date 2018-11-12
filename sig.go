package main

import (
	"bufio"
	"io"
	"log"
	"net"
	"os"
	"runtime"
)

const (
	sigBroken = -1 //所有携程已经退出(结束或者崩溃)
	sigReload = 1  //重新读取配置 并重新启动所有协程
	sigExit   = 0  //管理进程退出
	sigStart  = 2  //启动协程
)

var (
	sigMap      map[string]int
	serviceDonw chan bool
	s           *net.UnixListener
	signalChan  chan int
	sysSigChan  chan os.Signal
)

func init() {
	signalChan = make(chan int)
	sysSigChan = make(chan os.Signal)
	serviceDonw = make(chan bool)
	sigMap = map[string]int{
		"break":  sigBroken,
		"reload": sigReload,
		"exit":   sigExit,
	}
}

func startListenService() {
	//windows下暂时不做处理
	switch runtime.GOOS {
	case "windows":
	//macos和linux 启动unix通信
	case "darwin":
		unixListen()
	case "linux":

	}
}

func unixListen() {
	log.Println("listen service starting ...")
	var err error
	s, err = net.ListenUnix("unix", &net.UnixAddr{Name: sockPath, Net: "unix"})
	if err != nil {
		log.Fatalln("unix listen start faild : " + err.Error())
	}
	s.SetUnlinkOnClose(true)

	go func() {
		for {
			c, err := s.AcceptUnix()
			if err != nil {
				if _, ok := err.(*net.OpError); ok {
					break
				}

				log.Printf("unix listen accept error : %s\n", err.Error())
				continue
			}

			go listenHandle(c)
		}
	}()
	log.Println("unix listen service started")
}

func listenHandle(c *net.UnixConn) {
	reader := bufio.NewReader(c)
	defer c.Close()
	for {
		line, _, err := reader.ReadLine()
		if err != nil {
			if err == io.EOF {
				log.Println("client quit:" + err.Error())
			} else {
				log.Println("client error:" + err.Error())
			}
			break
		}

		//		c.SetDeadline(time.Now().Add(5 * time.Second))
		data := string(line)
		//向通道内发送信号
		if sig, ok := sigMap[data]; ok {
			signalChan <- sig
		} else {
			log.Println("undefined signal name : " + data)
		}
	}
}

func stopListenSerivce() {
	switch runtime.GOOS {
	case "windows":
	case "darwin":
		s.Close()
		log.Println("unix listen service stopped")
	case "linux":

	}
}

//启动一个协程 监听系统信号
//如果是ctrl+c 或普通 kill 信号 则发送退出指令 有序退出程序
func listenSystemSig() {
	go func() {
		sig := <-sysSigChan
		log.Println("system signal :" + sig.String())
		signalChan <- sigExit
	}()
}
